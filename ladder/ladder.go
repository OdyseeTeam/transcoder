package ladder

import (
	"math"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	yaml "gopkg.in/yaml.v3"
)

type Definition string

type Ladder struct {
	Args     map[string]string
	Metadata *Metadata
	Tiers    []Tier `yaml:",flow"`
}

type Tier struct {
	Definition    Definition
	Height        int
	Width         int
	VideoBitrate  int             `yaml:"bitrate"`
	AudioBitrate  string          `yaml:"audio_bitrate"`
	Framerate     decimal.Decimal `yaml:",omitempty"`
	KeepFramerate bool            `yaml:"keep_framerate"`
	BitrateCutoff int             `yaml:"bitrate_cutoff"`
	CRF           int
	AudioChannels int
}

func Load(yamlLadder []byte) (Ladder, error) {
	l := Ladder{}
	err := yaml.Unmarshal(yamlLadder, &l)
	return l, err
}

// Tweak generates encoding parameters from the ladder for provided video metadata.
func (x Ladder) Tweak(md *Metadata) (Ladder, error) {
	newLadder := Ladder{
		Args:     x.Args,
		Tiers:    []Tier{},
		Metadata: md,
	}
	originalBitrate, _ := strconv.Atoi(md.VideoStream.GetBitRate())
	originalAudioChannels := md.AudioStream.Cha
	var vert, origResSeen bool
	w := md.VideoStream.GetWidth()
	h := md.VideoStream.GetHeight()
	if h > w {
		vert = true
	}
	for _, t := range x.Tiers {
		if t.BitrateCutoff >= originalBitrate {
			logger.Debugw("video bitrate lower than cut-off", "bitrate", originalBitrate, "cutoff", t.BitrateCutoff)
			if t.Height == h {
				origResSeen = true
			}
			continue
		}
		if vert {
			t.Width, t.Height = t.Height, t.Width
		}
		if t.Height > h {
			logger.Debugw("tier definition higher than stream", "tier", t.Height, "height", h)
			continue
		}
		if t.Height == h {
			origResSeen = true
		}
		if t.CRF == 0 {
			t.CRF = DefaultCRF
		}
		newLadder.Tiers = append(newLadder.Tiers, t)
	}

	if !origResSeen && x.Tiers[0].Height >= h && len(newLadder.Tiers) > 0 {
		newLadder.Tiers = append([]Tier{{
			Height:       h,
			Width:        w,
			VideoBitrate: nsRate(w, h),
			AudioBitrate: "128k",
			CRF:          DefaultCRF,
		}}, newLadder.Tiers...)
	}

	logger.Debugw("ladder built", "tiers", newLadder.Tiers)
	return newLadder, nil
}

func (x Ladder) ArgumentSet(output string) *ArgumentSet {
	d := map[string]string{}
	for k, v := range hlsDefaultArguments {
		d[k] = v
	}
	return &ArgumentSet{
		Output:    output,
		Arguments: d,
		Ladder:    x,
		Metadata:  x.Metadata,
	}
}

func (x Ladder) String() string {
	return strings.Join(x.ArgumentSet("...").GetStrArguments(), " ")
}

func nsRate(w, h int) int {
	return int(math.Ceil(float64(800*600) / nsRateFactor))
}
