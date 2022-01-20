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
	orig        *ffmpeg.Metadata
	FPS         float64
	IntFPS      int
	VideoStream transcoder.Streams
	AudioStream transcoder.Streams
}

var fpsPattern = regexp.MustCompile(`^(\d+)/(\d+)$`)

func WrapMeta(origin *ffmpeg.Metadata) (*Metadata, error) {
	m := &Metadata{
		orig: origin,
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

	f, err := m.detectFPS()
	if err != nil {
		return nil, errors.Wrap(err, "cannot determine framerate")
	}
	m.FPS = f
	m.IntFPS = int(math.Ceil(f))

	return m, nil
}

func (m *Metadata) videoStream() transcoder.Streams {
	return GetVideoStream(m.orig)
}

func (m *Metadata) audioStream() transcoder.Streams {
	for _, s := range m.orig.GetStreams() {
		if s.GetCodecType() == "audio" {
			return s
		}
	}
	return nil
}

func (m *Metadata) detectFPS() (float64, error) {
	fpsm := fpsPattern.FindStringSubmatch(m.VideoStream.GetAvgFrameRate())
	if len(fpsm) < 2 {
		return 0, fmt.Errorf("no match found in %s", m.VideoStream.GetAvgFrameRate())
	}
	fpsdd, err := strconv.Atoi(fpsm[1])
	if err != nil {
		return 0, err
	}
	fpsds, err := strconv.Atoi(fpsm[2])
	if err != nil {
		return 0, err
	}
	if fpsds == 0 {
		return 0, errors.New("divisor cannot be zero")
	}
	return float64(fpsdd) / float64(fpsds), nil
}

func GetVideoStream(meta *ffmpeg.Metadata) transcoder.Streams {
	for _, s := range meta.GetStreams() {
		if s.GetCodecType() == "video" {
			return s
		}
	}
	return nil
}
