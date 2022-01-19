package ladder

import (
	"math"
	"strconv"

	"gopkg.in/yaml.v3"
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
func (l Ladder) Tweak(m *Metadata) (Ladder, error) {
	logger.Debugw("m.VideoStream.GetBitRate()", "rate", m.VideoStream.GetBitRate())
	vrate, _ := strconv.Atoi(m.VideoStream.GetBitRate())
	var vert, origResSeen bool
	w := m.VideoStream.GetWidth()
	h := m.VideoStream.GetHeight()
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

	if !origResSeen && l.Tiers[0].Height >= h {
		tweakedTiers = append([]Tier{{
			Height:       h,
			Width:        w,
			VideoBitrate: nsRate(w, h),
			AudioBitrate: "128k",
		}}, tweakedTiers...)
	}

	l.Tiers = tweakedTiers
	return l, nil
}

func nsRate(w, h int) int {
	return int(math.Ceil(float64(800*600) / nsRateFactor))
}
