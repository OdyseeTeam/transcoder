package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/hibiken/asynq"
	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/library"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/resolve"
	"github.com/lbryio/transcoder/pkg/retriever"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/tower/metrics"
	"github.com/pkg/errors"
)

const (
	TypeTranscodingRequest = "transcoder:transcode"

	StageAccepted     = "accepted"
	StageDownloading  = "downloading"
	StageEncoding     = "encoding"
	StageUploading    = "uploading"
	StageMetadataFill = "metadata_fill"

	QueueTranscodingResults = "transcoding:results"
)

type ResultWriter interface {
	io.Writer
}

type EncoderRunner struct {
	resultWriter ResultWriter
	encoder      encoder.Encoder
	storage      *storage.S3Driver
	options      *EncoderRunnerOptions
}

type EncoderRunnerOptions struct {
	StreamsDir, OutputDir string
	Logger                logging.KVLogger
}

type RedisResultWriter struct {
	rdb redis.UniversalClient
}

func WithLogger(logger logging.KVLogger) func(options *EncoderRunnerOptions) {
	return func(options *EncoderRunnerOptions) {
		options.Logger = logger
	}
}

func WithStreamsDir(dir string) func(options *EncoderRunnerOptions) {
	return func(options *EncoderRunnerOptions) {
		options.StreamsDir = dir
	}
}

func WithOutputDir(dir string) func(options *EncoderRunnerOptions) {
	return func(options *EncoderRunnerOptions) {
		options.OutputDir = dir
	}
}

func NewTranscodingTask(req TranscodingRequest) (*asynq.Task, error) {
	return asynq.NewTask(TypeTranscodingRequest, []byte(req.String()), asynq.MaxRetry(5)), nil
}

func NewEncoderRunner(
	storage *storage.S3Driver, encoder encoder.Encoder, resultWriter ResultWriter, optionFuncs ...func(*EncoderRunnerOptions),
) (*EncoderRunner, error) {
	options := &EncoderRunnerOptions{
		Logger: logging.NoopKVLogger{},
	}
	for _, optionFunc := range optionFuncs {
		optionFunc(options)
	}
	if options.StreamsDir == "" {
		d, err := os.MkdirTemp("", "streams")
		if err != nil {
			return nil, err
		}
		options.StreamsDir = d
	}
	if options.OutputDir == "" {
		d, err := os.MkdirTemp("", "output")
		if err != nil {
			return nil, err
		}
		options.OutputDir = d
	}
	r := &EncoderRunner{
		encoder:      encoder,
		resultWriter: resultWriter,
		storage:      storage,
		options:      options,
	}

	return r, nil
}

func (r *EncoderRunner) Run(ctx context.Context, t *asynq.Task) error {
	if t.Type() != TypeTranscodingRequest {
		return fmt.Errorf("can only handle %s", TypeTranscodingRequest)
	}
	var payload TranscodingRequest
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}

	var stream *library.Stream
	log := logging.AddLogRef(r.options.Logger, payload.SDHash).With("tid", t.ResultWriter().TaskID())

	var origFile, encodedPath string
	errMtr := metrics.TranscodingErrorsCount

	var resolved *resolve.ResolvedStream

	{
		timer := time.Now()
		runMtr := metrics.PipelineStagesRunning.WithLabelValues(string(StageDownloading))
		spentMtr := metrics.PipelineSpentSeconds.WithLabelValues(string(StageDownloading))

		runMtr.Inc()
		dl, err := retriever.Retrieve(payload.URL, r.options.StreamsDir)
		if err != nil {
			log.Error("download failed", "err", err)
			errMtr.WithLabelValues(string(StageDownloading)).Inc()
			spentMtr.Add(time.Since(timer).Seconds())
			runMtr.Dec()
			return errors.Wrap(err, "download failed")
		}
		metrics.InputBytes.Add(float64(dl.Size))
		runMtr.Dec()
		spentMtr.Add(time.Since(timer).Seconds())
		encodedPath = path.Join(r.options.OutputDir, dl.Resolved.SDHash)
		origFile = dl.File.Name()
		defer os.RemoveAll(origFile)
		defer os.RemoveAll(encodedPath)
		resolved = dl.Resolved
	}

	{
		timer := time.Now()
		runMtr := metrics.PipelineStagesRunning.WithLabelValues(string(StageEncoding))
		spentMtr := metrics.PipelineSpentSeconds.WithLabelValues(string(StageEncoding))

		runMtr.Inc()
		res, err := r.encoder.Encode(origFile, encodedPath)
		if err != nil {
			log.Error("encoder failure", "err", err)
			spentMtr.Add(time.Since(timer).Seconds())
			errMtr.WithLabelValues(string(StageEncoding)).Inc()
			runMtr.Dec()

			return fmt.Errorf("encoder failure: %v: %w", err, asynq.SkipRetry)
		}

		seen := map[int]bool{}
		for p := range res.Progress {
			pg := int(math.Ceil(p.GetProgress()))
			if pg%5 == 0 && !seen[pg] {
				seen[pg] = true
				log.Info("encoding", "progress", pg)
			}
		}

		stream = library.InitStream(encodedPath, r.storage.Name())
		err = stream.GenerateManifest(payload.URL, resolved.ChannelURI, payload.SDHash)
		if err != nil {
			log.Error("failed to fill manifest", "err", err)
			runMtr.Dec()
			spentMtr.Add(time.Since(timer).Seconds())
			errMtr.WithLabelValues(string(StageMetadataFill)).Inc()
			return fmt.Errorf("failed to fill manifest: %w", err)
		}
		stream.Manifest.Ladder = res.Ladder
		metrics.OutputBytes.Add(float64(stream.Size()))
		spentMtr.Add(time.Since(timer).Seconds())
		runMtr.Dec()

		log.Info("encoding done", "stream_size", stream.Size())
		defer os.RemoveAll(stream.LocalPath)
	}

	{
		timer := time.Now()
		runMtr := metrics.PipelineStagesRunning.WithLabelValues(string(StageUploading))
		spentMtr := metrics.PipelineSpentSeconds.WithLabelValues(string(StageUploading))

		runMtr.Inc()
		err := r.storage.PutWithContext(context.Background(), stream, true)
		if err != nil {
			errMtr.WithLabelValues(string(StageUploading)).Inc()
			spentMtr.Add(time.Since(timer).Seconds())
			runMtr.Dec()
			if errors.Is(err, storage.ErrStreamExists) {
				return fmt.Errorf("stream upload failed: %w", err)
			}
			return fmt.Errorf("stream upload failed: %v: %w", err, asynq.SkipRetry)
		}
		log.Info("stream uploaded")
		spentMtr.Add(time.Since(timer).Seconds())
		runMtr.Dec()
		res, err := json.Marshal(TranscodingResult{Stream: stream})
		if err != nil {
			return fmt.Errorf("cannot serialize transcoding result: %w", err)
		}
		t.ResultWriter().Write(res)
		r.resultWriter.Write(res)
		log.Info("stream processed")
	}

	return nil
}

func (r *EncoderRunner) Cleanup() {
	os.RemoveAll(r.options.StreamsDir)
	os.RemoveAll(r.options.OutputDir)
}

func NewResultWriter(redisOpts asynq.RedisConnOpt) *RedisResultWriter {
	rw := &RedisResultWriter{
		rdb: redisOpts.MakeRedisClient().(redis.UniversalClient),
	}
	return rw
}

func (w *RedisResultWriter) Write(data []byte) (int, error) {
	_, err := w.rdb.RPush(context.Background(), QueueTranscodingResults, data).Result()
	if err != nil {
		return 0, err
	}
	return len(data), nil
}
