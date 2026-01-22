# Transcoder Server/Worker for Odysee

[![Go Report Card](https://goreportcard.com/badge/github.com/OdyseeTeam/transcoder)](https://goreportcard.com/report/github.com/OdyseeTeam/transcoder)
![Test Status](https://github.com/OdyseeTeam/transcoder/workflows/Test/badge.svg)
[![Docker conductor](https://img.shields.io/docker/v/odyseeteam/transcoder-conductor?label=conductor)](https://hub.docker.com/r/odyseeteam/transcoder-conductor)
[![Docker cworker](https://img.shields.io/docker/v/odyseeteam/transcoder-cworker?label=cworker)](https://hub.docker.com/r/odyseeteam/transcoder-cworker)
[![Docker tccli](https://img.shields.io/docker/v/odyseeteam/transcoder-tccli?label=tccli)](https://hub.docker.com/r/odyseeteam/transcoder-tccli)

A distributed video transcoding system for [Odysee](https://odysee.com). It retrieves videos from the LBRY network, transcodes them to multi-quality HLS streams (1080p, 720p, 360p, 144p), and stores the results in S3-compatible storage.

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   HTTP Client   │────▶│    Conductor    │────▶│  Redis (asynq)  │
└─────────────────┘     └─────────────────┘     └────────┬────────┘
                               │                         │
                               ▼                         ▼
                        ┌─────────────┐          ┌─────────────┐
                        │  PostgreSQL │          │   Workers   │
                        │  (Library)  │          │  (1..N)     │
                        └─────────────┘          └──────┬──────┘
                                                        │
                               ┌────────────────────────┼────────────────────────┐
                               ▼                        ▼                        ▼
                        ┌─────────────┐          ┌─────────────┐          ┌─────────────┐
                        │ LBRY Blobs  │          │   ffmpeg    │          │  S3 Storage │
                        │ (download)  │          │ (transcode) │          │  (upload)   │
                        └─────────────┘          └─────────────┘          └─────────────┘
```

**Conductor**: Central orchestrator that receives transcoding requests via HTTP API, queues jobs using asynq (Redis-backed), and dispatches work to available workers.

**Worker**: Distributed transcoding nodes that download source videos from LBRY, transcode to HLS using ffmpeg, and upload results to S3.

## Requirements

- Go 1.25+
- Redis
- PostgreSQL
- S3-compatible storage (AWS S3, Wasabi, MinIO)
- ffmpeg (included in worker Docker image)

## Configuration

### Conductor (`conductor.yml`)

```yaml
Storages:
  - Name: main
    Type: S3
    Endpoint: https://s3.amazonaws.com
    Region: us-east-1
    Bucket: transcoded-videos
    Key: ACCESS_KEY
    Secret: SECRET_KEY
    MaxSize: 1TB  # Auto-cleanup when exceeded

Library:
  DSN: postgres://user:pass@localhost/transcoder
  ManagerToken: your-api-token

Redis: redis://:password@localhost:6379/0

AdaptiveQueue:
  MinHits: 1
```

### Worker (`worker.yml`)

```yaml
Storage:
  Name: main
  Type: S3
  Endpoint: https://s3.amazonaws.com
  Region: us-east-1
  Bucket: transcoded-videos
  Key: ACCESS_KEY
  Secret: SECRET_KEY

Redis: redis://:password@localhost:6379/0

EdgeToken: lbry-edge-token

DiskPressure:
  Enabled: true
  Path: /tmp
  Threshold: 90
  CheckInterval: 10s
  MaxWait: 5m
```

## Building

```bash
# Build the transcoder binary (conductor + worker)
make transcoder

# Build tccli (local testing tool)
make tccli
```

## Docker Images

Docker images are automatically built and pushed to Docker Hub when a `transcoder-v*` tag is pushed.

| Image | Purpose |
|-------|---------|
| `odyseeteam/transcoder-conductor` | Central orchestrator |
| `odyseeteam/transcoder-cworker` | Transcoding worker with ffmpeg |
| `odyseeteam/transcoder-tccli` | CLI tool for local testing |
| `odyseeteam/transcoder-ffmpeg` | Base ffmpeg image for cworker |

Build locally:

```bash
make conductor_image cworker_image tccli_image
```

## Running

### With Docker Compose

```bash
docker compose up -d
```

### Manually

```bash
# Start conductor
./transcoder conductor --http-bind 0.0.0.0:8080

# Start worker(s)
./transcoder worker --concurrency 5 --streams-dir /tmp/streams --output-dir /tmp/output
```

## CLI Tool

The `tccli` tool is useful for local testing and debugging:

```bash
# Download and transcode a video locally
docker run -v $(pwd):$(pwd) -w $(pwd) odyseeteam/transcoder-tccli transcode "lbry://@channel/video"

# Validate a stream
docker run odyseeteam/transcoder-tccli validate-stream "lbry://@channel/video"

# Get video URL from transcoding server
docker run odyseeteam/transcoder-tccli get-video-url --server host:8080 "lbry://@channel/video"
```

## Versioning

This project uses [CalVer](https://calver.org) with format `YY.MM.MINOR`:

```bash
git tag transcoder-v26.1.0
git push origin transcoder-v26.1.0
```

## Testing

```bash
# Start test services (Redis, PostgreSQL, MinIO)
make test_prepare

# Run tests
go test ./...

# Cleanup
make test_clean
```

## Contributing

Please ensure your code:
- Builds successfully
- Passes `golangci-lint run`
- Has tests that pass

## License

This project is MIT licensed. See [LICENSE](LICENSE).

## Security

We take security seriously. Please contact security@odysee.com regarding any security issues.

## Contact

Primary contact: [@nikooo777](https://github.com/nikooo777) (niko@odysee.com)
