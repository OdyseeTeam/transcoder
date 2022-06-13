package tasks

import (
	"encoding/json"

	"github.com/lbryio/transcoder/library"
)

type TranscodingRequest struct {
	URL    string `json:"url"`
	SDHash string `json:"sd_hash"`
}

type TranscodingResult struct {
	Stream *library.Stream `json:"stream"`
}

func (m TranscodingRequest) String() string {
	out, _ := json.Marshal(m)
	return string(out)
}

func (m *TranscodingRequest) FromString(s string) error {
	return json.Unmarshal([]byte(s), m)
}

func (m TranscodingResult) String() string {
	out, _ := json.Marshal(m)
	return string(out)
}

func (m *TranscodingResult) FromString(s string) error {
	return json.Unmarshal([]byte(s), m)
}
