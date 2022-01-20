package encoder

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/lbryio/transcoder/internal/metrics"
	"github.com/lbryio/transcoder/ladder"
	"github.com/lbryio/transcoder/pkg/logging"

	ffmpegt "github.com/floostack/transcoder"
	"github.com/floostack/transcoder/ffmpeg"
)

const MasterPlaylist = "master.m3u8"

type Encoder interface {
	Encode(in, out string) (*Result, error)
}

type Configuration struct {
	ffmpegPath, ffprobePath,
	thumbnailGeneratorPath string

	ladder ladder.Ladder
	log    logging.KVLogger
}

type encoder struct {
	*Configuration
	tg *ThumbnailGenerator
}

type Result struct {
	Input, Output string
	Meta          *ffmpeg.Metadata
	Ladder        ladder.Ladder
	Progress      <-chan ffmpegt.Progress
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
		ladder:                 ladder.Default,
		log:                    logging.NoopKVLogger{},
	}
}

func NewEncoder(cfg *Configuration) (Encoder, error) {
	if cfg.ffmpegPath == "" {
		return nil, errors.New("ffmpeg binary path not set")
	}
	if cfg.ffprobePath == "" {
		return nil, errors.New("ffprobe binary path not set")
	}
	if len(cfg.ladder.Tiers) == 0 {
		return nil, errors.New("encoding ladder not configured")
	}

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
		e.log.Info("thumbnail generator was not configured", "err", err)
	} else {
		e.tg = tg
	}

	e.log.Info("encoder configured", "ffmpeg", e.ffmpegPath, "ffprobe", e.ffprobePath, "generator", e.thumbnailGeneratorPath)
	return &e, nil
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

// Ladder configures encoding ladder.
func (c *Configuration) Ladder(l ladder.Ladder) *Configuration {
	c.ladder = l
	return c
}

// Encode does transcoding of specified video file into a series of HLS streams.
func (e encoder) Encode(input, output string) (*Result, error) {
	meta, err := e.getMetadata(input)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(output, os.ModePerm); err != nil {
		return nil, err
	}

	targetLadder, err := e.ladder.Tweak(meta)
	if err != nil {
		return nil, err
	}
	res := &Result{Input: input, Output: output, Meta: meta, Ladder: targetLadder}

	if e.tg != nil {
		err := e.tg.Generate(input, path.Join(output, "thumbnails10k.png"))
		if err != nil {
			return nil, err
		}
	}

	a, err := ladder.NewArguments(output, e.ladder, meta)
	if err != nil {
		return nil, err
	}

	vs := ladder.GetVideoStream(meta)
	e.log.Info(
		"starting transcoding",
		"args", strings.Join(a.GetStrArguments(), " "),
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
			Verbose:         false, // Set this to false if ffmpeg is failing to launch to see the reason
			OutputDir:       output,
		}).
		Input(input).
		Output("var_%v.m3u8").
		Start(a)
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
