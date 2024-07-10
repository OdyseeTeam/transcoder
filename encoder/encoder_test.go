package encoder

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OdyseeTeam/transcoder/ladder"
	"github.com/OdyseeTeam/transcoder/pkg/logging/zapadapter"
	"github.com/OdyseeTeam/transcoder/pkg/resolve"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	FPS0  = decimal.NewFromInt(0)
	FPS15 = decimal.NewFromInt(15)
)

type encoderSuite struct {
	suite.Suite
	file    *os.File
	in, out string
}

func TestEncoderSuite(t *testing.T) {
	suite.Run(t, new(encoderSuite))
}

func (s *encoderSuite) SetupSuite() {
	s.out = path.Join(os.TempDir(), "encoderSuite_out")
	s.in = path.Join(os.TempDir(), "encoderSuite_in")

	url := "@specialoperationstest#3/fear-of-death-inspirational#a"
	c, err := resolve.ResolveStream(url)
	if err != nil {
		panic(err)
	}
	s.file, _, err = c.Download(s.in)
	s.file.Close()
	s.Require().NoError(err)
}

func (s *encoderSuite) TearDownSuite() {
	os.Remove(s.file.Name())
	os.RemoveAll(s.out)
}

func (s *encoderSuite) TestCheckFastStart() {
	absPath, _ := filepath.Abs(s.file.Name())
	e, err := NewEncoder(Configure().Log(zapadapter.NewKV(nil)).Ladder(ladder.Default).SpritegenPath(""))
	s.Require().NoError(err)
	m, err := e.GetMetadata(absPath)
	s.Require().NoError(err)
	s.True(m.FastStart)
}

func (s *encoderSuite) TestEncode() {
	absPath, _ := filepath.Abs(s.file.Name())
	e, err := NewEncoder(Configure().Log(zapadapter.NewKV(nil)).Ladder(ladder.Default).SpritegenPath(""))
	s.Require().NoError(err)

	res, err := e.Encode(absPath, s.out)
	s.Require().NoError(err)

	vs := res.OrigMeta.VideoStream
	s.Equal(1920, vs.GetWidth())
	s.Equal(1080, vs.GetHeight())

	progress := 0.0
	for p := range res.Progress {
		progress = p.GetProgress()
	}

	s.Require().GreaterOrEqual(progress, 99.5)

	s.Equal(1080, res.Ladder.Tiers[0].Height)
	s.Equal(720, res.Ladder.Tiers[1].Height)
	s.Equal(360, res.Ladder.Tiers[2].Height)
	s.Equal(144, res.Ladder.Tiers[3].Height)

	outFiles := map[string]string{
		"master.m3u8": `
#EXTM3U
#EXT-X-VERSION:6
#EXT-X-STREAM-INF:BANDWIDTH=\d+,RESOLUTION=1920x1080,CODECS="avc1.\w+,mp4a.40.2"
v0.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=\d+,RESOLUTION=1280x720,CODECS="avc1.\w+,mp4a.40.2"
v1.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=\d+,RESOLUTION=640x360,CODECS="avc1.\w+,mp4a.40.2"
v2.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=\d+,RESOLUTION=256x144,CODECS="avc1.\w+,mp4a.40.2"
v3.m3u8`,
		"v0.m3u8":       "v0_s000000.ts",
		"v1.m3u8":       "v1_s000000.ts",
		"v2.m3u8":       "v2_s000000.ts",
		"v3.m3u8":       "v3_s000000.ts",
		"v0_s000000.ts": "",
		"v1_s000000.ts": "",
		"v2_s000000.ts": "",
		"v3_s000000.ts": "",
	}
	for f, str := range outFiles {
		cont, err := os.ReadFile(path.Join(s.out, f))
		s.NoError(err)
		s.Regexp(strings.TrimSpace(str), string(cont))
	}
}

func TestTweakRealStreams(t *testing.T) {
	t.Skip()
	encoder, err := NewEncoder(Configure().Log(zapadapter.NewKV(nil)).Ladder(ladder.Default).SpritegenPath(""))
	require.NoError(t, err)

	testCases := []struct {
		url           string
		expectedTiers []ladder.Tier
	}{
		{
			// "hot-tub-streamers-are-furious-at#06e0bc43f55fec0bd946a3cb18fc2ff9bc1cb2aa",
			"hot-tub-streamers-are-furious.mp4",
			[]ladder.Tier{
				{Width: 1920, Height: 1080, VideoBitrate: 3500_000, AudioBitrate: "160k", Framerate: FPS0},
				{Width: 1280, Height: 720, VideoBitrate: 2500_000, AudioBitrate: "128k", Framerate: FPS0},
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: FPS0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "96k", Framerate: FPS15},
			},
		},
		{
			// "hot-tub-streamers-are-furious-at#06e0bc43f55fec0bd946a3cb18fc2ff9bc1cb2aa",
			"why-mountain-biking-here-will.mp4",
			[]ladder.Tier{
				{Width: 1920, Height: 1080, VideoBitrate: 3500_000, AudioBitrate: "160k", Framerate: FPS0},
				{Width: 1280, Height: 720, VideoBitrate: 2500_000, AudioBitrate: "128k", Framerate: FPS0},
				{Width: 640, Height: 360, VideoBitrate: 500_000, AudioBitrate: "96k", Framerate: FPS0},
				{Width: 256, Height: 144, VideoBitrate: 100_000, AudioBitrate: "96k", Framerate: FPS15},
			},
		},
	}

	for _, tc := range testCases {
		absPath, err := filepath.Abs(filepath.Join("./testdata", tc.url))
		require.NoError(t, err)
		lmeta, err := encoder.GetMetadata(absPath)
		require.NoError(t, err)

		stream := ladder.GetVideoStream(lmeta.FMeta)
		testName := fmt.Sprintf(
			"%s (%vx%v@%vbps)",
			tc.url,
			stream.GetWidth(),
			stream.GetHeight(),
			lmeta.FMeta.GetFormat().GetBitRate(),
		)
		t.Run(testName, func(t *testing.T) {
			assert.NoError(t, err)
			targetLadder, err := ladder.Default.Tweak(lmeta)
			assert.NoError(t, err)
			assert.NoError(t, err)
			require.Equal(t, len(tc.expectedTiers), len(targetLadder.Tiers), targetLadder.Tiers)
			for i, tier := range targetLadder.Tiers {
				assert.Equal(t, tc.expectedTiers[i].Width, tier.Width, tier)
				assert.Equal(t, tc.expectedTiers[i].Height, tier.Height, tier)
				assert.Equal(t, tc.expectedTiers[i].VideoBitrate, tier.VideoBitrate, tier)
				assert.Equal(t, tc.expectedTiers[i].AudioBitrate, tier.AudioBitrate, tier)
				assert.Equal(t, tc.expectedTiers[i].Framerate, tier.Framerate, tier)
			}
		})
	}
}
