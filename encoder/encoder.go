package encoder

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/lbryio/transcoder/formats"

	ffmpegt "github.com/floostack/transcoder"
	"github.com/floostack/transcoder/ffmpeg"
)

var binFFMpeg, binFFProbe string

type Encoder struct {
	in, out string
	Meta    *ffmpeg.Metadata
}

func init() {
	binFFMpeg = firstExistingFile([]string{"/usr/local/bin/ffmpeg", "/usr/bin/ffmpeg"})
	binFFProbe = firstExistingFile([]string{"/usr/local/bin/ffprobe", "/usr/bin/ffprobe"})
}

func firstExistingFile(paths []string) string {
	for _, p := range paths {
		_, err := os.Stat(p)
		if !os.IsNotExist(err) {
			return p
		}
	}
	return ""
}

func NewEncoder(in, out string) (*Encoder, error) {
	if binFFMpeg == "" || binFFProbe == "" {
		return nil, errors.New("ffmpeg/ffprobe not found")
	}
	e := &Encoder{in: in, out: out}
	meta, err := GetMetadata(e.in)
	if err != nil {
		return nil, err
	}
	e.Meta = meta
	return e, nil
}

// Encode does transcoding of specified video file into a series of HLS streams.
func (e *Encoder) Encode() (<-chan ffmpegt.Progress, error) {
	ffmpegConf := &ffmpeg.Config{
		FfmpegBinPath:   binFFMpeg,
		FfprobeBinPath:  binFFProbe,
		ProgressEnabled: true,
		Verbose:         false,
	}
	ll := logger.With("in", e.in)

	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot get working dir: %v", err)
	}
	if err := os.MkdirAll(e.out, os.ModePerm); err != nil {
		return nil, err
	}
	if err := os.Chdir(e.out); err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	defer os.Chdir(wd)

	targetFormats, err := formats.TargetFormats(formats.H264, e.Meta)
	if err != nil {
		return nil, err
	}

	fps, err := formats.DetectFPS(e.Meta)
	if err != nil {
		return nil, err
	}

	args, err := NewArguments(e.out, targetFormats, fps)
	if err != nil {
		return nil, err
	}

	vs := e.Meta.GetStreams()[0]
	ll.Debugw(
		"starting encoding",
		"args", strings.Join(args.GetStrArguments(), " "),
		"media_duration", e.Meta.GetFormat().GetDuration(),
		"media_bitrate", e.Meta.GetFormat().GetBitRate(),
		"media_width", vs.GetWidth(),
		"media_height", vs.GetHeight(),
	)
	return ffmpeg.New(ffmpegConf).
		Input(e.in).
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
