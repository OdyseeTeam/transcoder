package encoder

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/lbryio/transcoder/pkg/claim"
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
	s.out = path.Join(os.TempDir(), "encoder_test_out")

	url := "lbry://@specialoperationstest#3/fear-of-death-inspirational#a"
	c, err := claim.Resolve(url)
	if err != nil {
		panic(err)
	}
	s.file, _, err = c.Download(path.Join(os.TempDir(), "transcoder_test"))
	s.file.Close()
	s.Require().NoError(err)
	s.Require().NoError(s.file.Close())
}

func (s *EncoderSuite) TearDownSuite() {
	os.Remove(s.file.Name())
	os.RemoveAll(s.out)
}

func (s *EncoderSuite) TestEncode() {
	absPath, _ := filepath.Abs(s.file.Name())
	e, err := NewEncoder(absPath, s.out)
	s.Require().NoError(err)
	ch, err := e.Encode()
	s.Require().NoError(err)
	progress := 0.0
	for p := range ch {
		progress = p.GetProgress()
		if progress >= 99.9 {
			break
		}
	}

	s.Require().GreaterOrEqual(progress, 99.9)

	outFiles := []string{
		"master.m3u8",
		"stream_0.m3u8",
		"seg_0_000000.ts",
		"seg_1_000000.ts",
		"seg_2_000000.ts",
		"seg_3_000000.ts",
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
