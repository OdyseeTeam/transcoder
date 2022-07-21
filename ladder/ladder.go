package ladder

import (
	"math"
	"strconv"

	"gopkg.in/yaml.v3"
)

const (
	FPS30 = 30
	FPS60 = 60
)

type Definition string

type Ladder struct {
	Args  map[string]string
	Tiers []Tier `yaml:",flow"`
}

type Tier struct {
	Definition    Definition
	Height        int
	Width         int
	VideoBitrate  int    `yaml:"bitrate"`
	AudioBitrate  string `yaml:"audio_bitrate"`
	Framerate     int    `yaml:",omitempty"`
	BitrateCutoff int    `yaml:"bitrate_cutoff"`
}

func Load(yamlLadder []byte) (Ladder, error) {
	l := Ladder{}
	err := yaml.Unmarshal(yamlLadder, &l)
	return l, err
}

// Tweak modifies existing ladder according to supplied video metadata
func (l Ladder) Tweak(meta *Metadata) (Ladder, error) {
	vrate, _ := strconv.Atoi(meta.VideoStream.GetBitRate())
	var vert, origResSeen bool
	w := meta.VideoStream.GetWidth()
	h := meta.VideoStream.GetHeight()
	if h > w {
		vert = true
	}
	tweakedTiers := []Tier{}
	for _, t := range l.Tiers {
		if t.BitrateCutoff >= vrate {
			logger.Debugw("video bitrate lower than the cut-off", "bitrate", vrate, "cutoff", t.BitrateCutoff)
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
		tweakedTiers = append(tweakedTiers, t)
	}

	if !origResSeen && l.Tiers[0].Height >= h && len(tweakedTiers) > 0 {
		tweakedTiers = append([]Tier{{
			Height:       h,
			Width:        w,
			VideoBitrate: nsRate(w, h),
			AudioBitrate: "128k",
		}}, tweakedTiers...)
	}

	l.Tiers = tweakedTiers
	logger.Debugw("ladder built", "tiers", l.Tiers)
	return l, nil
}

func (l Ladder) ArgumentSet(out string, meta *Metadata) *ArgumentSet {
	d := map[string]string{}
	for k, v := range hlsDefaultArguments {
		d[k] = v
	}
	return &ArgumentSet{
		Output:    out,
		Arguments: d,
		Ladder:    l,
		Meta:      meta,
	}
}

func nsRate(w, h int) int {
	return int(math.Ceil(float64(800*600) / nsRateFactor))
}
