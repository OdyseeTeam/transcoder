package encoder

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/lbryio/transcoder/formats"
	"github.com/lbryio/transcoder/internal/metrics"

	ffmpegt "github.com/floostack/transcoder"
	"github.com/floostack/transcoder/ffmpeg"
)

var ffmpegConf = ffmpeg.Config{
	FfmpegBinPath:   "",
	FfprobeBinPath:  "",
	ProgressEnabled: true,
	Verbose:         false,
}

type Encoder interface {
	Encode() (<-chan ffmpegt.Progress, error)
	Meta() *ffmpeg.Metadata
}

type encoder struct {
	in, out string
	meta    *ffmpeg.Metadata
}

func init() {
	var err error
	ffmpegConf.FfmpegBinPath, err = exec.LookPath("ffmpeg")
	if err != nil {
		ffmpegConf.FfmpegBinPath = firstExistingFile([]string{"/usr/local/bin/ffmpeg", "/usr/bin/ffmpeg"})
	}
	ffmpegConf.FfprobeBinPath, err = exec.LookPath("ffprobe")
	if err != nil {
		ffmpegConf.FfprobeBinPath = firstExistingFile([]string{"/usr/local/bin/ffmpeg", "/usr/bin/ffmpeg"})
	}
	logger.Infow("ffmpeg configuration", "conf", ffmpegConf)
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

func NewEncoder(in, out string) (Encoder, error) {
	if ffmpegConf.FfmpegBinPath == "" || ffmpegConf.FfprobeBinPath == "" {
		return nil, errors.New("ffmpeg/ffprobe not found")
	}
	e := &encoder{in: in, out: out}
	meta, err := GetMetadata(e.in)
	if err != nil {
		return nil, err
	}
	e.meta = meta
	return e, nil
}

func (e *encoder) Meta() *ffmpeg.Metadata {
	return e.meta
}

// Encode does transcoding of specified video file into a series of HLS streams.
func (e *encoder) Encode() (<-chan ffmpegt.Progress, error) {
	ll := logger.With("in", e.in)
	conf := ffmpegConf
	conf.OutputDir = e.out

	if err := os.MkdirAll(e.out, os.ModePerm); err != nil {
		return nil, err
	}

	targetFormats, err := formats.TargetFormats(formats.H264, e.meta)
	if err != nil {
		return nil, err
	}

	fps, err := formats.DetectFPS(e.meta)
	if err != nil {
		return nil, err
	}

	args, err := NewArguments(e.out, targetFormats, fps)
	if err != nil {
		return nil, err
	}

	vs := formats.GetVideoStream(e.meta)
	ll.Infow(
		"starting transcoding",
		"args", strings.Join(args.GetStrArguments(), " "),
		"media_duration", e.meta.GetFormat().GetDuration(),
		"media_bitrate", e.meta.GetFormat().GetBitRate(),
		"media_width", vs.GetWidth(),
		"media_height", vs.GetHeight(),
	)

	dur, _ := strconv.ParseFloat(e.meta.GetFormat().GetDuration(), 64)
	btr, _ := strconv.ParseFloat(e.meta.GetFormat().GetBitRate(), 64)
	metrics.EncodedDurationSeconds.Add(dur)
	metrics.EncodedBitrateMbit.WithLabelValues(fmt.Sprintf("%v", vs.GetHeight())).Observe(btr / 1024 / 1024)

	progress, err := ffmpeg.New(&conf).
		Input(e.in).
		Output("stream_%v.m3u8").
		Start(args)
	if err != nil {
		return nil, err
	}

	return progress, nil
}

// GetMetadata uses ffprobe to parse video file metadata.
func GetMetadata(file string) (*ffmpeg.Metadata, error) {
	metadata := &ffmpeg.Metadata{}

	var outb, errb bytes.Buffer

	args := []string{"-i", file, "-print_format", "json", "-show_format", "-show_streams", "-show_error"}

	cmd := exec.Command(ffmpegConf.FfprobeBinPath, args...)
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf(
			"error executing (%s) with args (%s) | error: %s | message: %s %s",
			ffmpegConf.FfprobeBinPath, args, err, outb.String(), errb.String())
	}

	if err = json.Unmarshal([]byte(outb.String()), &metadata); err != nil {
		return nil, err
	}

	return metadata, nil
}
