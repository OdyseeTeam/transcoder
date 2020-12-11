package api

import (
	"context"
	"database/sql"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/queue"
	"github.com/lbryio/transcoder/video"
)

const (
	videoPlaylistPath = "."
)

// GetVideoOrCreateTask checks if video exists in the library or is waiting in the queue.
// If neither, it validates and adds video for later processing.
func GetVideoOrCreateTask(uri, sdHash, kind string) (*video.Video, error) {
	videoDB := video.New(db.OpenDB("video.sqlite"))
	queueDB := queue.NewQueue(db.OpenDB("queue.sqlite"))

	v, err := videoDB.Get(context.Background(), sdHash)
	if v == nil || err == sql.ErrNoRows {
		t, err := queueDB.GetBySDHash(sdHash)
		if err != nil {
			return nil, err
		}
		if t != nil {
			return nil, video.ErrTranscodingUnderway
		}

		_, err = video.ValidateIncomingVideo(uri)
		if err != nil {
			return nil, err
		}
		_, err = queueDB.Add(uri, sdHash, kind)
		if err != nil {
			return nil, err
		}
		return nil, video.ErrTranscodingUnderway
	}
	return v, nil
}
