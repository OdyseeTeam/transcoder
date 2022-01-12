package tower

import (
	"context"
	"math"
	"os"
	"path"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/retriever"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/tower/metrics"

	"github.com/karrick/godirwalk"
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

func (p *pipeline) UploadLeftovers(stop chan struct{}) (<-chan streamUploadResult, error) {
	// Upload left over streams
	// tc.sendStatus(taskProgress{Stage: StageUploading, Percent: 0})
	streams, err := godirwalk.ReadDirnames(p.workDirs[dirTranscoded], nil)

	if err != nil {
		return nil, errors.Wrap(err, "cannot get streams list")
	}

	results := make(chan streamUploadResult)

	go func() {
		defer close(results)
		for _, sdHash := range streams {
			select {
			case <-stop:
				return
			default:
			}
			// Skip non-sdHashes
			if len(sdHash) != 96 {
				continue
			}
			res := streamUploadResult{url: sdHash}
			ls, err := storage.OpenLocalStream(path.Join(p.workDirs[dirTranscoded], sdHash))
			if err != nil {
				res.err = errors.Wrap(err, "cannot open stream")
				results <- res
				return
			}
			_, err = p.s3.Put(ls, true)
			if err != nil {
				res.err = errors.Wrap(err, "cannot upload stream")
				results <- res
				return
			}
			results <- res
		}
	}()
	return results, nil
}

func (c *pipeline) Process(stop chan struct{}, task workerTask) {
	var ls *storage.LocalStream
	log := logging.AddLogRef(c.log, task.payload.SDHash)

	go func() {
		var origFile, encodedPath string
		errMtr := metrics.TranscodingErrors

		{
			runMtr := metrics.PipelineStagesRunning.WithLabelValues(string(StageDownloading))

			task.progress <- taskProgress{Stage: StageDownloading}

			runMtr.Inc()
			res, err := retriever.Retrieve(task.payload.URL, c.workDirs[dirStreams])
			if err != nil {
				log.Error("download failed", "err", err)
				task.errors <- taskError{err: err, fatal: false}
				runMtr.Dec()
				errMtr.WithLabelValues(string(StageDownloading)).Inc()
				return
			}
			runMtr.Dec()
			encodedPath = path.Join(c.workDirs[dirTranscoded], res.Resolved.SDHash)
			origFile = res.File.Name()
			defer os.RemoveAll(origFile)
			defer os.RemoveAll(encodedPath)
		}

		{
			runMtr := metrics.PipelineStagesRunning.WithLabelValues(string(StageEncoding))
			task.progress <- taskProgress{Stage: StageEncoding}
			res, err := c.encoder.Encode(origFile, encodedPath)
			if err != nil {
				log.Error("encoder failed", "err", err)
				task.errors <- taskError{err: errors.Wrap(err, "encoder failed"), fatal: true}
				runMtr.Dec()
				errMtr.WithLabelValues(string(StageEncoding)).Inc()
				return
			}

			runMtr.Inc()
			seen := map[int]bool{}
			for p := range res.Progress {
				pg := int(math.Ceil(p.GetProgress()))
				if pg%10 == 0 && !seen[pg] {
					task.progress <- taskProgress{Percent: float32(pg), Stage: StageEncoding}
				}
			}

			m := storage.Manifest{
				URL:     task.payload.URL,
				SDHash:  task.payload.SDHash,
				Formats: res.Formats,
			}
			ls, err = storage.OpenLocalStream(encodedPath, m)
			if err != nil {
				log.Error("stream object initialization failed", "err", err)
				task.errors <- taskError{err: errors.Wrap(err, "stream object initialization failed"), fatal: true}
				runMtr.Dec()
				errMtr.WithLabelValues(string(StageUploading)).Inc()
				return
			}
			runMtr.Dec()

			err = ls.FillManifest()
			if err != nil {
				log.Error("failed to fill manifest", "err", err)
				task.errors <- taskError{err: errors.Wrap(err, "failed to fill manifest"), fatal: true}
				runMtr.Dec()
				errMtr.WithLabelValues(string(StageMetadataFill)).Inc()
				return
			}

			log.Info("encoding done", "stream", ls)
			defer os.RemoveAll(ls.Path)
		}

		{
			runMtr := metrics.PipelineStagesRunning.WithLabelValues(c.workerID, string(StageUploading))
			task.progress <- taskProgress{Stage: StageUploading, Percent: 0}
			runMtr.Inc()
			rs, err := c.s3.PutWithContext(context.Background(), ls, true)
			if err != nil {
				e := taskError{err: errors.Wrap(err, "stream upload failed")}
				if errors.Is(err, storage.ErrStreamExists) {
					e.fatal = true
				}
				task.errors <- e
				runMtr.Dec()
				errMtr.WithLabelValues(string(StageUploading)).Inc()
				return
			}
			runMtr.Dec()
			task.progress <- taskProgress{Stage: StageUploading, Percent: 100}
			task.result <- taskResult{remoteStream: rs}
		}
	}()
}
