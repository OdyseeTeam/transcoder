package tower

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/lbryio/transcoder/manager"
)

type RunningRequest struct {
	Ref,
	URL,
	SDHash,
	WorkerID,
	Channel string

	Stage    RequestStage
	Progress float32

	TsCreated, TsStarted, TsHeartbeat, TsUpdated time.Time

	Error string

	CallbackToken string
	Uploaded      bool

	FailedAttempts int

	transcodingRequest *manager.TranscodingRequest
}

type State struct {
	Requests map[string]*RunningRequest
	lock     sync.RWMutex
	file     string
	stopChan chan struct{}
}

func NewState(file string, load bool) (*State, error) {
	s := &State{
		lock: sync.RWMutex{},
		file: file,
	}

	if load {
		err := s.Load(file)
		if err != nil {
			return nil, err
		}
	}
	if s.Requests == nil {
		s.Requests = map[string]*RunningRequest{}
	}
	return s, nil
}

func (s *State) Load(file string) error {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	s.lock = sync.RWMutex{}
	s.file = file
	return nil
}

func (s *State) Dump() error {
	s.lock.RLock()
	defer s.lock.RUnlock()
	if s.file == "" {
		return nil
	}
	data, err := json.Marshal(&s)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(s.file, data, os.ModePerm); err != nil {
		return err
	}
	return nil
}

func (s *State) StartDump() {
	pulse := time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-s.stopChan:
				s.Dump()
				return
			case <-pulse.C:
				s.Dump()
			}
		}
	}()
}

func (s *State) StopDump() {
	close(s.stopChan)
}

func (r *RunningRequest) TimedOut(base time.Duration) bool {
	if r.TsStarted.IsZero() {
		return false
	}

	// First updates should come as soon as worker picks up the request
	requestAge := time.Since(r.TsStarted)
	if r.TsUpdated.IsZero() && requestAge > 5*base {
		return true
	}

	// Downloads might take long but 30 minutes is a resonable time to wait for it
	// TODO: Lower when download status report is implemented
	updateAge := time.Since(r.TsUpdated)
	if updateAge > 30*base {
		return true
	}

	// Heartbeats should start coming right after worker picks up the request, if there's none
	// or not a recent one, it's a sign of anomaly
	if r.TsHeartbeat.IsZero() && requestAge > 2*base {
		return true
	}

	heartbeatAge := time.Since(r.TsHeartbeat)
	if !r.TsHeartbeat.IsZero() && heartbeatAge > 2*base {
		return true
	}

	return false
}
