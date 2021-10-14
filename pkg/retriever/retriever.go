package retriever

import (
	"os"

	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/dispatcher"
)

type downloadTask struct {
	url, output string
}

type DownloadResult struct {
	File   *os.File
	Size   int64
	SDHash string
}

type pool struct {
	dispatcher.Dispatcher
}

type worker struct{}

// NewPool will create a pool of retrievers that you can throw work at.
func NewPool(parallel int) pool {
	d := dispatcher.Start(parallel, worker{}, 0)
	return pool{d}
}

// Retrieve throws download into a pool of workers.
// It will block if all workers are busy.
// Duplicate urls are not checked for.
func (p pool) Retrieve(url, out string) *dispatcher.Result {
	return p.Dispatcher.Dispatch(downloadTask{url, out})
}

func (w worker) Work(t dispatcher.Task) error {
	dt := t.Payload.(downloadTask)
	r, err := manager.ResolveRequest(dt.url)
	if err != nil {
		return err
	}
	f, n, err := r.Download(dt.output)
	if err != nil {
		return err
	}

	t.SetResult(DownloadResult{f, n, r.SDHash})
	return nil
}
