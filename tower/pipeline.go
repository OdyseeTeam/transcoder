package tower

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/pkg/retriever"
	"github.com/lbryio/transcoder/pkg/uploader"
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

type pipeline struct {
	workDir          string
	hearbeatInterval time.Duration
	workDirs         map[string]string
	encoder          encoder.Encoder
}

type task struct {
	url, callbackURL, token string
	paths                   []string
}

type pipelineProgress struct {
	Error   error        `json:"error"`
	Stage   RequestStage `json:"stage"`
	Percent float32      `json:"progress"`
}

func newPipeline(workDir string, encoder encoder.Encoder, hearbeatInterval time.Duration) (*pipeline, error) {
	c := pipeline{
		workDir:          workDir,
		hearbeatInterval: hearbeatInterval,
		encoder:          encoder,
	}

	return &c, nil
}

func (c *pipeline) Process(stop chan interface{}, t *task) (<-chan interface{}, <-chan pipelineProgress) {
	status := make(chan pipelineProgress)
	heartbeat := make(chan interface{})
	pulse := time.NewTicker(c.hearbeatInterval)
	funcExit := make(chan interface{})

	go func() {
		defer close(heartbeat)
		defer close(status)
		defer close(funcExit)

		// sendPulse wouldn't block if no one is listening
		sendPulse := func() {
			select {
			case heartbeat <- struct{}{}:
			default:
			}
		}

		go func() {
			for {
				select {
				case <-stop:
					return
				case <-funcExit:
					return
				case <-pulse.C:
					sendPulse()
				}
			}
		}()

		sendStatus := func(p pipelineProgress) {
			for {
				select {
				case <-stop:
					return
				case <-funcExit:
					return
				case status <- p:
					return
				}
			}
		}

		var origFile, encodedPath string
		{
			sendStatus(pipelineProgress{Stage: StageDownloading})
			res, err := retriever.Retrieve(t.url, path.Join(c.workDir, "downloads"))
			if err != nil {
				sendStatus(pipelineProgress{Error: err, Stage: StageDownloading})
				return
			}
			encodedPath = path.Join(c.workDir, "transcoded", res.Resolved.SDHash)
			origFile = res.File.Name()
			t.addPath(origFile)
			t.addPath(encodedPath)
		}

		{
			sendStatus(pipelineProgress{Stage: StageEncoding})
			res, err := c.encoder.Encode(origFile, encodedPath)
			if err != nil {
				sendStatus(pipelineProgress{Error: err, Stage: StageEncoding})
				return
			}

			for p := range res.Progress {
				sendStatus(pipelineProgress{Percent: float32(p.GetProgress()), Stage: StageEncoding})
			}
		}

		{
			sendStatus(pipelineProgress{Stage: StageUploading})
			err := uploader.Upload(encodedPath, t.callbackURL, t.token)
			if err != nil {
				sendStatus(pipelineProgress{Error: err, Stage: StageUploading})
				return
			}
		}

		sendStatus(pipelineProgress{Stage: StageDone})
	}()

	return heartbeat, status
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
