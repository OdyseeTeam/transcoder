package ladder

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

// Commonly defined resolutions.
var (
	UHD4K  = Resolution{Height: 2160, Width: 3840}
	QHD2K  = Resolution{Height: 1440, Width: 2560}
	HD1080 = Resolution{Height: 1080, Width: 1920}
	HD720  = Resolution{Height: 720, Width: 1280}
	SD360  = Resolution{Height: 360, Width: 640}
	SD240  = Resolution{Height: 240, Width: 320}
	SD144  = Resolution{Height: 144, Width: 256}
)

var Resolutions = []Resolution{
	UHD4K, QHD2K, HD1080, HD720, SD360, SD240, SD144,
}

// H264 ladder.
var H264 = Codec{
	Format{UHD4K, Bitrate{FPS30: 18000}},
	Format{QHD2K, Bitrate{FPS30: 10000}},
	Format{HD1080, Bitrate{FPS30: 3500}},
	Format{HD720, Bitrate{FPS30: 2500}},
	// Format{SD480, Bitrate{FPS30: 900, FPS60: 1700}},
	Format{SD360, Bitrate{FPS30: 500}},
	Format{SD144, Bitrate{FPS30: 100}},
}
