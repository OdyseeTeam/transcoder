package ladder

import (
	"fmt"
	"testing"

	"github.com/floostack/transcoder/ffmpeg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadLadderConfig(t *testing.T) {
	ladder, err := Load(yamlConfig)
	require.NoError(t, err)

	assert.Equal(t, 3500_000, ladder.Tiers[0].VideoBitrate)
	assert.Equal(t, D1080p, ladder.Tiers[0].Definition)
	assert.Equal(t, "bilinear", ladder.Args["sws_flags"])
}

func TestTweak(t *testing.T) {
	ladder, err := Load(yamlConfig)
	require.NoError(t, err)

	testCases := []struct {
		meta          ffmpeg.Metadata
		expectedTiers []Tier
	}{
		{
			generateMeta(720, 480, 5000, FPS30),
			[]Tier{
				{Width: 720, Height: 480, VideoBitrate: nsRate(720, 480), AudioBitrate: "128k", Framerate: 0},
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: 0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "64k", Framerate: 15},
			},
		},
		{
			generateMeta(1920, 1080, 8000, FPS30),
			[]Tier{
				{Width: 1920, Height: 1080, VideoBitrate: 3500_000, AudioBitrate: "160k", Framerate: 0},
				{Width: 1280, Height: 720, VideoBitrate: 2500_000, AudioBitrate: "128k", Framerate: 0},
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: 0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "64k", Framerate: 15},
			},
		},
		{
			generateMeta(800, 600, 3000, FPS30),
			[]Tier{
				{Width: 800, Height: 600, VideoBitrate: nsRate(800, 600), AudioBitrate: "128k", Framerate: 0},
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: 0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "64k", Framerate: 15},
			},
		},
		{
			generateMeta(1920, 1080, 3000, FPS30),
			[]Tier{
				{Width: 1280, Height: 720, VideoBitrate: 2500_000, AudioBitrate: "128k", Framerate: 0},
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: 0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "64k", Framerate: 15},
			},
		},
	}

	for _, tc := range testCases {
		meta := tc.meta
		stream := GetVideoStream(&meta)
		testName := fmt.Sprintf(
			"%vx%v@%vbps",
			stream.GetWidth(),
			stream.GetHeight(),
			meta.GetFormat().GetBitRate(),
		)
		t.Run(testName, func(t *testing.T) {
			m, err := WrapMeta(&tc.meta)
			assert.NoError(t, err)
			l, err := ladder.Tweak(m)
			assert.NoError(t, err)
			for i, tier := range l.Tiers {
				assert.Equal(t, tc.expectedTiers[i].Width, tier.Width, tier)
				assert.Equal(t, tc.expectedTiers[i].Height, tier.Height, tier)
				assert.Equal(t, tc.expectedTiers[i].VideoBitrate, tier.VideoBitrate, tier)
				assert.Equal(t, tc.expectedTiers[i].AudioBitrate, tier.AudioBitrate, tier)
				assert.Equal(t, tc.expectedTiers[i].Framerate, tier.Framerate, tier)

			}
		})
	}
}

var yamlConfig = []byte(`
args:
  sws_flags: bilinear
  profile:v: main
  crf: 23
  refs: 1
  preset: veryfast
  force_key_frames: "expr:gte(t,n_forced*2)"
  hls_time: 6

tiers:
  - definition: 1080p
    bitrate: 3500_000
    bitrate_cutoff: 6000_000
    audio_bitrate: 160k
    width: 1920
    height: 1080
  - definition: 720p
    bitrate: 2500_000
    audio_bitrate: 128k
    width: 1280
    height: 720
  - definition: 360p
    bitrate: 500_000
    audio_bitrate: 96k
    width: 640
    height: 360
  - definition: 144p
    width: 256
    height: 144
    bitrate: 100_000
    audio_bitrate: 64k
    framerate: 15
`)
