package tower

import (
	"path"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/stretchr/testify/require"
)

func TestStateDumpNew(t *testing.T) {
	file := path.Join(t.TempDir(), "state.json")
	state, err := NewState(file, false)
	require.NoError(t, err)

	state.Requests = map[string]*RunningRequest{
		randomdata.Alphanumeric(96): {
			URL:       "lbry://" + randomdata.SillyName(),
			SDHash:    randomdata.Alphanumeric(96),
			Stage:     StageEncoding,
			TsStarted: time.Now().Add(-3 * time.Minute).Truncate(time.Millisecond),
		},
	}

	err = state.Dump()
	require.NoError(t, err)

	restored, err := NewState(file, true)
	require.NoError(t, err)
	require.EqualValues(t, state.Requests, restored.Requests)
}

func TestStateBlank(t *testing.T) {
	file := path.Join(t.TempDir(), "state.json")
	state, err := NewState(file, false)
	require.NoError(t, err)

	err = state.Dump()
	require.NoError(t, err)

	state.Requests[randomdata.Alphanumeric(96)] = &RunningRequest{}
}

func TestRunningRequest_TimedOut(t *testing.T) {
	type fields struct {
		TsStarted   time.Time
		TsUpdated   time.Time
		TsHeartbeat time.Time
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			"updated 15 minutes ago",
			fields{time.Now().Add(-65 * time.Minute), time.Now().Add(-15 * time.Minute), time.Now().Add(-1 * time.Minute)},
			false,
		},
		{
			"updated 35 minutes ago",
			fields{time.Now().Add(-65 * time.Minute), time.Now().Add(-35 * time.Minute), time.Now().Add(-10 * time.Second)},
			true,
		},
		{
			"updated 3 minutes ago",
			fields{time.Now().Add(-65 * time.Minute), time.Now().Add(-3 * time.Minute), time.Now().Add(-1 * time.Minute)},
			false,
		},
		{
			"no heartbeat received yet",
			fields{time.Now().Add(-1 * time.Minute), time.Now().Add(-1 * time.Minute), time.Time{}},
			false,
		},
		{
			"heartbeat received too long ago",
			fields{time.Now().Add(-100 * time.Minute), time.Now().Add(-5 * time.Minute), time.Now().Add(-5 * time.Minute)},
			true,
		},
		{
			"no timestamps",
			fields{time.Time{}, time.Time{}, time.Time{}},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RunningRequest{
				TsStarted:   tt.fields.TsStarted,
				TsHeartbeat: tt.fields.TsHeartbeat,
				TsUpdated:   tt.fields.TsUpdated,
			}
			if got := r.TimedOut(1 * time.Minute); got != tt.want {
				t.Errorf("RunningRequest.TimedOut() = %v, want %v", got, tt.want)
			}
		})
	}
}
