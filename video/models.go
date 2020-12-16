package video

type Video struct {
	SDHash    string
	CreatedAt string
	URL       string
	Path      string
	Type      string
}

func (v Video) GetPath() string {
	return v.Path
}
