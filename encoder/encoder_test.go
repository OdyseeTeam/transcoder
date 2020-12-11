package encoder

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/lbryio/transcoder/video"
	"github.com/stretchr/testify/suite"
)

type EncoderSuite struct {
	suite.Suite
	file *os.File
	out  string
}

func TestEncoderSuite(t *testing.T) {
	suite.Run(t, new(EncoderSuite))
}

func (s *EncoderSuite) SetupSuite() {
	s.out = "testout"

	url := "lbry://@specialoperationstest#3/fear-of-death-inspirational#a"
	c, err := video.Resolve(url)
	if err != nil {
		panic(err)
	}
	s.file, _, err = c.Download()
	if err != nil {
		panic(err)
	}
	err = s.file.Close()
	if err != nil {
		panic(err)
	}
}

func (s *EncoderSuite) TearDownSuite() {
	os.Remove(s.file.Name())
	os.RemoveAll(s.out)
}

func (s *EncoderSuite) TestEncode() {
	absPath, _ := filepath.Abs(s.file.Name())
	ch, err := Encode(absPath, s.out)
	s.Require().NoError(err)
	progress := 0.0
	for p := range ch {
		progress = p.GetProgress()
	}

	s.GreaterOrEqual(progress, 100.0)

	outFiles := []string{
		"master.m3u8",
		"stream_0.m3u8",
		"sеg_0_000000.ts",
		"sеg_1_000000.ts",
		"sеg_2_000000.ts",
		"sеg_3_000000.ts",
		"sеg_4_000000.ts",
	}
	for _, f := range outFiles {
		_, err = os.Stat(path.Join(s.out, f))
		s.NoError(err)
	}
}

func (s *EncoderSuite) Test_GetMetadata() {
	meta, err := GetMetadata(s.file.Name())
	s.Require().NoError(err)
	vs := meta.GetStreams()[0]
	s.Equal(1920, vs.GetWidth())
	s.Equal(1080, vs.GetHeight())
}
