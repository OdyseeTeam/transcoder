package ladder

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/floostack/transcoder/ffmpeg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadLadderConfig(t *testing.T) {
	ladder, err := Load(defaultLadderYaml)
	require.NoError(t, err)

	assert.Equal(t, 3500_000, ladder.Tiers[0].VideoBitrate)
	assert.Equal(t, D1080p, ladder.Tiers[0].Definition)
	assert.Equal(t, "bilinear", ladder.Args["sws_flags"])
}

func TestTweak(t *testing.T) {
	ladder, err := Load(defaultLadderYaml)
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
		{
			generateMeta(3840, 2160, 20000, FPS30),
			[]Tier{
				{Width: 1920, Height: 1080, VideoBitrate: 3500_000, AudioBitrate: "160k", Framerate: 0},
				{Width: 1280, Height: 720, VideoBitrate: 2500_000, AudioBitrate: "128k", Framerate: 0},
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: 0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "64k", Framerate: 15},
			},
		},
		{
			generateMeta(100, 50, 110, FPS30),
			[]Tier{},
		},
		{
			generateMeta(1080, 1920, 3000, FPS30),
			[]Tier{
				{Width: 720, Height: 1280, VideoBitrate: 2500_000, AudioBitrate: "128k", Framerate: 0},
				{Width: 360, Height: 640, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: 0},
				{Width: 144, Height: 256, VideoBitrate: 100_000, AudioBitrate: "64k", Framerate: 15},
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
			assert.NoError(t, err)
			m, err := WrapMeta(&tc.meta)
			assert.NoError(t, err)
			l, err := ladder.Tweak(m)
			assert.NoError(t, err)
			assert.Equal(t, len(tc.expectedTiers), len(l.Tiers), l.Tiers)
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

func generateMeta(w, h, br, fr int) ffmpeg.Metadata {
	meta := ffmpeg.Metadata{
		Format:  ffmpeg.Format{BitRate: strconv.Itoa(br * 1000)},
		Streams: []ffmpeg.Streams{{CodecType: "audio"}, {CodecType: "video", BitRate: strconv.Itoa(br * 1000), Index: 0, Width: w, Height: h, AvgFrameRate: fmt.Sprintf("%v/1", fr)}},
	}
	return meta
}
