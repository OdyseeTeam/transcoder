LOCAL_ARCH ?= $(shell uname)
VERSION ?= $(shell git describe --tags --match 'v*'|sed -e 's/v//')
TRANSCODER_VERSION ?= $(shell git describe --tags --match 'transcoder-v*'|sed 's/transcoder-v\([0-9.]*\).*/\1/')
BUILD_DIR ?= dist
GOOS ?= linux
GOARCH ?= amd64
GO_BUILD ?= go build

transcoder:
	GOARCH=$(GOARCH) GOOS=$(GOOS) CGO_ENABLED=0 \
  	$(GO_BUILD) -o $(BUILD_DIR)/$(GOOS)_$(GOARCH)/transcoder \
	  -ldflags "-s -w -X github.com/OdyseeTeam/transcoder/internal/version.Version=$(TRANSCODER_VERSION)" \
	  ./pkg/conductor/cmd/

conductor_image: $(BUILD_DIR)/$(GOOS)_$(GOARCH)/transcoder
	docker buildx build -f docker/Dockerfile-conductor -t odyseeteam/transcoder-conductor:$(TRANSCODER_VERSION) --platform linux/amd64 .
	docker tag odyseeteam/transcoder-conductor:$(TRANSCODER_VERSION) odyseeteam/transcoder-conductor:latest

cworker_image: $(BUILD_DIR)/$(GOOS)_$(GOARCH)/transcoder
	docker buildx build -f docker/Dockerfile-cworker -t odyseeteam/transcoder-cworker:$(TRANSCODER_VERSION) --platform linux/amd64 .
	docker tag odyseeteam/transcoder-cworker:$(TRANSCODER_VERSION) odyseeteam/transcoder-cworker:latest

ffmpeg_image:
	docker buildx build -f docker/Dockerfile-ffmpeg -t odyseeteam/transcoder-ffmpeg:git --platform linux/amd64 .

tccli_image:
	docker buildx build -f docker/Dockerfile-tccli -t odyseeteam/transcoder-tccli:latest --platform linux/amd64 .

test_clean:
	docker-compose down
	docker volume rm -f transcoder_minio-data
	docker volume rm -f transcoder_redis-data
	docker volume rm -f transcoder_db-data

test_prepare:
	make transcoder
	make conductor_image
	make cworker_image
	docker compose -p transcoder up -d minio db redis
	docker compose -p transcoder up -d cworker conductor
	docker compose -p transcoder up minio-prepare

test: test_prepare
	go test -covermode=count -coverprofile=coverage.out ./...

towerz:
	docker run --rm -v "$(PWD)":/usr/src/transcoder -w /usr/src/transcoder --platform linux/amd64 golang:1.16.10 make tower

.PHONY: tccli
tccli:
	GOARCH=$(GOARCH) GOOS=$(GOOS) CGO_ENABLED=0 \
  	$(GO_BUILD) -o $(BUILD_DIR)/$(GOOS)_$(GOARCH)/tccli \
	  -ldflags "-s -w -X github.com/OdyseeTeam/transcoder/internal/version.Version=$(TRANSCODER_VERSION)" \
	  ./tccli/

tccli_mac:
	CGO_ENABLED=0 go build -o dist/arm64_darwin/tccli ./tccli

clean:
	rm -rf $(BUILD_DIR)/*
