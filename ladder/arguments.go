package ladder

import (
	"fmt"
	"strconv"

	"github.com/shopspring/decimal"
)

const (
	MasterPlaylist     = "master.m3u8"
	preset             = "veryfast"
	videoCodec         = "libx264"
	constantRateFactor = "26"
	hlsTime            = "10"
)

const (
	argVarStreamMap = "var_stream_map"
)

type ArgumentSet struct {
	Output    string
	Ladder    Ladder
	Arguments map[string]string
	Metadata  *Metadata
}

var hlsDefaultArguments = map[string]string{
	"preset":       preset,
	"sc_threshold": "0",
	"c:v":          "libx264",
	"pix_fmt":      "yuv420p",
	// "crf":                  constantRateFactor,
	"c:a":                  "aac",
	"ac":                   "2",
	"ar":                   "44100",
	"f":                    "hls",
	"hls_time":             hlsTime,
	"hls_playlist_type":    "vod",
	"hls_flags":            "independent_segments",
	"master_pl_name":       MasterPlaylist,
	"strftime_mkdir":       "1",
	"hls_segment_filename": "v%v_s%06d.ts",
}

// GetStrArguments serializes ffmpeg arguments in a format sutable for `ffmpeg.Transcoder.Start“.
func (a *ArgumentSet) GetStrArguments() []string {
	strArgs := []string{}

	args := a.Arguments
	ladArgs := []string{}
	args[argVarStreamMap] = ""

	for k, v := range a.Ladder.Args {
		args[k] = v
	}

	for n, tier := range a.Ladder.Tiers {
		s := strconv.Itoa(n)
		args[argVarStreamMap] += fmt.Sprintf("v:%s,a:%s ", s, s)
		vRate := strconv.Itoa(tier.VideoBitrate)
		ladArgs = append(ladArgs,
			"-map", "v:0",
			"-filter:v:"+s, "scale=-2:"+strconv.Itoa(tier.Height),
			"-b:v:"+s, vRate,
			"-maxrate:v:"+s, vRate,
			"-bufsize:v:"+s, vRate,
		)

		switch {
		case tier.KeepFramerate:
			ladArgs = append(ladArgs,
				"-g:v:"+s, strconv.Itoa(a.Metadata.FPS.Int()*2)) // nolint:goconst
		case !tier.Framerate.IsZero():
			ladArgs = append(ladArgs,
				"-r:v:"+s, tier.Framerate.String(),
				"-g:v:"+s, (tier.Framerate.Mul(decimal.NewFromInt(2)).String())) // nolint:goconst
		default:
			ladArgs = append(ladArgs,
				"-r:v:"+s, a.Metadata.FPS.String(),
				"-g:v:"+s, strconv.Itoa(a.Metadata.FPS.Int()*2)) // nolint:goconst
		}
		ladArgs = append(ladArgs, "-map", "a:0", "-b:a:"+s, tier.AudioBitrate)
	}

	for k, v := range args {
		strArgs = append(strArgs, fmt.Sprintf("-%v", k), v)
	}
	strArgs = append(strArgs, ladArgs...)
	return strArgs
}
