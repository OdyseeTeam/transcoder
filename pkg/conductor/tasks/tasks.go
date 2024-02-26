package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/odyseeteam/transcoder/encoder"
	"github.com/odyseeteam/transcoder/internal/version"
	"github.com/odyseeteam/transcoder/library"
	"github.com/odyseeteam/transcoder/pkg/conductor/metrics"
	"github.com/odyseeteam/transcoder/pkg/logging"
	"github.com/odyseeteam/transcoder/pkg/resolve"
	"github.com/odyseeteam/transcoder/pkg/retriever"
	"github.com/odyseeteam/transcoder/storage"

	"github.com/hibiken/asynq"
	"github.com/pkg/errors"
	redis "github.com/redis/go-redis/v9"
)

const (
	TypeTranscodingRequest = "transcoder:transcode"

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
	Name                  string
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

// WithName sets worker name (defaults to os.Hostname).
func WithName(name string) func(options *EncoderRunnerOptions) {
	return func(options *EncoderRunnerOptions) {
		options.Name = name
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
	if options.Name == "" {
		options.Name, _ = os.Hostname()
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
	log := logging.AddLogRef(r.options.Logger, payload.SDHash)
	if t.ResultWriter() != nil {
		log = log.With("tid", t.ResultWriter().TaskID())
	}

	var origFile, encodedPath string
	errMtr := metrics.ErrorsCount

	var resolved *resolve.ResolvedStream

	{
		timer := time.Now()
		runMtr := metrics.StageRunning.WithLabelValues(metrics.StageDownloading)
		spentMtr := metrics.SpentSeconds.WithLabelValues(metrics.StageDownloading)

		runMtr.Inc()
		dl, err := retriever.Retrieve(payload.URL, r.options.StreamsDir)
		if err != nil {
			log.Error("download failed", "err", err)
			errMtr.WithLabelValues(metrics.StageDownloading).Inc()
			spentMtr.Add(time.Since(timer).Seconds())
			runMtr.Dec()
			return fmt.Errorf("download failed: %w", err)
		}
		metrics.InputBytes.Add(float64(dl.Size))
		runMtr.Dec()
		spentMtr.Add(time.Since(timer).Seconds())
		encodedPath = path.Join(r.options.OutputDir, dl.Resolved.SDHash)
		origFile = dl.File.Name()
		defer os.RemoveAll(encodedPath)
		defer os.RemoveAll(origFile)
		resolved = dl.Resolved
	}

	{
		timer := time.Now()
		runMtr := metrics.StageRunning.WithLabelValues(metrics.StageEncoding)
		spentMtr := metrics.SpentSeconds.WithLabelValues(metrics.StageEncoding)

		runMtr.Inc()
		res, err := r.encoder.Encode(origFile, encodedPath)
		if err != nil {
			log.Error("encoder failure", "err", err)
			spentMtr.Add(time.Since(timer).Seconds())
			errMtr.WithLabelValues(metrics.StageEncoding).Inc()
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

		time.Sleep(5 * time.Second)
		// This is removed twice to not wait for upload to finish before freeing up disk space
		os.RemoveAll(origFile)

		stream = library.InitStream(encodedPath, r.storage.Name())
		err = stream.GenerateManifest(
			payload.URL, resolved.ChannelURI, payload.SDHash,
			library.WithTimestamp(time.Now()),
			library.WithWorkerName(r.options.Name),
			library.WithVersion(version.Version),
		)
		if err != nil {
			log.Error("failed to fill manifest", "err", err)
			runMtr.Dec()
			spentMtr.Add(time.Since(timer).Seconds())
			errMtr.WithLabelValues(metrics.StageMetadataFill).Inc()
			return fmt.Errorf("failed to fill manifest: %w", err)
		}

		stream.Manifest.FfmpegArgs = res.Ladder.String()
		metrics.OutputBytes.Add(float64(stream.Size()))
		metrics.TranscodedCount.Inc()
		d, _ := strconv.ParseFloat(res.OrigMeta.FMeta.GetFormat().GetDuration(), 64)
		metrics.TranscodedSeconds.Add(d)
		spentMtr.Add(time.Since(timer).Seconds())
		runMtr.Dec()

		log.Info("encoding done", "stream_size", stream.Size())
		defer os.RemoveAll(stream.LocalPath)
	}

	{
		timer := time.Now()
		runMtr := metrics.StageRunning.WithLabelValues(metrics.StageUploading)
		spentMtr := metrics.SpentSeconds.WithLabelValues(metrics.StageUploading)

		runMtr.Inc()
		err := r.storage.PutWithContext(context.Background(), stream, true)
		if err != nil {
			errMtr.WithLabelValues(metrics.StageUploading).Inc()
			spentMtr.Add(time.Since(timer).Seconds())
			runMtr.Dec()
			if errors.Is(err, storage.ErrStreamExists) {
				return fmt.Errorf("stream already exists: %v: %w", err, asynq.SkipRetry)
			}
			return fmt.Errorf("stream upload failed: %w", err)
		}
		log.Info("stream uploaded")
		spentMtr.Add(time.Since(timer).Seconds())
		runMtr.Dec()
		res, err := json.Marshal(TranscodingResult{Stream: stream})
		if err != nil {
			return fmt.Errorf("cannot serialize transcoding result: %w", err)
		}
		if t.ResultWriter() != nil {
			t.ResultWriter().Write(res)
		}
		r.resultWriter.Write(res)
		log.Info("stream processed")
	}

	return nil
}

func (r *EncoderRunner) RetryDelay(n int, e error, t *asynq.Task) time.Duration {
	if errors.Is(e, resolve.ErrNotReflected) {
		delay := 1 * time.Hour
		var payload TranscodingRequest
		json.Unmarshal(t.Payload(), &payload)
		r.options.Logger.Info("stream not fully reflected, delayed", "payload", payload, "delay", delay)
		return delay
	}
	if errors.Is(e, resolve.ErrNetwork) {
		delay := 2 * time.Minute
		r.options.Logger.Info("resolve failed, delayed", "err", e, "delay", delay)
		return delay
	}
	// return asynq.DefaultRetryDelayFunc(n, e, t)
	return 1 * time.Minute
}

func (r *EncoderRunner) Cleanup() {
	os.RemoveAll(r.options.StreamsDir)
	os.RemoveAll(r.options.OutputDir)
}

// func (r *EncoderRunner) err(format string, a ...interface{}) error {
// 	r.options.Logger.("stream not fully reflected, delayed", "payload", payload, "delay", delay)
// }

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
