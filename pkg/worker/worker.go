package worker

import (
	"errors"
	"time"
)

var FatalError = errors.New("workload error")
var ErrShutdown = errors.New("worker shut down")

type Ticker struct {
	Interval time.Duration
	workload Workload
	stop     chan bool
}

type Workload interface {
	Process() error
	Shutdown()
	IsShutdown() bool
}

func NewTicker(l Workload, i time.Duration) *Ticker {
	w := &Ticker{Interval: i, workload: l}
	return w
}

func (w *Ticker) Stop() {
	w.stop <- true
}

func (w *Ticker) Start() {
	ticker := time.NewTicker(w.Interval)

	go func() {
		for {
			select {
			case <-w.stop:
				ticker.Stop()
				return
			case <-ticker.C:
				err := w.workload.Process()
				if errors.Is(err, FatalError) || errors.Is(err, ErrShutdown) {
					logger.Errorw("workload had a fatal error", "err", err)
					w.Stop()
					go func() {
						w.workload.Shutdown()
					}()
				} else if err != nil {
					logger.Errorw("workload errored", "err", err)
				}
			}
		}
	}()
}
