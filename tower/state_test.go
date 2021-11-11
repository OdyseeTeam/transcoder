package tower

import (
	"path"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/stretchr/testify/require"
)

func TestRunningRequest(t *testing.T) {
	file := path.Join(t.TempDir(), "state.json")
	state := State{Requests: map[string]*RunningRequest{
		randomdata.Alphanumeric(96): {
			URL:       "lbry://" + randomdata.SillyName(),
			SDHash:    randomdata.Alphanumeric(96),
			Stage:     StageEncoding,
			TsStarted: time.Now().Add(-3 * time.Minute).Truncate(time.Millisecond),
		},
	}}
	err := SaveState(&state, file)
	require.NoError(t, err)
	restored, err := RestoreState(file)
	require.NoError(t, err)
	require.EqualValues(t, state.Requests, restored.Requests)
}
