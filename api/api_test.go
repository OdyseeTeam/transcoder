package api

import (
	"testing"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/formats"
	"github.com/lbryio/transcoder/queue"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/video"

	"github.com/stretchr/testify/assert"
)

func TestGetVideoOrCreateTask(t *testing.T) {
	vdb := db.OpenTestDB()
	vdb.MigrateUp(video.InitialMigration)

	qdb := db.OpenTestDB()
	qdb.MigrateUp(queue.InitialMigration)

	lib := video.NewLibrary(video.Configure().LocalStorage(storage.Local("/tmp/test")).DB(vdb))
	q := queue.NewQueue(qdb)

	m := NewManager(q, lib)
	_, err := m.GetVideoOrCreateTask("lbry://nonexistsaotsaotihasoihfa", formats.TypeHLS)
	assert.EqualError(t, err, "could not resolve stream URI")
}
