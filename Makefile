BINARY  := bin/switch
PKG     := ./cmd/switch
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: all build test vet fmt tidy clean install

all: build

build:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

install: build
	install -m 0755 $(BINARY) $${GOBIN:-$$HOME/go/bin}/switch

clean:
	rm -rf bin
