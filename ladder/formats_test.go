package ladder

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/floostack/transcoder/ffmpeg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTargetFormats(t *testing.T) {
	testInputs := []struct {
		meta   ffmpeg.Metadata
		target []Format
	}{
		{
			generateMeta(720, 480, 5000, FPS30),
			[]Format{H264.CustomFormat(SD360), H264.CustomFormat(SD144), H264.CustomFormat(Resolution{720, 480})},
		},
		{
			generateMeta(1920, 1080, 8000, FPS30),
			[]Format{H264.CustomFormat(HD1080), H264.CustomFormat(HD720), H264.CustomFormat(SD360), H264.CustomFormat(SD144)},
		},
		{
			generateMeta(800, 600, 3000, FPS30),
			[]Format{H264.CustomFormat(SD360), H264.CustomFormat(SD144), H264.CustomFormat(Resolution{800, 600})},
		},
		{
			generateMeta(1920, 1080, 3000, FPS30),
			[]Format{H264.CustomFormat(HD1080), H264.CustomFormat(HD720), H264.CustomFormat(SD360), H264.CustomFormat(SD144)},
		},
	}

	for _, ti := range testInputs {
		meta := ti.meta
		stream := GetVideoStream(&meta)
		testName := fmt.Sprintf(
			"%v x %v x %vbps",
			stream.GetWidth(),
			stream.GetHeight(),
			meta.GetFormat().GetBitRate(),
		)
		t.Run(testName, func(t *testing.T) {
			tf, err := TargetFormats(H264, &meta)
			assert.NoError(t, err)
			assert.Equal(t, ti.target, tf)
		})
	}
}

func TestTargetFormatsErr(t *testing.T) {
	tf, err := TargetFormats(H264, &ffmpeg.Metadata{
		Streams: []ffmpeg.Streams{{CodecType: "audio"}},
	})
	require.Nil(t, tf)
	assert.EqualError(t, err, "no video stream detected")
}

func TestFormat(t *testing.T) {
	f := H264.CustomFormat(HD1080)
	assert.Equal(t, 3300, f.Bitrate.FPS30)
	assert.Equal(t, 5280, f.Bitrate.FPS60)

	f = H264.CustomFormat(Resolution{800, 600})
	assert.Equal(t, 528, f.Bitrate.FPS30)
	assert.Equal(t, 844, f.Bitrate.FPS60)
}

func generateMeta(w, h, br, fr int) ffmpeg.Metadata {
	meta := ffmpeg.Metadata{
		Format:  ffmpeg.Format{BitRate: strconv.Itoa(br * 1000)},
		Streams: []ffmpeg.Streams{{CodecType: "audio"}, {CodecType: "video", BitRate: strconv.Itoa(br * 1000), Index: 0, Width: w, Height: h, AvgFrameRate: fmt.Sprintf("%v/1", fr)}},
	}
	return meta
}
