package formats

import (
	"fmt"
	"testing"

	"github.com/floostack/transcoder/ffmpeg"
	"github.com/stretchr/testify/assert"
)

func TestTargetFormats(t *testing.T) {
	testInputs := []struct {
		meta   ffmpeg.Metadata
		target []Format
	}{
		{
			generateMeta(720, 480, 5000, FPS30),
			[]Format{H264.Format(SD480), H264.Format(SD360)},
		},
		{
			generateMeta(1920, 1080, 8000, FPS30),
			[]Format{H264.Format(HD1080), H264.Format(HD720), H264.Format(SD480), H264.Format(SD360)},
		},
		{
			generateMeta(800, 600, 3000, FPS30),
			[]Format{H264.Format(SD480), H264.Format(SD360), H264.Format(Resolution{800, 600})},
		},
		{
			generateMeta(1920, 1080, 3000, FPS30),
			[]Format{H264.Format(HD720), H264.Format(SD480), H264.Format(SD360)},
		},
	}

	for _, ti := range testInputs {
		meta := ti.meta
		stream := meta.GetStreams()[0]
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

func generateMeta(w, h, br, fr int) ffmpeg.Metadata {
	return ffmpeg.Metadata{
		Format:  ffmpeg.Format{BitRate: fmt.Sprintf("%v", br*1000)},
		Streams: []ffmpeg.Streams{{Index: 0, Width: w, Height: h, AvgFrameRate: fmt.Sprintf("%v/1", fr)}},
	}
}
