package video

import "database/sql"

type Video struct {
	SDHash       string
	CreatedAt    string
	URL          string
	Path         string
	Type         string
	Channel      string
	LastAccessed sql.NullTime
	AccessCount  int64
}

func (v Video) GetPath() string {
	return v.Path
}
