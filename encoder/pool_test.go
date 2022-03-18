package encoder

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lbryio/transcoder/ladder"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/pkg/resolve"
	"github.com/stretchr/testify/suite"
)

type poolSuite struct {
	suite.Suite
	file *os.File
	out  string
}

func TestPoolSuite(t *testing.T) {
	suite.Run(t, new(poolSuite))
}

func (s *poolSuite) SetupSuite() {
	s.out = path.Join(os.TempDir(), "poolSuite_out")

	url := "@specialoperationstest#3/fear-of-death-inspirational#a"
	c, err := resolve.ResolveStream(url)
	if err != nil {
		panic(err)
	}
	s.file, _, err = c.Download(path.Join(os.TempDir(), "poolSuite_in"))
	s.file.Close()
	s.Require().NoError(err)
}

func (s *poolSuite) TearDownSuite() {
	os.Remove(s.file.Name())
	os.RemoveAll(s.out)
}

func (s *poolSuite) TestEncode() {
	absPath, _ := filepath.Abs(s.file.Name())
	enc, err := NewEncoder(Configure().Log(zapadapter.NewKV(nil)).Ladder(ladder.Default))
	s.Require().NoError(err)
	p := NewPool(enc, 10)

	res := (<-p.Encode(absPath, s.out).Value()).(*Result)

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
#EXT-X-STREAM-INF:BANDWIDTH=316800,RESOLUTION=1920x1080,CODECS="avc1.\w+,mp4a.40.2"
v0.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=176000,RESOLUTION=1280x720,CODECS="avc1.\w+,mp4a.40.2"
v1.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=140800,RESOLUTION=640x360,CODECS="avc1.\w+,mp4a.40.2"
v2.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=140800,RESOLUTION=256x144,CODECS="avc1.\w+,mp4a.40.2"
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
		cont, err := ioutil.ReadFile(path.Join(s.out, f))
		s.NoError(err)
		s.Regexp(strings.TrimSpace(str), string(cont))
	}
}
