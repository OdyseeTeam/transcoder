# Transcoder Server/Worker for Odysee

[![Go Report Card](https://goreportcard.com/badge/github.com/odyseeteam/transcoder)](https://goreportcard.com/report/github.com/odyseeteam/transcoder)
![Test Status](https://github.com/OdyseeTeam/transcoder/workflows/Test/badge.svg)

## Building

To build an x86-64 Linux binaries for tower (server part) and worker:

```
make tower worker
```

#### Building docker images

```
make tower_image_latest worker_image_latest
```

#### Prerequisites on MacOS

To build for Linux on macos, you need to have musl toolchain installed. Using homebrew:

```
brew install filosottile/musl-cross/musl-cross
```

On ARM Macs:

```
brew install richard-vd/musl-cross/musl-cross
brew install zstd
```

## Versioning

This project is using [SemVer](https://semver.org) YY.MM.MINOR[.MICRO].

## Contributing

Please ensure that your code builds and automated tests run successfully before pushing your branch. You must `go fmt` your code before you commit it, or the build will fail.


## License

This project is MIT licensed. For the full license, see [LICENSE](LICENSE).


## Security

We take security seriously. Please contact security@odysee.com regarding any issues you may encounter.


## Contact

The primary contact for this project is [@andybeletsky](https://github.com/andybeletsky) (andrey.beletsky@odysee.com).

