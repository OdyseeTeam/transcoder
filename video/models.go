package video

import (
	"database/sql"
)

type Video struct {
	SDHash     string
	CreatedAt  string
	URL        string
	Path       string
	RemotePath string
	Type       string
	Channel    string

	LastAccessed sql.NullTime
	AccessCount  int64

	Size     int64
	Checksum string
}

// GetLocation returns a video location suitable for using in HTTP redirect response.
// Bool in return value signifies if it's a remote location (S3) or local (relative HTTP path).
func (v Video) GetLocation() (string, bool) {
	if v.Path != "" {
		return v.Path, false
	}
	return v.RemotePath, true
}

func (v Video) GetSize() int64 {
	return v.Size
}

func (v Video) GetWeight() int64 {
	return v.LastAccessed.Time.Unix()
}
