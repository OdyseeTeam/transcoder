package encoder

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lbryio/transcoder/formats"
	"github.com/lbryio/transcoder/manager"
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
	s.out = path.Join(os.TempDir(), "poolSuitee_out")

	url := "@specialoperationstest#3/fear-of-death-inspirational#a"
	c, err := manager.ResolveRequest(url)
	if err != nil {
		panic(err)
	}
	s.file, _, err = c.Download(path.Join(os.TempDir(), "poolSuitee_in"))
	s.file.Close()
	s.Require().NoError(err)
}

func (s *poolSuite) TearDownSuite() {
	os.Remove(s.file.Name())
	os.RemoveAll(s.out)
}

func (s *poolSuite) TestEncode() {
	absPath, _ := filepath.Abs(s.file.Name())
	enc, err := NewEncoder(Configure())
	s.Require().NoError(err)
	p := NewPool(enc, 10)

	res := p.Encode(absPath, s.out).Value().(*Result)

	vs := formats.GetVideoStream(res.Meta)
	s.Equal(1920, vs.GetWidth())
	s.Equal(1080, vs.GetHeight())

	progress := 0.0
	for p := range res.Progress {
		progress = p.GetProgress()
	}

	s.Require().GreaterOrEqual(progress, 99.5)

	outFiles := map[string]string{
		"master.m3u8": `
#EXTM3U
#EXT-X-VERSION:6
#EXT-X-STREAM-INF:BANDWIDTH=3660800,RESOLUTION=1920x1080,CODECS="avc1.640028,mp4a.40.2"
stream_0.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=2340800,RESOLUTION=1280x720,CODECS="avc1.64001f,mp4a.40.2"
stream_1.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=756800,RESOLUTION=640x360,CODECS="avc1.64001e,mp4a.40.2"
stream_2.m3u8

#EXT-X-STREAM-INF:BANDWIDTH=316800,RESOLUTION=256x144,CODECS="avc1.64000c,mp4a.40.2"
stream_3.m3u8
`,
		"stream_0.m3u8":   "seg_0_000000.ts",
		"stream_1.m3u8":   "seg_1_000000.ts",
		"stream_2.m3u8":   "seg_2_000000.ts",
		"stream_3.m3u8":   "seg_3_000000.ts",
		"seg_0_000000.ts": "",
		"seg_1_000000.ts": "",
		"seg_2_000000.ts": "",
		"seg_3_000000.ts": "",
	}
	for f, str := range outFiles {
		cont, err := ioutil.ReadFile(path.Join(s.out, f))
		s.NoError(err)
		s.Regexp(strings.TrimSpace(str), string(cont))
	}
}
