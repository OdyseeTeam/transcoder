package encoder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/lbryio/transcoder/formats"

	ffmpegt "github.com/floostack/transcoder"
	"github.com/floostack/transcoder/ffmpeg"
	"go.uber.org/zap"
)

const binFFProbe = "/usr/local/bin/ffprobe"
const binFFMpeg = "/usr/local/bin/ffmpeg"

var logger = zap.NewExample().Sugar().Named("encoder")

// Encode does transcoding of `in` video file into a series of HLS stream video files.
func Encode(in, out string) (<-chan ffmpegt.Progress, error) {
	ffmpegConf := &ffmpeg.Config{
		FfmpegBinPath:   binFFMpeg,
		FfprobeBinPath:  binFFProbe,
		ProgressEnabled: true,
		// Verbose:         true,
	}
	ll := logger.With("in", in)

	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot get working dir: %v", err)
	}
	if err := os.MkdirAll(out, os.ModePerm); err != nil {
		return nil, err
	}
	if err := os.Chdir(out); err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	defer os.Chdir(wd)

	meta, err := GetMetadata(in)
	if err != nil {
		return nil, err
	}
	vs := meta.GetStreams()[0]
	fs := formats.TargetFormats(formats.H264, vs.GetWidth(), vs.GetHeight())
	args, err := NewArguments(out, fs)
	if err != nil {
		return nil, err
	}

	ll.Debugw("encoding started", "args", strings.Join(args.GetStrArguments(), " "))

	return ffmpeg.New(ffmpegConf).
		Input(in).
		Output("stream_%v.m3u8").
		Start(args)
}

// GetMetadata uses ffprobe to parse video file metadata.
func GetMetadata(file string) (*ffmpeg.Metadata, error) {
	metadata := &ffmpeg.Metadata{}

	var outb, errb bytes.Buffer

	args := []string{"-i", file, "-print_format", "json", "-show_format", "-show_streams", "-show_error"}

	cmd := exec.Command(binFFProbe, args...)
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error executing (%s) with args (%s) | error: %s | message: %s %s", binFFProbe, args, err, outb.String(), errb.String())
	}

	if err = json.Unmarshal([]byte(outb.String()), &metadata); err != nil {
		return nil, err
	}

	return metadata, nil
}
