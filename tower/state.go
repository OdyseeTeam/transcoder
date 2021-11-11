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
	URL,
	SDHash,
	WorkerID,
	Ref string

	Stage    RequestStage
	Progress float32

	TsStarted, TsHeartbeat, TsUpdated time.Time

	Error string

	CallbackToken string
	Uploaded      bool

	FailedAttempts int

	transcodingRequest *manager.TranscodingRequest
}

type State struct {
	lock     sync.RWMutex
	Requests map[string]*RunningRequest
}

func RestoreState(file string) (*State, error) {
	state := State{}
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	state.lock = sync.RWMutex{}
	return &state, nil
}

func SaveState(state *State, file string) error {
	data, err := json.Marshal(&state)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(file, data, os.ModePerm); err != nil {
		return err
	}
	return nil
}

func (r *RunningRequest) TimedOut() bool {
	if r.TsStarted.IsZero() {
		return false
	}

	requestAge := time.Since(r.TsStarted)
	if r.TsUpdated.IsZero() && requestAge > 10*defaultHeartbeatInterval {
		return true
	}

	updateAge := time.Since(r.TsUpdated)
	if updateAge > 10*defaultHeartbeatInterval {
		return true
	}

	if r.TsHeartbeat.IsZero() && requestAge > 5*defaultHeartbeatInterval {
		return true
	}

	heartbeatAge := time.Since(r.TsHeartbeat)
	if heartbeatAge > 5*defaultHeartbeatInterval {
		return true
	}

	return false
}
