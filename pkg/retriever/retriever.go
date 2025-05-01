package retriever

import (
	"os"

	"github.com/OdyseeTeam/transcoder/pkg/dispatcher"
	"github.com/OdyseeTeam/transcoder/pkg/resolve"
)

type downloadTask struct {
	url, output string
}

type DownloadResult struct {
	File     *os.File
	Size     int64
	Resolved *resolve.ResolvedStream
}

type pool struct {
	dispatcher.Dispatcher
}

type worker struct{}

func Retrieve(url, out string) (*DownloadResult, error) {
	r, err := resolve.ResolveStream(url)
	if err != nil {
		return nil, err
	}
	f, n, err := r.Download(out)
	if err != nil {
		return nil, err
	}

	return &DownloadResult{f, n, r}, nil
}

// NewPool will create a pool of retrievers that you can throw work at.
func NewPool(parallel int) pool {
	d := dispatcher.Start(parallel, worker{}, 0)
	return pool{d}
}

// Retrieve throws download into a pool of workers.
// It will block if all workers are busy.
// Duplicate urls are not checked for.
func (p pool) Retrieve(url, out string) *dispatcher.Result {
	return p.Dispatch(downloadTask{url, out})
}

func (w worker) Work(t dispatcher.Task) error {
	dt := t.Payload.(downloadTask)
	res, err := Retrieve(dt.url, dt.output)
	if err != nil {
		return err
	}
	t.SetResult(res)
	return nil
}
