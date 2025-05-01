package encoder

import "github.com/OdyseeTeam/transcoder/pkg/dispatcher"

type encodeTask [2]string

type pool struct {
	dispatcher.Dispatcher
}

type worker struct {
	encoder Encoder
}

func (w worker) Work(t dispatcher.Task) error {
	et := t.Payload.(encodeTask)
	res, err := w.encoder.Encode(et[0], et[1])
	t.SetResult(res)
	return err
}

// NewPool will create a pool of encoders that you can throw work at.
func NewPool(encoder Encoder, parallel int) pool {
	d := dispatcher.Start(parallel, worker{encoder}, 0)
	return pool{d}
}

// Encode throws encoding task into a pool of workers.
// It works slightly different from encoder.Encode but the result should eventually be the same.
// For how to obtain encoding progress, see poolSuite.TestEncode.
func (p pool) Encode(in, out string) *dispatcher.Result {
	return p.Dispatch(encodeTask{in, out})
}
