# Canon — build, test and lint.
#
# CGO is disabled everywhere on purpose. A static binary with no runtime, no
# interpreter and no dependency install is what makes "one command, no external
# services" true rather than aspirational (ADR-0004).

BINARY  := canon
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

export CGO_ENABLED = 0

.PHONY: all build test vet fmt bench clean check

all: check build

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/canon

test:
	go test ./...

bench:
	go test -run '^$$' -bench . -benchmem ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

check: vet test

clean:
	rm -rf bin
