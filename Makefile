CC=x86_64-linux-musl-gcc
CXX=x86_64-linux-musl-g++
GOARCH=amd64
GOOS=linux
CGO_ENABLED=1
LDFLAGS = "-linkmode external -extldflags -static"
GO_BUILD = CC=$(CC) CXX=$(CXX) GOARCH=$(GOARCH) GOOS=$(GOOS) CGO_ENABLED=$(CGO_ENABLED) go build -ldflags $(LDFLAGS)
BUILD_DIR = dist/linux_amd64

linux:
	CC=$(CC) CXX=$(CXX) GOARCH=$(GOARCH) GOOS=$(GOOS) CGO_ENABLED=$(CGO_ENABLED) \
  	go build -ldflags "-linkmode external -extldflags -static" -o dist/linux_amd64/transcoder

.PHONY: tower
tower:
	$(GO_BUILD) -o $(BUILD_DIR)/tower ./tower/server/cmd/

worker:
	$(GO_BUILD) -o $(BUILD_DIR)/worker ./tower/worker/cmd/
