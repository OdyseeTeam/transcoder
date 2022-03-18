package tower

import (
	"errors"
)

type ServerLite struct {
	Server
	processor Processor
}

func NewServerLite(config *ServerConfig, processor Processor) (*ServerLite, error) {
	s := ServerLite{
		Server: Server{
			ServerConfig: config,
			stopChan:     make(chan struct{}),
		},
		processor: processor,
	}

	return &s, nil
}

func (s *ServerLite) StartAll() error {
	if s.videoManager == nil {
		return errors.New("VideoManager is not configured")
	}

	go func() {
		for {
			select {
			case trReq := <-s.videoManager.Requests():
				var mtt *MsgTranscodingTask
				mtt = &MsgTranscodingTask{
					URL:    trReq.URI,
					SDHash: trReq.SDHash,
				}
				at := &activeTask{
					payload:  make(chan MsgTranscodingTask),
					progress: make(chan MsgWorkerProgress),
					errors:   make(chan MsgWorkerError),
					success:  make(chan MsgWorkerSuccess),
				}
				wt := createWorkerTask(*mtt)
				s.log.Info("new task received", "payload", mtt)
				s.processor.Process(wt)
				go s.manageTask(at)
				for {
					select {
					case p := <-wt.progress:
						at.progress <- MsgWorkerProgress{
							Stage:   p.Stage,
							Percent: p.Percent,
						}
					case te := <-wt.errors:
						at.errors <- MsgWorkerError{
							Error: te.err.Error(),
							Fatal: te.fatal,
						}
					case r := <-wt.result:
						at.success <- MsgWorkerSuccess{RemoteStream: r.remoteStream}
					case <-s.stopChan:
						s.log.Info("worker exiting")
						at.errors <- MsgWorkerError{Error: "worker exiting"}
					}
				}
			case <-s.stopChan:
				return
			}
		}
	}()
	if err := s.startHttpServer(); err != nil {
		return err
	}
	return nil
}
