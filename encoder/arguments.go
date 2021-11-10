package encoder

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lbryio/transcoder/formats"
)

var ErrInvalidTargetType = errors.New("unsupported target type")

type Argument [2]string

const (
	MasterPlaylist     = "master.m3u8"
	preset             = "veryfast"
	keyint             = "100"
	videoCodec         = "libx264"
	constantRateFactor = "26"
	hlsTime            = "10"
)

type Arguments struct {
	defaultArgs []Argument
	formats     []formats.Format
	out         string
	fps         int
}

type TargetType string

const (
	TargetTypeUnknown            = "unknown"
	TargetTypeHLS     TargetType = "hls"
	TargetTypeTS                 = "mpegts"
)

// HLSArguments creates a default set of arguments for ffmpeg HLS encoding.
func HLSArguments() Arguments {
	return Arguments{
		defaultArgs: []Argument{
			{"threads", "2"},
			{"preset", "superfast"},
			{"keyint_min", keyint},
			{"g", keyint},
			{"sc_threshold", "0"},
			{"c:v", videoCodec},
			{"pix_fmt", "yuv420p"},
			{"crf", constantRateFactor},
			// Stream map items go here (in `GetStrArguments`)
			{"c:a", "aac"},
			{"b:a", "128k"},
			{"ac", "2"},
			{"ar", "44100"},
			{"f", "hls"},
			{"hls_time", hlsTime},
			{"hls_playlist_type", "vod"},
			{"hls_flags", "independent_segments"},
			{"master_pl_name", MasterPlaylist},
			// hls_segment_filename goes here
			// var_stream_map goes here
			{"strftime_mkdir", "1"},
		},
	}
}

// TSArguments creates a default set of arguments for ffmpeg TS encoding.
func TSArguments() Arguments {
	return Arguments{
		defaultArgs: []Argument{
			{"threads", "2"},
			{"keyint_min", keyint},
			{"g", keyint},
			{"sc_threshold", "0.2"},
			{"c:v", "h264"},
			// Stream map items go here (in `GetStrArguments`)
			{"c:a", "aac"},
			{"ac", "2"},
			{"f", "mpegts"},
		},
	}
}

func NewArguments(out string, target Target, fps int) (Arguments, error) {
	var args Arguments
	if len(target.Formats) == 0 {
		return args, errors.New("no target formats supplied")
	}

	switch target.Type {
	case TargetTypeHLS:
		args = HLSArguments()
	case TargetTypeTS:
		args = TSArguments()
	default:
		// for backward compatibility, by default treat empty target
		// type as HLS Stream.
		args = HLSArguments()
	}

	args.formats = target.Formats
	args.out = out
	args.fps = fps

	return args, nil
}

// GetStrArguments serializes ffmpeg arguments in a format sutable for cmd.Start.
func (a Arguments) GetStrArguments() []string {
	strArgs := []string{}

	opts := a.defaultArgs
	formatOpts := []Argument{}
	varStream := []string{}

	for i, f := range a.formats {
		varStream = append(varStream, fmt.Sprintf("v:%v,a:%v", i, i))

		formatOpts = append(formatOpts, Argument{"map", "v:0"})
		formatOpts = append(formatOpts, Argument{fmt.Sprintf("filter:%v", i), fmt.Sprintf(`scale=-2:%v`, f.Resolution.Height)})
		// Instead of using a encoding quality factor "-crf" I have to use a set bitrate with "-b:v:X" for every stream
		// and omit the "-crf" statement. To fine-tune the we can change the -bufsize:v:X between 150% of the set bitrate
		// to 200% of the set bitrate. I took 175%. The bufsize is the area in witch the bitrate is calculated and adjusted by the encoder.
		formatOpts = append(formatOpts, Argument{fmt.Sprintf("maxrate:%v", i), fmt.Sprintf("%vk", f.GetBitrateForFPS(a.fps))})
		formatOpts = append(formatOpts, Argument{fmt.Sprintf("bufsize:%v", i), fmt.Sprintf("%vk", f.GetBitrateForFPS(a.fps)*2)})
	}
	for range a.formats {
		formatOpts = append(formatOpts, Argument{"map", "a:0"})
	}

	opts = append(opts[:6], append(formatOpts, opts[6:]...)...)
	opts = append(opts, Argument{"hls_segment_filename", "seg_%v_%06d.ts"})
	opts = append(opts, Argument{"var_stream_map", strings.Join(varStream, " ")})

	for _, v := range opts {
		if v[1] != "" {
			strArgs = append(strArgs, fmt.Sprintf("-%v", v[0]), v[1])
		} else {
			strArgs = append(strArgs, v[0])
		}
	}
	return strArgs
}
