package tower

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/retriever"
	"github.com/lbryio/transcoder/pkg/uploader"
	"github.com/lbryio/transcoder/storage"
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
	workDir          string
	hearbeatInterval time.Duration
	workDirs         map[string]string
	encoder          encoder.Encoder
	batchUploader    uploader.BatchUploader
	log              logging.KVLogger
}

type task struct {
	url, sdHash, callbackURL, token string
	paths                           []string
}

type taskControl struct {
	Progress chan pipelineProgress
	TaskDone chan struct{}
	Errc     chan error
	Stop     chan struct{}
	Next     chan taskControl
}

type pipelineProgress struct {
	Stage   RequestStage `json:"stage"`
	Percent float32      `json:"progress"`
}

func newPipeline(workDir string, encoder encoder.Encoder, hearbeatInterval time.Duration, logger logging.KVLogger) (*pipeline, error) {
	p := pipeline{
		workDir:          workDir,
		hearbeatInterval: hearbeatInterval,
		encoder:          encoder,
		log:              logger,
	}

	p.workDirs = map[string]string{
		dirStreams:    path.Join(p.workDir, dirStreams),
		dirTranscoded: path.Join(p.workDir, dirTranscoded),
	}
	p.batchUploader = uploader.StartBatchUploader(uploader.NewUploader(
		uploader.DefaultUploaderConfig().Logger(logger),
	), 10)

	// Upload left over streams
	err := p.batchUploader.UploadDir(workDir)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *pipeline) Process(stop chan struct{}, t *task) taskControl {
	var ls *storage.LightLocalStream
	status := make(chan pipelineProgress)
	done := make(chan struct{})
	errc := make(chan error, 1)
	tc := taskControl{
		Progress: status,
		TaskDone: done,
		Errc:     errc,
		Next:     make(chan taskControl, 1),
	}

	go func() {
		defer close(tc.TaskDone)
		var origFile, encodedPath string
		{
			tc.sendStatus(pipelineProgress{Stage: StageDownloading})
			res, err := retriever.Retrieve(t.url, c.workDirs[dirStreams])
			if err != nil {
				c.log.Error("download failed", "err", err)
				errc <- err
				return
			}
			encodedPath = path.Join(c.workDirs[dirTranscoded], res.Resolved.SDHash)
			origFile = res.File.Name()
			t.addPath(origFile)
			t.addPath(encodedPath)
		}

		{
			tc.sendStatus(pipelineProgress{Stage: StageEncoding})
			res, err := c.encoder.Encode(origFile, encodedPath)
			if err != nil {
				c.log.Error("encoder failed", err)
				errc <- err
				return
			}

			for p := range res.Progress {
				tc.sendStatus(pipelineProgress{Percent: float32(p.GetProgress()), Stage: StageEncoding})
			}

			m := &storage.Manifest{
				URL:     t.url,
				SDHash:  t.sdHash,
				Formats: res.Formats,
				Tower: &storage.TowerStreamCredentials{
					CallbackURL: t.callbackURL,
					Token:       t.token,
				},
			}
			ls, err = storage.InitLocalStream(encodedPath, m)
			if err != nil {
				c.log.Error("could not initialize stream object", "err", err)
				errc <- err
				return
			} else {
				c.log.Info("transcoding done", "stream", ls)
			}
		}

		{
			tc.sendStatus(pipelineProgress{Stage: StageUploading, Percent: 0})
			_, upDone, upErrc := c.batchUploader.Upload(ls)
			select {
			case <-upDone:
				tc.sendStatus(pipelineProgress{Stage: StageUploading, Percent: 100})
			case err := <-upErrc:
				errc <- err
			}
		}
	}()

	return tc
}

func (c taskControl) sendStatus(p pipelineProgress) {
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
