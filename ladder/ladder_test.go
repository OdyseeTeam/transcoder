package ladder

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/floostack/transcoder/ffmpeg"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	FPS0  = decimal.NewFromInt(0)
	FPS15 = decimal.NewFromInt(15)
)

var baseExpectedArgs = []string{
	"-preset veryfast",
	"-refs 1",
	"-hls_playlist_type vod",
	"-pix_fmt yuv420p",
	"-c:a aac",
	"-strftime_mkdir 1",
	"-ac 2",
	"-master_pl_name master.m3u8",
	"-force_key_frames expr:gte(t,n_forced*2)",
	"-sws_flags bilinear",
	"-hls_segment_filename v%v_s%06d.ts",
	"-strftime_mkdir 1",
	"-sc_threshold 0",
	"-f hls",
	"-hls_flags independent_segments",
	"-profile:v main",
	"-c:v libx264",
}

func TestLoadLadderConfig(t *testing.T) {
	ladder, err := Load(defaultLadderYaml)
	require.NoError(t, err)

	assert.Equal(t, 3500_000, ladder.Tiers[0].VideoBitrate)
	assert.Equal(t, D1080p, ladder.Tiers[0].Definition)
	assert.Equal(t, "bilinear", ladder.Args["sws_flags"])
}

func TestTweak(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	ladder, err := Load(defaultLadderYaml)
	require.NoError(err)

	testCases := []struct {
		metadata      ffmpeg.Metadata
		expectedTiers []Tier
		expectedArgs  []string
	}{
		{
			metadata: generateMeta(640, 360, 5000, "30/1"),
			expectedTiers: []Tier{
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: FPS0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "96k", Framerate: FPS15},
			},
			expectedArgs: []string{
				"-var_stream_map v:0,a:0 v:1,a:1",
				// map v:0 must be present before every tier declaration, argument order (within the substring) is important
				"-map v:0 -filter:v:0 scale=-2:360",
				"-b:v:0 500000",
				"-crf:v:0 25",
				"-maxrate:v:0 500000",
				"-bufsize:v:0 500000",
				"-r:v:0 30/1",
				"-g:v:0 60",
				// Here again we are mapping video stream #0 to our transcoded tier
				"-map v:0 -filter:v:1 scale=-2:144",
				"-b:v:1 100000",
				"-crf:v:1 26",
				"-maxrate:v:1 100000",
				"-bufsize:v:1 100000",
				"-r:v:1 15",
				"-g:v:1 30",
				// Same goes for audio streams
				"-map a:0 -b:a:0 96k",
				"-map a:0 -b:a:1 96k",
			},
		},
		{
			metadata: generateMeta(720, 480, 5000, "30/1"),
			expectedTiers: []Tier{
				{Width: 720, Height: 480, VideoBitrate: nsRate(720, 480), AudioBitrate: "128k", Framerate: FPS0},
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: FPS0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "96k", Framerate: FPS15},
			},
			expectedArgs: []string{
				"-var_stream_map v:0,a:0 v:1,a:1 v:2,a:2",
				"-map v:0 -filter:v:0 scale=-2:480",
				"-b:v:0 1297298",
				"-crf:v:0 24",
				"-maxrate:v:0 1297298",
				"-bufsize:v:0 1297298",
				"-r:v:0 30/1",
				"-g:v:0 60",
				"-map v:0 -filter:v:1 scale=-2:360",
				"-crf:v:1 25",
				"-b:v:1 500000",
				"-maxrate:v:1 500000",
				"-bufsize:v:1 500000",
				"-r:v:1 30/1",
				"-g:v:1 60",
				"-map v:0 -filter:v:2 scale=-2:144",
				"-b:v:2 100000",
				"-crf:v:2 26",
				"-maxrate:v:2 100000",
				"-bufsize:v:2 100000",
				"-r:v:2 15",
				"-g:v:2 30",
				"-map a:0 -b:a:0 128k",
				"-map a:0 -b:a:1 96k",
				"-map a:0 -b:a:2 96k",
			},
		},
		{
			metadata: generateMeta(720, 480, 5000, "189941760/7981033"),
			expectedTiers: []Tier{
				{Width: 720, Height: 480, VideoBitrate: nsRate(720, 480), AudioBitrate: "128k", Framerate: FPS0},
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: FPS0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "96k", Framerate: FPS15},
			},
			expectedArgs: []string{
				"-var_stream_map v:0,a:0 v:1,a:1 v:2,a:2",
				"-map v:0 -filter:v:0 scale=-2:480",
				"-b:v:0 1297298",
				"-crf:v:0 24",
				"-maxrate:v:0 1297298",
				"-bufsize:v:0 1297298",
				"-r:v:0 189941760/7981033",
				"-g:v:0 48",
				"-map v:0 -filter:v:1 scale=-2:360",
				"-b:v:1 500000",
				"-crf:v:1 25",
				"-maxrate:v:1 500000",
				"-bufsize:v:1 500000",
				"-r:v:1 189941760/7981033",
				"-g:v:1 48",
				"-map v:0 -filter:v:2 scale=-2:144",
				"-b:v:2 100000",
				"-crf:v:2 26",
				"-maxrate:v:2 100000",
				"-bufsize:v:2 100000",
				"-r:v:2 15",
				"-g:v:2 30",
				"-map a:0 -b:a:0 128k",
				"-map a:0 -b:a:1 96k",
				"-map a:0 -b:a:2 96k",
			},
		},
		{
			metadata: generateMeta(1920, 1080, 8000, "30/1"),
			expectedTiers: []Tier{
				{Width: 1920, Height: 1080, VideoBitrate: 3500_000, AudioBitrate: "160k", Framerate: FPS0},
				{Width: 1280, Height: 720, VideoBitrate: 2500_000, AudioBitrate: "128k", Framerate: FPS0},
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: FPS0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "96k", Framerate: FPS15},
			},
		},
		{
			metadata: generateMeta(1920, 1080, 5000, "30/1"),
			expectedTiers: []Tier{
				{Width: 1920, Height: 1080, VideoBitrate: 3500_000, AudioBitrate: "160k", Framerate: FPS0},
				{Width: 1280, Height: 720, VideoBitrate: 2500_000, AudioBitrate: "128k", Framerate: FPS0},
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: FPS0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "96k", Framerate: FPS15},
			},
		},
		{
			metadata: generateMeta(800, 600, 3000, "30/1"),
			expectedTiers: []Tier{
				{Width: 800, Height: 600, VideoBitrate: nsRate(800, 600), AudioBitrate: "128k", Framerate: FPS0},
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: FPS0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "96k", Framerate: FPS15},
			},
		},
		{
			metadata: generateMeta(1920, 1080, 3000, "30/1"),
			expectedTiers: []Tier{
				{Width: 1920, Height: 1080, VideoBitrate: 3500_000, AudioBitrate: "160k", Framerate: FPS0},
				{Width: 1280, Height: 720, VideoBitrate: 2500_000, AudioBitrate: "128k", Framerate: FPS0},
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: FPS0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "96k", Framerate: FPS15},
			},
		},
		{
			metadata: generateMeta(3840, 2160, 20000, "30/1"),
			expectedTiers: []Tier{
				{Width: 1920, Height: 1080, VideoBitrate: 3500_000, AudioBitrate: "160k", Framerate: FPS0},
				{Width: 1280, Height: 720, VideoBitrate: 2500_000, AudioBitrate: "128k", Framerate: FPS0},
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: FPS0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "96k", Framerate: FPS15},
			},
		},
		{
			metadata:      generateMeta(100, 50, 110, "30/1"),
			expectedTiers: []Tier{},
		},
		{
			metadata: generateMeta(1080, 1920, 3000, "30/1"),
			expectedTiers: []Tier{
				{Width: 1080, Height: 1920, VideoBitrate: 3500_000, AudioBitrate: "160k", Framerate: FPS0},
				{Width: 720, Height: 1280, VideoBitrate: 2500_000, AudioBitrate: "128k", Framerate: FPS0},
				{Width: 360, Height: 640, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: FPS0},
				{Width: 144, Height: 256, VideoBitrate: 100_000, AudioBitrate: "96k", Framerate: FPS15},
			},
		},
	}

	for _, c := range testCases {
		testCase := c
		stream := GetVideoStream(&testCase.metadata)
		testName := fmt.Sprintf(
			"%vx%v@%vbps@%vfps",
			stream.GetWidth(),
			stream.GetHeight(),
			c.metadata.GetFormat().GetBitRate(),
			c.metadata.Streams[1].AvgFrameRate,
		)
		t.Run(testName, func(t *testing.T) {
			assert.NoError(err)
			m, err := WrapMeta(&testCase.metadata)
			assert.NoError(err)
			newLadder, err := ladder.Tweak(m)
			assert.NoError(err)
			require.Equal(len(testCase.expectedTiers), len(newLadder.Tiers), "%s", newLadder.Tiers)

			for i, tier := range newLadder.Tiers {
				assert.Equal(testCase.expectedTiers[i].Width, tier.Width, tier)
				assert.Equal(testCase.expectedTiers[i].Height, tier.Height, tier)
				assert.Equal(testCase.expectedTiers[i].VideoBitrate, tier.VideoBitrate, tier)
				assert.Equal(testCase.expectedTiers[i].AudioBitrate, tier.AudioBitrate, tier)
				assert.True(tier.Framerate.Equal(testCase.expectedTiers[i].Framerate), "expected %s fps, got %s", testCase.expectedTiers[i].Framerate, tier.Framerate)
			}

			argSet := newLadder.ArgumentSet("/usr/out")
			args := strings.Join(argSet.GetStrArguments(), " ")

			for _, a := range baseExpectedArgs {
				assert.Contains(args, a)
			}
			for _, a := range testCase.expectedArgs {
				assert.Contains(args, a)
			}
		})
	}
}

func generateMeta(w, h, br int, fr string) ffmpeg.Metadata {
	meta := ffmpeg.Metadata{
		Format:  ffmpeg.Format{BitRate: strconv.Itoa(br * 1000)},
		Streams: []ffmpeg.Streams{{CodecType: "audio"}, {CodecType: "video", BitRate: strconv.Itoa(br * 1000), Index: 0, Width: w, Height: h, AvgFrameRate: fr}},
	}
	return meta
}
