package formats

const (
	TypeHLS   = "hls"
	TypeDASH  = "dash"
	TypeRange = "range"
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
)

var Resolutions = []Resolution{
	UHD4K, QHD2K, HD1080, HD720, SD480, SD360, SD240,
}

// H264 codec with its suggested bitrates
var H264 = Codec{
	Format{UHD4K, Bitrate{FPS30: 23000, FPS60: 35000}},
	Format{QHD2K, Bitrate{FPS30: 12000, FPS60: 18000}},
	Format{HD1080, Bitrate{FPS30: 2300, FPS60: 3500}},
	Format{HD720, Bitrate{FPS30: 1400, FPS60: 2200}},
	Format{SD480, Bitrate{FPS30: 1100, FPS60: 1700}},
	Format{SD360, Bitrate{FPS30: 525, FPS60: 800}},
	Format{SD240, Bitrate{FPS30: 250, FPS60: 380}},
}

var TargetResolutions = map[Resolution][]Resolution{
	UHD4K:  {UHD4K, QHD2K, HD1080, HD720, SD480},
	QHD2K:  {QHD2K, HD1080, HD720, SD480},
	HD1080: {HD1080, HD720, SD480, SD360},
	HD720:  {HD720, SD480, SD240},
	SD480:  {SD480, SD240},
	SD360:  {SD360},
	SD240:  {SD240},
}

var brResolutionFactor = 800

func TargetFormats(codec Codec, w, h int) []Format {
	res := Resolution{Height: h}
	for _, r := range Resolutions {
		if h == r.Height {
			res = r
		}
	}

	formats := []Format{}
	// for _, tr := range TargetResolutions[res] {
	// 	fs = append(fs, codec[tr])
	// }
	for _, f := range codec {
		if res.Height >= f.Resolution.Height {
			formats = append(formats, f)
		}
	}
	hasSelf := false
	for _, f := range formats {
		if f.Resolution == res {
			hasSelf = true
		}
	}
	if !hasSelf {
		formats = append(formats, codec.Format(Resolution{Width: w, Height: h}))
	}
	return formats
}

func (c Codec) Format(r Resolution) Format {
	for _, f := range c {
		if f.Resolution == r {
			return f
		}
	}
	br := Bitrate{
		FPS30: (r.Width * r.Height) / brResolutionFactor,
		FPS60: int((float32(r.Width) * float32(r.Height) * 1.56) / float32(brResolutionFactor)),
	}
	return Format{Resolution: r, Bitrate: br}
}
