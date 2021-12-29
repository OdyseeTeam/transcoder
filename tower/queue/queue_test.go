package queue

import (
	"context"
	"testing"

	"github.com/Pallinder/go-randomdata"
	"github.com/stretchr/testify/require"
)

func TestConnect(t *testing.T) {
	db, teardown, err := CreateTestDB()
	require.NoError(t, err)
	defer teardown()
	q := New(db)
	task, err := q.CreateTask(context.Background(), CreateTaskParams{
		ULID:   randomdata.Alphanumeric(36),
		Worker: randomdata.Alphanumeric(12),
		URL:    randomdata.Alphanumeric(28),
		SDHash: randomdata.Alphanumeric(96),
	})
	require.NoError(t, err)
	task2, err := q.GetTask(context.Background(), task.ULID)
	require.NoError(t, err)
	require.EqualValues(t, task, task2)
}
