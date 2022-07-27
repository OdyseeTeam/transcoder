CC=x86_64-linux-musl-gcc
CXX=x86_64-linux-musl-g++
GOARCH=amd64
GOOS=linux
LDFLAGS=-ldflags "-linkmode external -extldflags -static"
GO_BUILD=go1.17 build
BUILD_DIR=dist/linux_amd64
LOCAL_ARCH=$(shell uname)
VERSION := $(shell git describe --tags --match 'v*'|sed -e 's/v//')

transcoder:
	GOARCH=$(GOARCH) GOOS=$(GOOS) CGO_ENABLED=0 \
  	$(GO_BUILD) -o $(BUILD_DIR)/transcoder \
	  -ldflags "-s -w -X github.com/lbryio/transcoder/internal/version.Version=$(VERSION)" \
	  ./pkg/conductor/cmd/

conductor_image: tower
	docker buildx build -f Dockerfile-conductor -t odyseeteam/transcoder-conductor:dev --platform linux/amd64 . --push

cworker_image: worker
	docker buildx build -f Dockerfile-cworker -t odyseeteam/transcoder-cworker:dev --platform linux/amd64 . --push

towerz:
	docker run --rm -v "$(PWD)":/usr/src/transcoder -w /usr/src/transcoder --platform linux/amd64 golang:1.16.10 make tower

.PHONY: tccli
tccli:
ifeq ($(LOCAL_ARCH),Darwin)
	CC=$(CC) CXX=$(CXX) GOARCH=$(GOARCH) GOOS=$(GOOS) \
	CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/tccli ./tccli
else
	CGO_ENABLED=1 go build -o $(BUILD_DIR)/tccli ./tccli
endif

tccli_mac:
	CGO_ENABLED=0 go build -o dist/arm64_darwin/tccli ./tccli

clean:
	rm -rf $(BUILD_DIR)/*
