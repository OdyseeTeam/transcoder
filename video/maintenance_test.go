package video

import (
	"testing"
	"time"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/queue"
	"github.com/lbryio/transcoder/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpawnPopularQueuing(t *testing.T) {
	vdb := db.OpenTestDB()
	vdb.MigrateUp(InitialMigration)

	qdb := db.OpenTestDB()
	qdb.MigrateUp(queue.InitialMigration)

	lib := NewLibrary(Configure().LocalStorage(storage.Local("/tmp/test")).DB(vdb))
	q := queue.NewQueue(qdb)

	vids := []struct{ url, sdHash string }{
		{"abc", "asdasda"}, {"cde", "sdagkkj"}, {"def", "asuyuia"},
		{"ghi", "ewury"}, {"jkl", "mdaslkdj"}, {"mno", "dasldyqwaiuy"},
	}

	for range [1000]int{} {
		lib.IncViews(vids[0].url, vids[0].sdHash)
	}
	for range [250]int{} {
		lib.IncViews(vids[1].url, vids[1].sdHash)
	}
	for range [13250]int{} {
		lib.IncViews(vids[4].url, vids[4].sdHash)
	}
	lib.IncViews(vids[2].url, vids[2].sdHash)
	lib.IncViews(vids[3].url, vids[3].sdHash)
	lib.IncViews(vids[5].url, vids[5].sdHash)

	stop := SpawnPopularQueuing(lib, q, PopularQueuingOpts{TopNumber: 3, Interval: 100 * time.Millisecond, LowerBound: 100})
	time.Sleep(1 * time.Second)
	stop <- true

	ts, err := q.List()
	require.NoError(t, err)
	assert.Len(t, ts, 3)
	assert.Equal(t, vids[4].url, ts[0].URL)
	assert.Equal(t, vids[4].sdHash, ts[0].SDHash)
	assert.Equal(t, vids[0].url, ts[1].URL)
	assert.Equal(t, vids[0].sdHash, ts[1].SDHash)
	assert.Equal(t, vids[1].url, ts[2].URL)
	assert.Equal(t, vids[1].sdHash, ts[2].SDHash)
}
