package uploader

import (
	"github.com/lbryio/transcoder/pkg/dispatcher"
)

const maxAttempts = 10

type uploadPool struct {
	dispatcher.Dispatcher
}

type uploadWorker struct {
}

type uploadTask struct {
	path, url string
	attempt   int
}

// NewPool will create a pool of uploaders that you can throw work at.
func NewPool(parallel int) uploadPool {
	d := dispatcher.Start(parallel, uploadWorker{}, 0)
	return uploadPool{d}
}

// Upload throws download into a pool of workers.
// It will block if all workers are busy.
// Duplicate urls are not checked for.
func (p uploadPool) Upload(path, url string) *dispatcher.Result {
	return p.Dispatcher.Dispatch(uploadTask{
		path: path,
		url:  url,
	})
}

func (w uploadWorker) Work(t dispatcher.Task) error {
	ut := t.Payload.(uploadTask)
	err := Upload(ut.path, ut.url)
	t.SetResult(struct{}{})
	return err
}
