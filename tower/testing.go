package tower

import (
	"database/sql"
	"testing"

	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/tower/queue"

	"github.com/stretchr/testify/require"
)

func CreateTestTowerRPC(t *testing.T, db *sql.DB) *towerRPC {
	tl, err := newTaskList(queue.New(db))
	require.NoError(t, err)
	tower, err := newTowerRPC("amqp://guest:guest@localhost/", tl, zapadapter.NewKV(nil))
	require.NoError(t, err)
	return tower
}
