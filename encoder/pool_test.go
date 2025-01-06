package encoder

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/OdyseeTeam/transcoder/ladder"
	"github.com/OdyseeTeam/transcoder/pkg/logging/zapadapter"
	"github.com/OdyseeTeam/transcoder/pkg/resolve"

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
	enc, err := NewEncoder(Configure().Log(zapadapter.NewKV(nil)).Ladder(ladder.Default).SpritegenPath(""))
	s.Require().NoError(err)
	p := NewPool(enc, 10)

	res := (<-p.Encode(absPath, s.out).Value()).(*Result)
	s.Require().NotNil(res, "result shouldn't be nil")
	vs := res.OrigMeta.VideoStream
	s.Equal(1920, vs.GetWidth())
	s.Equal(1080, vs.GetHeight())

	progress := 0.0
	for p := range res.Progress {
		progress = p.GetProgress()
	}

	s.Require().GreaterOrEqual(progress, 98.5)

	matchTranscodedOutput(s.T(), s.out, res)
}
