package ladder

var defaultLadderYaml = []byte(`
args:
  sws_flags: bilinear
  profile:v: main
  crf: 23
  refs: 1
  preset: veryfast
  force_key_frames: "expr:gte(t,n_forced*2)"
  hls_time: 6

tiers:
  - definition: 1080p
    bitrate: 3500_000
    bitrate_cutoff: 6000_000
    audio_bitrate: 160k
    width: 1920
    height: 1080
  - definition: 720p
    bitrate: 2500_000
    audio_bitrate: 128k
    width: 1280
    height: 720
  - definition: 360p
    bitrate: 500_000
    audio_bitrate: 96k
    width: 640
    height: 360
  - definition: 144p
    width: 256
    height: 144
    bitrate: 100_000
    audio_bitrate: 64k
    framerate: 15
`)

var Default, _ = Load(defaultLadderYaml)
