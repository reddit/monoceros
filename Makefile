.PHONY: test build install lint clean fmt processbin

CGO_ENABLED ?= 0

BRANCH    ?= $(shell git rev-parse --abbrev-ref HEAD)
BUILDDATE ?= $(shell date +"%Y-%m-%dT%H:%M:%S%z")
REVISION  ?= $(shell git rev-parse HEAD)
VERSION   ?= $(shell git log --date=short --pretty=format:'%h@%cd' -n 1 .)

VERSION_LDFLAGS := \
  -X github.com/prometheus/common/version.Branch=$(BRANCH) \
  -X github.com/prometheus/common/version.BuildDate=$(BUILDDATE) \
  -X github.com/prometheus/common/version.Revision=$(REVISION) \
  -X github.com/prometheus/common/version.Version=$(VERSION)

test: processbin
	go test -race ./...

test-ci: processbin
	go test -race ./... -count=30

test-unit: processbin
	CGO_ENABLED=$(CGO_ENABLED) go test -short -timeout 15s ./...

processbin:
	CGO_ENABLED=$(CGO_ENABLED) go build  -o dist/processbin ./cmd/processbin

build:
	CGO_ENABLED=$(CGO_ENABLED) go build -ldflags "$(VERSION_LDFLAGS)" -o dist/monoceros ./cmd/monoceros

build-race:
	CGO_ENABLED=$(CGO_ENABLED) go build -race -ldflags "$(VERSION_LDFLAGS)" -o dist/monoceros-race ./cmd/monoceros

install:
	CGO_ENABLED=$(CGO_ENABLED) go install -ldflags "$(VERSION_LDFLAGS)" ./cmd/monoceros

lint:
	go mod tidy
	go fmt ./...
	golangci-lint run --verbose

clean:
	rm ./dist/monoceros-*
