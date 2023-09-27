CC=x86_64-linux-musl-gcc
CXX=x86_64-linux-musl-g++
GOARCH=amd64
GOOS=linux
LDFLAGS=-ldflags "-linkmode external -extldflags -static"
GO_BUILD=go1.17 build
BUILD_DIR=dist
LOCAL_ARCH=$(shell uname)
VERSION := $(shell git describe --tags --match 'v*'|sed -e 's/v//')

transcoder: $(BUILD_DIR)/$(GOOS)_$(GOARCH)/transcoder
	GOARCH=$(GOARCH) GOOS=$(GOOS) CGO_ENABLED=0 \
  	$(GO_BUILD) -o $(BUILD_DIR)/$(GOOS)_$(GOARCH)/transcoder \
	  -ldflags "-s -w -X github.com/lbryio/transcoder/internal/version.Version=$(VERSION)" \
	  ./pkg/conductor/cmd/

conductor_image:
	docker buildx build -f Dockerfile-conductor -t odyseeteam/transcoder-conductor:dev --platform linux/amd64 .

cworker_image:
	docker buildx build -f Dockerfile-cworker -t odyseeteam/transcoder-cworker:dev --platform linux/amd64 .

test_down:
	docker-compose down

test_prepare:
	docker-compose up -d minio db redis
	docker-compose up -d cworker conductor
	docker-compose up minio-prepare

test: test_prepare
	go test -covermode=count -coverprofile=coverage.out ./...

towerz:
	docker run --rm -v "$(PWD)":/usr/src/transcoder -w /usr/src/transcoder --platform linux/amd64 golang:1.16.10 make tower

.PHONY: tccli
tccli:
	GOARCH=$(GOARCH) GOOS=$(GOOS) CGO_ENABLED=0 \
  	$(GO_BUILD) -o $(BUILD_DIR)/$(GOOS)_$(GOARCH)/tccli \
	  -ldflags "-s -w -X github.com/lbryio/transcoder/internal/version.Version=$(VERSION)" \
	  ./tccli/

tccli_mac:
	CGO_ENABLED=0 go build -o dist/arm64_darwin/tccli ./tccli

clean:
	rm -rf $(BUILD_DIR)/*
