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
}

func (v Video) GetPath() string {
	return v.Path
}
