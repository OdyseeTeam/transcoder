package encoder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/karrick/godirwalk"
	"github.com/lbryio/transcoder/internal/metrics"
	"github.com/lbryio/transcoder/ladder"
	"github.com/lbryio/transcoder/pkg/logging"

	ffmpegt "github.com/floostack/transcoder"
	"github.com/floostack/transcoder/ffmpeg"
	"github.com/pkg/errors"
)

const MasterPlaylist = "master.m3u8"

type Encoder interface {
	Encode(in, out string) (*Result, error)
	GetMetadata(input string) (*ladder.Metadata, error)
}

type Configuration struct {
	ffmpegPath, ffprobePath,
	spritegenPath string

	ladder ladder.Ladder
	log    logging.KVLogger
}

type encoder struct {
	*Configuration
	spriteGen *SpriteGenerator
}

type Result struct {
	Input, Output string
	OrigMeta      *ladder.Metadata
	Ladder        ladder.Ladder
	Progress      <-chan ffmpegt.Progress
}

// Configure will attempt to lookup paths to ffmpeg and ffprobe.
// Call FfmpegPath and FfprobePath if you need to set it manually.
func Configure() *Configuration {
	ffmpegPath, _ := exec.LookPath("ffmpeg")
	ffprobePath, _ := exec.LookPath("ffprobe")
	spritegenPath, _ := exec.LookPath("node")

	return &Configuration{
		ffmpegPath:    ffmpegPath,
		ffprobePath:   ffprobePath,
		spritegenPath: spritegenPath,
		ladder:        ladder.Default,
		log:           logging.NoopKVLogger{},
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

	spriteGen, err := NewSpriteGenerator(e.spritegenPath)
	if err != nil {
		e.log.Info("sprite generator was not configured", "err", err)
	} else {
		e.spriteGen = spriteGen
	}

	e.log.Info("encoder configured", "ffmpeg", e.ffmpegPath, "ffprobe", e.ffprobePath, "spritegen", e.spritegenPath)
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

func (c *Configuration) SpritegenPath(p string) *Configuration {
	c.spritegenPath = p
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
	meta, err := e.GetMetadata(input)
	if err != nil {
		return nil, err
	}
	ll := e.log.With("input", input, "output", output)

	if err := os.MkdirAll(output, os.ModePerm); err != nil {
		return nil, err
	}

	targetLadder, err := e.ladder.Tweak(meta)
	if err != nil {
		return nil, err
	}
	res := &Result{Input: input, Output: output, OrigMeta: meta, Ladder: targetLadder}

	if e.spriteGen != nil {
		ll.Info("starting sprite generator")
		err := e.spriteGen.Generate(input, output)
		if err != nil {
			return nil, errors.Wrap(err, "could not start sprite generator")
		}
		outputFiles, err := godirwalk.ReadDirnames(output, nil)
		if err != nil {
			return nil, errors.Wrap(err, "could not get a list of files")
		}
		e.log.Info("sprite generator done", "files", outputFiles)
	}

	args := targetLadder.ArgumentSet(output, meta)
	vs := meta.VideoStream
	ll.Info(
		"starting transcoding",
		"args", strings.Join(args.GetStrArguments(), " "),
		"media_duration", meta.FMeta.GetFormat().GetDuration(),
		"media_bitrate", meta.FMeta.GetFormat().GetBitRate(),
		"media_width", vs.GetWidth(),
		"media_height", vs.GetHeight(),
	)

	dur, _ := strconv.ParseFloat(meta.FMeta.GetFormat().GetDuration(), 64)
	btr, _ := strconv.ParseFloat(meta.FMeta.GetFormat().GetBitRate(), 64)
	metrics.EncodedDurationSeconds.Add(dur)
	metrics.EncodedBitrateMbit.WithLabelValues(fmt.Sprintf("%v", vs.GetHeight())).Observe(btr / 1024 / 1024)

	progress, err := ffmpeg.New(
		&ffmpeg.Config{
			FfmpegBinPath:   e.ffmpegPath,
			FfprobeBinPath:  e.ffprobePath,
			ProgressEnabled: true,
			Verbose:         false, // Set this to true if ffmpeg is failing to launch to see the reason
			OutputDir:       output,
		}).
		Input(input).
		Output("v%v.m3u8").
		Start(args)
	if err != nil {
		return nil, err
	}

	res.Progress = progress
	return res, nil
}

// getMetadata uses ffprobe to parse video file metadata.
func (e encoder) GetMetadata(input string) (*ladder.Metadata, error) {
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

	lm, err := ladder.WrapMeta(meta)
	if err != nil {
		return nil, errors.Wrap(err, "unable to wrap with ladder.Metadata")
	}
	lm.FastStart, err = e.checkFastStart(input)
	if err != nil {
		return nil, errors.Wrap(err, "unable to check for faststart")
	}
	return lm, nil
}

func (e encoder) checkFastStart(input string) (bool, error) {
	cmd := exec.Command(e.ffmpegPath, "-v", "trace", "-i", input)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Run()
	result := strings.Fields(out.String())
	var seenMdat bool
	for _, l := range result {
		if strings.Contains(l, "moov") && !seenMdat {
			return true, nil
		}
	}
	return false, nil
}
