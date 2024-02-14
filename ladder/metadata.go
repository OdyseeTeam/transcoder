package ladder

import (
	"fmt"
	"math"
	"regexp"
	"strconv"

	"github.com/floostack/transcoder"
	"github.com/floostack/transcoder/ffmpeg"

	"github.com/pkg/errors"
)

type Metadata struct {
	FMeta       *ffmpeg.Metadata
	FPS         *FPS
	FastStart   bool
	VideoStream transcoder.Streams
	AudioStream transcoder.Streams
}

type FPS struct {
	Ratio string
	Float float64
}

var fpsPattern = regexp.MustCompile(`^(\d+)/(\d+)$`)

func WrapMeta(fmeta *ffmpeg.Metadata) (*Metadata, error) {
	m := &Metadata{
		FMeta: fmeta,
	}
	vs := m.videoStream()
	if vs == nil {
		return nil, errors.New("no video stream found")
	}
	m.VideoStream = vs
	as := m.videoStream()
	if as == nil {
		return nil, errors.New("no audio stream found")
	}
	m.AudioStream = as

	f, err := m.determineFramerate()
	if err != nil {
		return nil, errors.Wrap(err, "cannot determine framerate")
	}
	m.FPS = f

	return m, nil
}

func (m *Metadata) videoStream() transcoder.Streams {
	return GetVideoStream(m.FMeta)
}

func (m *Metadata) audioStream() transcoder.Streams {
	for _, s := range m.FMeta.GetStreams() {
		if s.GetCodecType() == "audio" {
			return s
		}
	}
	return nil
}

func (m *Metadata) determineFramerate() (*FPS, error) {
	fr := m.VideoStream.GetAvgFrameRate()
	fpsm := fpsPattern.FindStringSubmatch(fr)
	if len(fpsm) < 2 {
		return nil, fmt.Errorf("no match found in %s", fr)
	}
	fpsdd, err := strconv.Atoi(fpsm[1])
	if err != nil {
		return nil, err
	}
	fpsds, err := strconv.Atoi(fpsm[2])
	if err != nil {
		return nil, err
	}
	if fpsds == 0 {
		return nil, errors.New("divisor cannot be zero")
	}
	return &FPS{Ratio: fr, Float: float64(fpsdd) / float64(fpsds)}, nil
}

func (f FPS) Int() int {
	return int(math.Ceil(f.Float))
}

func (f FPS) String() string {
	return f.Ratio
}
func GetVideoStream(meta *ffmpeg.Metadata) transcoder.Streams {
	for _, s := range meta.GetStreams() {
		if s.GetCodecType() == "video" {
			return s
		}
	}
	return nil
}
