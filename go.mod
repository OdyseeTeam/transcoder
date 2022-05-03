module github.com/lbryio/transcoder

go 1.16

require (
	github.com/BurntSushi/toml v0.4.1 // indirect
	github.com/Pallinder/go-randomdata v1.2.0
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/alecthomas/kong v0.2.17
	github.com/aws/aws-sdk-go v1.36.29
	github.com/brk0v/directio v0.0.0-20190225130936-69406e757cf7
	github.com/c2h5oh/datasize v0.0.0-20200825124411-48ed595a09d2
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/draganm/miniotest v0.1.0
	github.com/fasthttp/router v1.3.3
	github.com/floostack/transcoder v1.2.0
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/grafov/m3u8 v0.11.1
	github.com/gramework/gramework v1.7.1 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/karlseguin/ccache/v2 v2.0.8
	github.com/karrick/godirwalk v1.16.1
	github.com/lbryio/lbry.go/v2 v2.7.2-0.20220208210038-a0391bec7915
	github.com/lbryio/reflector.go v1.1.3-0.20220209235713-2f7d67794f93 // indirect
	github.com/lib/pq v1.10.4
	github.com/mattn/go-colorable v0.1.11 // indirect
	github.com/mattn/go-sqlite3 v1.14.6
	github.com/minio/minio-go/v7 v7.0.7-0.20201217170524-3baf9ea06f7c // indirect
	github.com/nikooo777/lbry-blobs-downloader v1.0.9
	github.com/oklog/ulid/v2 v2.0.2
	github.com/onsi/gomega v1.16.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/rabbitmq/amqp091-go v1.3.0
	github.com/rubenv/sql-migrate v0.0.0-20211023115951-9f02b1e13857
	github.com/simukti/sqldb-logger v0.0.0-20201125162808-c35f87e285f2
	github.com/simukti/sqldb-logger/logadapter/zapadapter v0.0.0-20201125162808-c35f87e285f2
	github.com/spf13/cast v1.4.1 // indirect
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.7.0
	github.com/valyala/fasthttp v1.31.0
	github.com/wagslane/go-rabbitmq v0.7.2
	go.uber.org/goleak v1.1.10
	go.uber.org/zap v1.16.0
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519 // indirect
	golang.org/x/lint v0.0.0-20201208152925-83fdc39ff7b5 // indirect
	golang.org/x/net v0.0.0-20211008194852-3b03d305991f // indirect
	golang.org/x/sys v0.0.0-20220114195835-da31bd327af9 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/time v0.0.0-20211116232009-f0f3c7e86c11 // indirect
	golang.org/x/tools v0.1.6-0.20210908190839-cf92b39a962c // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	honnef.co/go/tools v0.2.0 // indirect
	logur.dev/logur v0.17.0
)

replace github.com/floostack/transcoder => github.com/andybeletsky/transcoder v1.2.1

// replace github.com/wagslane/go-rabbitmq => github.com/andybeletsky/go-rabbitmq v0.6.3-0.20220318172029-6b36934a7885

// replace github.com/nikooo777/lbry-blobs-downloader => /Users/silence/Documents/LBRY/Repos/lbry-blobs-downloader

//  replace github.com/floostack/transcoder => /Users/silence/Documents/LBRY/External/transcoder
