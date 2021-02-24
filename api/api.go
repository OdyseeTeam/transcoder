package api

import (
	"database/sql"
	"errors"

	"github.com/lbryio/transcoder/pkg/claim"
	"github.com/lbryio/transcoder/queue"
	"github.com/lbryio/transcoder/video"
)

const (
	videoPlaylistPath = "."
)

type Queue interface {
	Add(url, sdHash, _type string) (*Task, error)
	GetBySDHash(sdHash string) (*Task, error)
}

type Library interface {
	Add(url, sdHash, _type string) (*Video, error)
	Get(string) (*Video, error)
}

type Video interface {
	GetLocation() (location string, external bool)
}

type Task interface {
}

type VideoManager struct {
	// queue   Queue
	// library Library
	queue   *queue.Queue
	library *video.Library
}

func NewManager(q *queue.Queue, l *video.Library) *VideoManager {
	m := &VideoManager{
		queue:   q,
		library: l,
	}
	return m
}

// GetVideoOrCreateTask checks if video exists in the library or is waiting in the queue.
// If neither, it validates and adds video for later processing.
func (m *VideoManager) GetVideoOrCreateTask(uri, kind string) (Video, error) {
	claim, err := claim.Resolve(uri)
	v, err := m.library.Get(claim.SDHash)
	if v == nil || err == sql.ErrNoRows {
		err := video.ValidateByClaim(claim)
		if err != nil {
			if errors.Is(err, video.ErrChannelNotEnabled) {
				m.library.IncViews(claim.PermanentURL, claim.SDHash)
			}
			return nil, err
		}

		t, err := m.queue.GetBySDHash(claim.SDHash)
		if err != nil {
			return nil, err
		}
		if t != nil {
			return nil, video.ErrTranscodingUnderway
		}
		_, err = m.queue.Add(uri, claim.SDHash, kind)
		if err != nil {
			return nil, err
		}
		return nil, video.ErrTranscodingUnderway
	}
	return *v, nil
}
