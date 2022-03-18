package tower

import (
	"database/sql"
	"testing"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/pkg/migrator"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/tower/queue"

	"github.com/stretchr/testify/require"
)

type TowerTestHelper struct {
	TowerDB        *sql.DB
	TowerDBCleanup migrator.TestDBCleanup
}

func (h *TowerTestHelper) SetupTowerDB() error {
	db, dbCleanup, err := migrator.CreateTestDB(queue.MigrationsFS)
	if err != nil {
		return err
	}
	h.TowerDB = db
	h.TowerDBCleanup = dbCleanup
	return nil
}

func (h *TowerTestHelper) TearDownTowerDB() error {
	db, dbCleanup, err := migrator.CreateTestDB(queue.MigrationsFS)
	if err != nil {
		return err
	}
	h.TowerDB = db
	h.TowerDBCleanup = dbCleanup
	return nil
}

func NewTestTowerRPC(t *testing.T, db *sql.DB) *towerRPC {
	tl, err := newTaskList(queue.New(db))
	require.NoError(t, err)
	tower, err := newTowerRPC("amqp://guest:guest@localhost/", tl, zapadapter.NewKV(nil))
	require.NoError(t, err)
	return tower
}

func NewTestTowerLite(t *testing.T, storage *storage.S3Driver, mgr *manager.VideoManager) (*ServerLite, error) {
	logger := zapadapter.NewKV(nil)

	enc, err := encoder.NewEncoder(encoder.Configure().Log(logger))
	if err != nil {
		return nil, err
	}
	processor, err := newPipeline(t.TempDir(), "test-worker", storage, enc, logger)
	if err != nil {
		return nil, err
	}

	s, err := NewServerLite(DefaultServerConfig().
		Logger(logger).
		HttpServer(":18080", "http://localhost:18080").
		VideoManager(mgr).
		// WorkDir(srvWorkDir).
		DevMode(),
		processor,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}
