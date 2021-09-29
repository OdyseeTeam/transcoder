package formats

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/floostack/transcoder"
	"github.com/floostack/transcoder/ffmpeg"
	"github.com/pkg/errors"
)

const (
	TypeHLS   = "hls"
	TypeDASH  = "dash"
	TypeRange = "range"

	FPS30 = 30
	FPS60 = 60
)

type Format struct {
	Resolution Resolution
	Bitrate    Bitrate
}

type Codec []Format

type Resolution struct {
	Width, Height int
}

// Bitrate in kilobits
type Bitrate struct {
	FPS30, FPS60 int
}

// Commonly defined resolutions
var (
	UHD4K  = Resolution{Height: 2160}
	QHD2K  = Resolution{Height: 1440}
	HD1080 = Resolution{Height: 1080}
	HD720  = Resolution{Height: 720}
	SD480  = Resolution{Height: 480}
	SD360  = Resolution{Height: 360}
	SD240  = Resolution{Height: 240}
	SD144  = Resolution{Height: 144}
)

var Resolutions = []Resolution{
	UHD4K, QHD2K, HD1080, HD720, SD480, SD360, SD240, SD144,
}

// H264 codec with its suggested bitrates
var H264 = Codec{
	Format{UHD4K, Bitrate{FPS30: 18000, FPS60: 28000}},
	Format{QHD2K, Bitrate{FPS30: 10000, FPS60: 16000}},
	Format{HD1080, Bitrate{FPS30: 2000, FPS60: 3200}},
	Format{HD720, Bitrate{FPS30: 1200, FPS60: 2000}},
	// Format{SD480, Bitrate{FPS30: 900, FPS60: 1700}},
	Format{SD360, Bitrate{FPS30: 350, FPS60: 560}},
	Format{SD144, Bitrate{FPS30: 100, FPS60: 160}},
}

// brResolutionFactor is a quality factor for non-standard resolution videos. The higher it is
var brResolutionFactor = .11
var fpsPattern = regexp.MustCompile(`^(\d+)?.+`)

func (f Format) GetBitrateForFPS(fps int) int {
	if fps == FPS60 {
		return f.Bitrate.FPS60
	}
	return f.Bitrate.FPS30
}

func TargetFormats(codec Codec, meta *ffmpeg.Metadata) ([]Format, error) {
	var (
		origFPS, origBitrate int
		err                  error
	)

	vs := GetVideoStream(meta)
	if vs == nil {
		return nil, errors.New("no video stream detected")
	}
	w, h := vs.GetWidth(), vs.GetHeight()

	origFPS, err = DetectFPS(meta)
	if err != nil {
		return nil, err
	}

	origRes := Resolution{Height: h}
	for _, r := range Resolutions {
		if h == r.Height {
			origRes = r
		}
	}

	origBitrate, err = strconv.Atoi(meta.GetFormat().GetBitRate())
	if err != nil {
		return nil, errors.Wrap(err, "cannot determine bitrate")
	}
	origBitrate = origBitrate / 1000

	// Adding all resolutions that are equal or lower in height than the original media.
	formats := []Format{}
	for _, f := range codec {
		if origRes.Height >= f.Resolution.Height {
			formats = append(formats, f)
		}
	}

	// Excluding all target resolutions that have bitrate higher than the original media.
	formatsFinal := []Format{}
	for _, f := range formats {
		// Do not remove original resolution
		if f.Resolution.Height == origRes.Height {
			formatsFinal = append(formatsFinal, f)
			continue
		}
		targetBitrate := f.GetBitrateForFPS(origFPS)
		cutoffBitrate := targetBitrate + int(float32(targetBitrate)*.4)
		if origBitrate > cutoffBitrate {
			formatsFinal = append(formatsFinal, f)
		} else {
			logger.Debugw(
				"extraneous format",
				"format", f,
				"orig_bitrate", origBitrate,
				"target_bitrate", targetBitrate,
				"cutoff_bitrate", cutoffBitrate,
			)
		}
	}

	hasSelf := false
	for _, f := range formatsFinal {
		if f.Resolution == origRes {
			hasSelf = true
			break
		}
	}
	if !hasSelf {
		formatsFinal = append(formatsFinal, codec.CustomFormat(Resolution{Width: w, Height: h}))
	}

	return formatsFinal, nil
}

// CustomFormat generates a Format for non-standard resolutions, calculating optimal bitrates (note: it should be calculated better).
func (c Codec) CustomFormat(r Resolution) Format {
	for _, f := range c {
		if f.Resolution == r {
			return f
		}
	}
	br := Bitrate{
		FPS30: int(float64(r.Width*r.Height) * brResolutionFactor / 100),
		FPS60: int(float64(float64(r.Width)*float64(r.Height)*1.56) * brResolutionFactor / 100),
	}
	return Format{Resolution: r, Bitrate: br}
}

func DetectFPS(meta *ffmpeg.Metadata) (int, error) {
	var (
		err error
		fps int
	)
	vs := GetVideoStream(meta)
	fpsMatch := fpsPattern.FindStringSubmatch(vs.GetAvgFrameRate())
	if len(fpsMatch) > 0 {
		fps, err = strconv.Atoi(fpsMatch[1])
		if err != nil {
			return fps, errors.Wrap(err, "cannot determine FPS")
		}

		if fps > 40 {
			fps = FPS60
		} else {
			fps = FPS30
		}
	} else {
		return fps, fmt.Errorf("cannot determine FPS from `%v`", vs.GetAvgFrameRate())
	}
	return fps, nil
}

func GetVideoStream(meta *ffmpeg.Metadata) transcoder.Streams {
	for _, s := range meta.GetStreams() {
		if s.GetCodecType() == "video" {
			return s
		}
	}
	return nil
}
