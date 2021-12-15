package tower

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/retriever"
	"github.com/lbryio/transcoder/storage"

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
	workDirs map[string]string
	encoder  encoder.Encoder
	s3       *storage.S3Driver
	log      logging.KVLogger
}

type task struct {
	url, sdHash, callbackURL, token string
	paths                           []string
}

type taskControl struct {
	Progress chan taskProgress
	TaskDone chan struct{}
	Errc     chan error
	Stop     chan struct{}
	Next     chan taskControl
}

type streamUploadResult struct {
	err error
	url string // sd hash
}

func newPipeline(workDir string, s3 *storage.S3Driver, encoder encoder.Encoder, logger logging.KVLogger) (*pipeline, error) {
	p := pipeline{
		workDir: workDir,
		encoder: encoder,
		log:     logger,
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
			_, err = p.s3.Put(ls)
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
		{
			task.progress <- taskProgress{Stage: StageDownloading}
			res, err := retriever.Retrieve(task.payload.URL, c.workDirs[dirStreams])
			if err != nil {
				log.Error("download failed", "err", err)
				task.errChan <- err
				return
			}
			encodedPath = path.Join(c.workDirs[dirTranscoded], res.Resolved.SDHash)
			origFile = res.File.Name()
			defer os.RemoveAll(origFile)
			defer os.RemoveAll(encodedPath)
		}

		{
			task.progress <- taskProgress{Stage: StageEncoding}
			res, err := c.encoder.Encode(origFile, encodedPath)
			if err != nil {
				log.Error("encoder failed", err)
				task.errChan <- err
				return
			}

			for p := range res.Progress {
				task.progress <- taskProgress{Percent: float32(p.GetProgress()), Stage: StageEncoding}
			}

			m := storage.Manifest{
				URL:     task.payload.URL,
				SDHash:  task.payload.SDHash,
				Formats: res.Formats,
			}
			ls, err = storage.OpenLocalStream(encodedPath, m)
			if err != nil {
				log.Error("could not initialize stream object", "err", err)
				task.errChan <- err
				return
			} else {
				ls.FillManifest()
				log.Info("transcoding done", "stream", ls)
			}
		}

		{
			task.progress <- taskProgress{Stage: StageUploading, Percent: 0}
			rs, err := c.s3.PutWithContext(context.Background(), ls)
			if err != nil {
				task.errChan <- err
				return
			}
			task.progress <- taskProgress{Stage: StageUploading, Percent: 100}
			task.result <- taskResult{remoteStream: rs}
		}
	}()
}

func (c taskControl) sendStatus(p taskProgress) {
	for {
		select {
		case <-c.Stop:
			return
		case <-c.TaskDone:
			return
		case c.Progress <- p:
			return
		}
	}
}

func (t *task) addPath(p string) error {
	if !path.IsAbs(p) {
		return fmt.Errorf("path is not absolute: %v", p)
	}
	t.paths = append(t.paths, p)
	return nil
}

func (t *task) cleanup() {
	for _, p := range t.paths {
		os.RemoveAll(p)
	}
}
