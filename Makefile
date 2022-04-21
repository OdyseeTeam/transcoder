CC=x86_64-linux-musl-gcc
CXX=x86_64-linux-musl-g++
GOARCH=amd64
GOOS=linux
LDFLAGS=-ldflags "-linkmode external -extldflags -static"
GO_BUILD=go build
BUILD_DIR=dist/linux_amd64
LOCAL_ARCH=$(shell uname)

transcoder:
	CC=$(CC) CXX=$(CXX) GOARCH=$(GOARCH) GOOS=$(GOOS) CGO_ENABLED=1 \
  	go build -ldflags "-linkmode external -extldflags -static" -o dist/linux_amd64/transcoder

.PHONY: tower
tower:
ifeq ($(LOCAL_ARCH),Darwin)
	CC=$(CC) CXX=$(CXX) GOARCH=$(GOARCH) GOOS=$(GOOS) \
	CGO_ENABLED=1 $(GO_BUILD) $(LDFLAGS) -o $(BUILD_DIR)/tower ./tower/cmd/tower/
else
	GOARCH=$(GOARCH) GOOS=$(GOOS) \
	CGO_ENABLED=1 $(GO_BUILD) -o $(BUILD_DIR)/tower ./tower/cmd/tower/
endif

towerz:
	docker run --rm -v "$(PWD)":/usr/src/transcoder -w /usr/src/transcoder --platform linux/amd64 golang:1.16.10 make tower

.PHONY: worker
worker:
	GOARCH=$(GOARCH) GOOS=$(GOOS) CGO_ENABLED=0 go build -o $(BUILD_DIR)/worker ./tower/cmd/worker/

.PHONY: tccli
tccli:
ifeq ($(LOCAL_ARCH),Darwin)
	CC=$(CC) CXX=$(CXX) GOARCH=$(GOARCH) GOOS=$(GOOS) \
	CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/tccli ./tccli
else
	CGO_ENABLED=1 go build -o $(BUILD_DIR)/tccli ./tccli
endif

tccli-mac:
	CGO_ENABLED=0 go build -o dist/arm64_darwin/tccli ./tccli

tower_image_latest:
	docker buildx build -f Dockerfile-tower -t odyseeteam/transcoder-tower:dev4 --platform linux/amd64 --push .

worker_image_latest:
	docker buildx build -f Dockerfile-worker -t odyseeteam/transcoder-worker:dev4 --platform linux/amd64 --push .
