package tower

import (
	"path"
	"time"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/dispatcher"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/retriever"
	"github.com/lbryio/transcoder/pkg/uploader"

	zapadapter "github.com/lbryio/transcoder/pkg/logging/adapter-zap"
)

// inject task: url. claim id, sd hash
// -> download -> (in, out)
// -> encode -> (progress)
// -> upload -> (progress)
// -> done

// Separate chains for download, transcoding and upload so things can happen at full speed.
// So Dispatcher is still necessary

// Whole chain should have unbuffered channels so it doesn't get overloaded with things waiting in the pipeline.

// Client should not have any state in v1, whatever got lost between restarts is lost.

// Client rabbitmq concurrency should be 1 so it doesn't ack more tasks that it can start.

type Retriever interface {
	Retrieve(url, out string) *dispatcher.Result
}

type Encoder interface {
	Encode(in, out string) *dispatcher.Result
}

type Uploader interface {
	Upload(path, url string) *dispatcher.Result
}

type chain struct {
	baseDir   string
	workDirs  map[string]string
	retriever Retriever
	encoder   Encoder
	uploader  Uploader
}

type chainTask struct {
	url string
}

type chainProgress struct {
	Error   error
	Stage   string
	Percent float64
}

func newChain(workDir string, concurrency int) (*chain, error) {
	enc, err := encoder.NewEncoder(encoder.Configure())
	if err != nil {
		return nil, err
	}
	c := chain{
		baseDir:   workDir,
		retriever: retriever.NewPool(concurrency),
		encoder:   encoder.NewPool(enc, concurrency),
		uploader:  uploader.NewPool(concurrency),
	}

	return &c, nil
}

func (c *chain) Enter(url, callbackUrl string) <-chan chainProgress {
	progress := make(chan chainProgress, 200)

	go func() {
		progress <- chainProgress{Stage: "download"}
		defer close(progress)
		r := c.retriever.Retrieve(url, path.Join(c.baseDir, "downloads"))
		res := <-r.Value()
		if r.Error != nil {
			progress <- chainProgress{Error: r.Error, Stage: "download"}
			return
		}
		dr := res.(retriever.DownloadResult)
		encodedPath := path.Join(c.baseDir, "transcoded", dr.SDHash)

		r = c.encoder.Encode(dr.File.Name(), encodedPath)
		res = <-r.Value()
		if r.Error != nil {
			progress <- chainProgress{Error: r.Error, Stage: "encoding"}
			return
		}
		encResult := res.(*encoder.Result)

		for p := range encResult.Progress {
			progress <- chainProgress{Percent: p.GetProgress(), Stage: "encoding"}
		}

		progress <- chainProgress{Stage: "upload"}
		r = c.uploader.Upload(encResult.Output, callbackUrl)
		<-r.Value()
		if r.Error != nil {
			progress <- chainProgress{Error: r.Error, Stage: "upload"}
			return
		}
		progress <- chainProgress{Stage: "done"}
	}()

	return progress
}

func Start(mgr *manager.VideoManager) (chan<- interface{}, error) {
	logger := zapadapter.NewKV(logging.Create("formats", logging.Prod).Desugar())
	srv, err := NewServer("amqp://guest:guest@localhost/")
	if err != nil {
		return nil, err
	}
	stopChan := make(chan interface{})

	requests := mgr.Requests()
	go func() {
		for {
			for srv.Available() == 0 {
				time.Sleep(1000 * time.Second)
			}
			select {
			case r := <-requests:
				if r != nil {
					logger.Info("got transcoding request", "lbry_url", r.URI)
					srv.SendRequest(
						request{
							Payload: Payload{
								URL:         r.URI,
								CallbackURL: "????",
							},
						},
					)
				}
			case <-stopChan:
				// d.Stop()
				return
			}
		}
	}()
	return stopChan, nil
}
