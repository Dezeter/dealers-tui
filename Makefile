# Makefile — convenience builds (cross-platform). Pure-Go, so CGO is off and no
# C toolchain is needed. On Windows prefer build.ps1.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PKG := ./cmd/dealers-tui

.PHONY: build windows linux test vet clean

build:            ## build for the host OS
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o dealers-tui $(PKG)

windows:          ## cross-build dealers-tui.exe
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dealers-tui.exe $(PKG)

linux:            ## cross-build the Linux amd64 binary
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dealers-tui-linux $(PKG)

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f dealers-tui dealers-tui.exe dealers-tui-linux
