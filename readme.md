# Transcoder Server/Worker for Odysee

[![Go Report Card](https://goreportcard.com/badge/github.com/odyseeteam/transcoder)](https://goreportcard.com/report/github.com/odyseeteam/transcoder)
![Test Status](https://github.com/OdyseeTeam/transcoder/workflows/Test/badge.svg)

## Development

Requires go 1.22.

## Building

To build the x86 Linux binary, which is used both for `conductor` (controller part) and `cworker` (transcoding worker part):

```
make transcoder
```

#### Building docker images

```
make conductor_image cworker_image
```

## Versioning

This project is using [SemVer](https://semver.org) YY.MM.MINOR[.MICRO] for `client` package and [CalVer](https://calver.org) YY.MM.MINOR for `transcoder` releases since February 2024:

```
git tag transcoder-v24.2.0
```

## Contributing

Please ensure that your code builds, passes `golanci-lint` and automated tests run successfully before pushing your branch.

## License

This project is MIT licensed. For the full license, see [LICENSE](LICENSE).

## Security

We take security seriously. Please contact security@odysee.com regarding any issues you may encounter.

## Contact

The primary contact for this project is [@anbsky](https://github.com/anbsky) (andrey.beletsky@odysee.com).
