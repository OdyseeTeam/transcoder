CC=x86_64-linux-musl-gcc
CXX=x86_64-linux-musl-g++
GOARCH=amd64
GOOS=linux
LDFLAGS=-ldflags "-linkmode external -extldflags -static"
GO_BUILD=GOARCH=$(GOARCH) GOOS=$(GOOS) go build
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
	CGO_ENABLED=1 $(GO_BUILD) -o $(BUILD_DIR)/tower ./tower/cmd/tower/
endif

.PHONY: tower
towerz:
	docker run --rm -v "$(PWD)":/usr/src/transcoder -w /usr/src/transcoder --platform linux/amd64 golang:1.16.10 make tower

worker:
	$(GO_BUILD) -o $(BUILD_DIR)/worker ./tower/cmd/worker/
