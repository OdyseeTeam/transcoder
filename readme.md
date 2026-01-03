# Transcoder Server/Worker for Odysee

[![Go Report Card](https://goreportcard.com/badge/github.com/OdyseeTeam/transcoder)](https://goreportcard.com/report/github.com/OdyseeTeam/transcoder)
![Test Status](https://github.com/OdyseeTeam/transcoder/workflows/Test/badge.svg)

## Development

Requires go 1.25.

## Building

To build the x86 Linux binary, which is used both for `conductor` (controller part) and `cworker` (transcoding worker part):

```
make transcoder
```

#### Docker images

```
make conductor_image cworker_image
```

This will build and tag images with a version tag, as well as the `latest`. To push latest images:

```
docker push odyseeteam/transcoder-conductor:latest
docker push odyseeteam/transcoder-cworker:latest
```

`cworker` image is using ffmpeg image as a base. To update or rebuild it, see [its dockerfile](./docker/Dockerfile-ffmpeg) and run:

```
make ffmpeg_image
```

## Versioning

This project is using [SemVer](https://semver.org) YY.MM.MINOR[.MICRO] for `client` package and [CalVer](https://calver.org) YY.MM.MINOR for `transcoder` releases since February 2024:

```
git tag transcoder-v24.2.0
```

## Tools

To download a regular stream and produce a transcoded copy locally:

```
docker run -v $(pwd):$(pwd) -w $(pwd) odyseeteam/transcoder-tccli transcode "lbry://@specialoperationstest#3/fear-of-dea
th-inspirational#a"
```

Check `./tccli/main.go` for more commands.

## Contributing

Please ensure that your code builds, passes `golanci-lint` and automated tests run successfully before pushing your branch.

## License

This project is MIT licensed. For the full license, see [LICENSE](LICENSE).

## Security

We take security seriously. Please contact security@odysee.com regarding any issues you may encounter.

## Contact

The primary contact for this project is [@anbsky](https://github.com/anbsky) (andrey.beletsky@odysee.com).
