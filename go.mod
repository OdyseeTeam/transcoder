module github.com/lbryio/transcoder

go 1.15

require (
	github.com/alecthomas/kong v0.2.12
	github.com/fasthttp/router v1.3.3
	github.com/floostack/transcoder v1.2.0
	github.com/golang/protobuf v1.4.3 // indirect
	github.com/grafov/m3u8 v0.11.1
	github.com/karlseguin/ccache/v2 v2.0.7
	github.com/karrick/godirwalk v1.16.1
	github.com/lbryio/lbry.go/v2 v2.6.0
	github.com/mattn/go-sqlite3 v1.14.4
	github.com/orcaman/concurrent-map v0.0.0-20190826125027-8c72a8bb44f6
	github.com/pkg/errors v0.9.1
	github.com/simukti/sqldb-logger v0.0.0-20201125162808-c35f87e285f2
	github.com/simukti/sqldb-logger/logadapter/zapadapter v0.0.0-20201125162808-c35f87e285f2
	github.com/sirupsen/logrus v1.6.0 // indirect
	github.com/stretchr/testify v1.6.1
	github.com/valyala/fasthttp v1.17.0
	go.uber.org/zap v1.16.0
	golang.org/x/sys v0.0.0-20201015000850-e3ed0017c211 // indirect
	golang.org/x/tools v0.0.0-20200103221440-774c71fcf114 // indirect
	gopkg.in/yaml.v2 v2.3.0 // indirect
)

replace github.com/floostack/transcoder => github.com/andybeletsky/transcoder v1.2.0
