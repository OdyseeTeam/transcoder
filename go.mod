module github.com/lbryio/transcoder

go 1.15

require (
	github.com/alecthomas/kong v0.2.12
	github.com/aws/aws-sdk-go v1.36.29
	github.com/c2h5oh/datasize v0.0.0-20200825124411-48ed595a09d2
	github.com/draganm/miniotest v0.1.0
	github.com/fasthttp/router v1.3.3
	github.com/floostack/transcoder v1.2.0
	github.com/grafov/m3u8 v0.11.1
	github.com/karlseguin/ccache/v2 v2.0.8
	github.com/karrick/godirwalk v1.16.1
	github.com/lbryio/lbry.go/v2 v2.6.0
	github.com/mattn/go-sqlite3 v1.14.4
	github.com/orcaman/concurrent-map v0.0.0-20190826125027-8c72a8bb44f6
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.8.0
	github.com/simukti/sqldb-logger v0.0.0-20201125162808-c35f87e285f2
	github.com/simukti/sqldb-logger/logadapter/zapadapter v0.0.0-20201125162808-c35f87e285f2
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.6.1
	github.com/valyala/fasthttp v1.17.0
	go.uber.org/zap v1.16.0
)

replace github.com/floostack/transcoder => github.com/andybeletsky/transcoder v1.2.0
