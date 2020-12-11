package video

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/lbryio/transcoder/formats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testDB *sql.DB

func ns(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}

func TestMain(m *testing.M) {
	testDB = OpenDB()

	code := m.Run()

	dbCleanup()
	os.Exit(code)
}

func TestVideoAdd(t *testing.T) {
	v := New(testDB)
	params := AddParams{
		URL:    "what",
		SDHash: "string",
		Type:   formats.TypeHLS,
		Path:   "/tmp/test",
	}
	video, err := v.Add(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, params.URL, video.URL)
	assert.EqualValues(t, params.SDHash, video.SDHash)
	assert.EqualValues(t, params.Type, video.Type)
	assert.EqualValues(t, params.Path, video.Path)

	video, err = v.Add(context.Background(), params)
	require.Error(t, err, "UNIQUE constraint failed")
}

func TestVideoGet(t *testing.T) {
	v := New(testDB)
	params := AddParams{
		URL:    "what",
		SDHash: "string",
		Type:   formats.TypeHLS,
		Path:   "/tmp/test",
	}
	video, err := v.Get(context.Background(), params.SDHash)
	assert.Error(t, err, sql.ErrNoRows)
	assert.Nil(t, video)

	_, err = v.Add(context.Background(), params)
	require.NoError(t, err)

	video, err = v.Get(context.Background(), params.SDHash)
	assert.EqualValues(t, params.URL, video.URL)
	assert.EqualValues(t, params.SDHash, video.SDHash)
	assert.EqualValues(t, params.Type, video.Type)
	assert.EqualValues(t, params.Path, video.Path)
}
