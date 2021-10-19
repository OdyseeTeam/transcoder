package encoder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/lbryio/transcoder/formats"
	"github.com/lbryio/transcoder/internal/metrics"
	"github.com/lbryio/transcoder/pkg/logging"

	ffmpegt "github.com/floostack/transcoder"
	"github.com/floostack/transcoder/ffmpeg"
)

type Encoder interface {
	Encode(in, out string) (*Result, error)
}

type encoder struct {
	*Configuration
	tg *ThumbnailGenerator
}

type Result struct {
	Input, Output string
	Meta          *ffmpeg.Metadata
	Progress      <-chan ffmpegt.Progress
}

type Configuration struct {
	ffmpegPath, ffprobePath, thumbnailGeneratorPath string
	log                                             logging.KVLogger
}

func NewEncoder(cfg *Configuration) (Encoder, error) {
	var cmd *exec.Cmd
	cmd = exec.Command(cfg.ffmpegPath, "-h")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("unable to execute ffmpeg: %w", err)
	}

	cmd = exec.Command(cfg.ffprobePath, "-h")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("unable to execute ffprobe: %w", err)
	}

	e := encoder{Configuration: cfg}

	tg, err := NewThumbnailGenerator(e.thumbnailGeneratorPath)
	if err != nil {
		e.log.Warn("thumbnail generator was not configured", "err", err)
	} else {
		e.tg = tg
	}

	e.log.Info("encoder configured", "ffmpeg", cfg.ffmpegPath, "ffprobe", cfg.ffprobePath)
	return &e, nil
}

// Configure will attempt to lookup paths to ffmpeg and ffprobe.
// Call FfmpegPath and FfprobePath if you need to set it manually.
func Configure() *Configuration {
	ffmpegPath, _ := exec.LookPath("ffmpeg")
	ffprobePath, _ := exec.LookPath("ffprobe")
	tgPath, _ := exec.LookPath("generator")

	return &Configuration{
		ffmpegPath:             ffmpegPath,
		ffprobePath:            ffprobePath,
		thumbnailGeneratorPath: tgPath,
		log:                    logging.NoopKVLogger{},
	}
}

func (c *Configuration) FfmpegPath(p string) *Configuration {
	c.ffmpegPath = p
	return c
}

func (c *Configuration) FfprobePath(p string) *Configuration {
	c.ffprobePath = p
	return c
}

func (c *Configuration) ThumbnailGeneratorPath(p string) *Configuration {
	c.thumbnailGeneratorPath = p
	return c
}

// Log configures encoder logging. Default configuration is a no-op logger.
func (c *Configuration) Log(l logging.KVLogger) *Configuration {
	c.log = l
	return c
}

// Encode does transcoding of specified video file into a series of HLS streams.
func (e encoder) Encode(input, output string) (*Result, error) {
	meta, err := e.getMetadata(input)
	if err != nil {
		return nil, err
	}
	res := &Result{Input: input, Output: output, Meta: meta}

	if err := os.MkdirAll(output, os.ModePerm); err != nil {
		return nil, err
	}

	targetFormats, err := formats.TargetFormats(formats.H264, meta)
	if err != nil {
		return nil, err
	}

	// Generate thumbnails before we do anything else
	if e.tg != nil {
		err := e.tg.Generate(input, output)
		if err != nil {
			return nil, err
		}
	}

	fps, err := formats.DetectFPS(meta)
	if err != nil {
		return nil, err
	}

	args, err := NewArguments(output, targetFormats, fps)
	if err != nil {
		return nil, err
	}

	vs := formats.GetVideoStream(meta)
	e.log.Info(
		"starting transcoding",
		"args", strings.Join(args.GetStrArguments(), " "),
		"media_duration", meta.GetFormat().GetDuration(),
		"media_bitrate", meta.GetFormat().GetBitRate(),
		"media_width", vs.GetWidth(),
		"media_height", vs.GetHeight(),
		"input", input, "output", output,
	)

	dur, _ := strconv.ParseFloat(meta.GetFormat().GetDuration(), 64)
	btr, _ := strconv.ParseFloat(meta.GetFormat().GetBitRate(), 64)
	metrics.EncodedDurationSeconds.Add(dur)
	metrics.EncodedBitrateMbit.WithLabelValues(fmt.Sprintf("%v", vs.GetHeight())).Observe(btr / 1024 / 1024)

	progress, err := ffmpeg.New(
		&ffmpeg.Config{
			FfmpegBinPath:   e.ffmpegPath,
			FfprobeBinPath:  e.ffprobePath,
			ProgressEnabled: true,
			Verbose:         false,
			OutputDir:       output,
		}).
		Input(input).
		Output("stream_%v.m3u8").
		Start(args)
	if err != nil {
		return nil, err
	}

	res.Progress = progress
	return res, nil
}

// getMetadata uses ffprobe to parse video file metadata.
func (e encoder) getMetadata(input string) (*ffmpeg.Metadata, error) {
	meta := &ffmpeg.Metadata{}

	var outb, errb bytes.Buffer

	args := []string{"-i", input, "-print_format", "json", "-show_format", "-show_streams", "-show_error"}

	cmd := exec.Command(e.ffprobePath, args...)
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf(
			"error executing (%s) with args (%s) | error: %s | message: %s %s",
			e.ffprobePath, args, err, outb.String(), errb.String())
	}

	if err = json.Unmarshal(outb.Bytes(), &meta); err != nil {
		return nil, err
	}

	return meta, nil
}
