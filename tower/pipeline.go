package tower

import (
	"context"
	"math"
	"os"
	"path"
	"time"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/library"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/resolve"
	"github.com/lbryio/transcoder/pkg/retriever"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/tower/metrics"

	"github.com/pkg/errors"
)

// inject task: url. claim id, sd hash
// -> download -> (in, out)
// -> encode -> (progress)
// -> upload -> (progress)
// -> done

// Separate chains for download, transcoding and upload so things can happen at full speed.
// So Dispatcher is still necessary

// Whole chain should have unbuffered channels so it doesn't get overloaded with things waiting in the pipeline.

// Worker should not have any state in v1, whatever got lost between restarts is lost.

// Worker rabbitmq concurrency should be 1 so it doesn't ack more tasks that it can start.

const (
	dirStreams    = "streams"
	dirTranscoded = "transcoded"
)

type pipeline struct {
	workDir  string
	workerID string
	workDirs map[string]string
	encoder  encoder.Encoder
	s3       *storage.S3Driver
	log      logging.KVLogger
}

type streamUploadResult struct {
	err error
	url string // sd hash
}

func newPipeline(workDir, workerID string, s3 *storage.S3Driver, encoder encoder.Encoder, logger logging.KVLogger) (*pipeline, error) {
	p := pipeline{
		workDir:  workDir,
		encoder:  encoder,
		log:      logger,
		workerID: workerID,
	}

	p.workDirs = map[string]string{
		dirStreams:    path.Join(p.workDir, dirStreams),
		dirTranscoded: path.Join(p.workDir, dirTranscoded),
	}
	p.s3 = s3

	return &p, nil
}

func (c *pipeline) Process(task workerTask) {
	var stream *library.Stream
	log := logging.AddLogRef(c.log, task.payload.SDHash)

	go func() {
		var origFile, encodedPath string
		errMtr := metrics.TranscodingErrorsCount

		var resolved *resolve.ResolvedStream

		{
			timer := time.Now()
			runMtr := metrics.PipelineStagesRunning.WithLabelValues(string(StageDownloading))
			spentMtr := metrics.PipelineSpentSeconds.WithLabelValues(string(StageDownloading))
			task.progress <- taskProgress{Stage: StageDownloading}

			runMtr.Inc()
			dl, err := retriever.Retrieve(task.payload.URL, c.workDirs[dirStreams])
			if err != nil {
				log.Error("download failed", "err", err)
				errMtr.WithLabelValues(string(StageDownloading)).Inc()
				spentMtr.Add(time.Since(timer).Seconds())
				runMtr.Dec()
				task.errors <- taskError{err: err, fatal: false}
				return
			}
			metrics.InputBytes.Add(float64(dl.Size))
			runMtr.Dec()
			spentMtr.Add(time.Since(timer).Seconds())
			encodedPath = path.Join(c.workDirs[dirTranscoded], dl.Resolved.SDHash)
			origFile = dl.File.Name()
			defer os.RemoveAll(origFile)
			defer os.RemoveAll(encodedPath)
			resolved = dl.Resolved
		}

		{
			timer := time.Now()
			runMtr := metrics.PipelineStagesRunning.WithLabelValues(string(StageEncoding))
			spentMtr := metrics.PipelineSpentSeconds.WithLabelValues(string(StageEncoding))

			task.progress <- taskProgress{Stage: StageEncoding}

			runMtr.Inc()
			res, err := c.encoder.Encode(origFile, encodedPath)
			if err != nil {
				log.Error("encoder failed", "err", err)
				spentMtr.Add(time.Since(timer).Seconds())
				errMtr.WithLabelValues(string(StageEncoding)).Inc()
				runMtr.Dec()
				task.errors <- taskError{err: errors.Wrap(err, "encoder failed"), fatal: true}
				return
			}

			seen := map[int]bool{}
			for p := range res.Progress {
				pg := int(math.Ceil(p.GetProgress()))
				if pg%5 == 0 && !seen[pg] {
					seen[pg] = true
					task.progress <- taskProgress{Percent: float32(pg), Stage: StageEncoding}
				}
			}

			time.Sleep(5 * time.Second)
			stream = library.InitStream(encodedPath, c.s3.Name())
			err = stream.GenerateManifest(task.payload.URL, resolved.ChannelURI, task.payload.SDHash)
			if err != nil {
				log.Error("failed to fill manifest", "err", err)
				runMtr.Dec()
				spentMtr.Add(time.Since(timer).Seconds())
				errMtr.WithLabelValues(string(StageMetadataFill)).Inc()
				task.errors <- taskError{err: errors.Wrap(err, "failed to fill manifest"), fatal: true}
				return
			}
			stream.Manifest.Ladder = res.Ladder
			metrics.OutputBytes.Add(float64(stream.Size()))
			spentMtr.Add(time.Since(timer).Seconds())
			runMtr.Dec()

			log.Info("encoding done", "stream", stream)
			defer os.RemoveAll(stream.LocalPath)
		}

		{
			timer := time.Now()
			runMtr := metrics.PipelineStagesRunning.WithLabelValues(string(StageUploading))
			spentMtr := metrics.PipelineSpentSeconds.WithLabelValues(string(StageUploading))

			task.progress <- taskProgress{Stage: StageUploading, Percent: 0}
			runMtr.Inc()
			err := c.s3.PutWithContext(context.Background(), stream, true)
			if err != nil {
				e := taskError{err: errors.Wrap(err, "stream upload failed")}
				if errors.Is(err, storage.ErrStreamExists) {
					e.fatal = true
				}
				errMtr.WithLabelValues(string(StageUploading)).Inc()
				spentMtr.Add(time.Since(timer).Seconds())
				runMtr.Dec()

				task.errors <- e
				return
			}
			spentMtr.Add(time.Since(timer).Seconds())
			runMtr.Dec()
			task.progress <- taskProgress{Stage: StageUploading, Percent: 100}
			task.result <- taskResult{remoteStream: stream}
		}
	}()
}
