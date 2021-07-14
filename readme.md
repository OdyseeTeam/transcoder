# Transcoder Server for LBRY network

## Building

To build an x86 Linux binary: `make linux`

## Configuring

Configuration is done via a file named transcoder.yml file (an [example](./transcoder.ex.yml)).

Some settings are available via command line options, for example:

```
go run . serve --debug --video-path=/tmp/transcoder --bind=:18081
```

Run `go run . serve --help` for a list of options.

## Running



## Versioning

This project is using [CalVer](https://calver.org) YY.MM.MINOR[.MICRO].

## Contributing

Contributions to this project are welcome, encouraged, and compensated. For more details, see [lbry.io/faq/contributing](https://lbry.io/faq/contributing).

Please ensure that your code builds and automated tests run successfully before pushing your branch. You must `go fmt` your code before you commit it, or the build will fail.


## License

This project is MIT licensed. For the full license, see [LICENSE](LICENSE).


## Security

We take security seriously. Please contact security@lbry.io regarding any issues you may encounter.
Our PGP key is [here](https://keybase.io/lbry/key.asc) if you need it.


## Contact

The primary contact for this project is [@andybeletsky](https://github.com/andybeletsky) (andrey@lbry.com).

