# Local development. Official releases are GoReleaser Pro — see .github/workflows/README.md.

VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
REVISION ?= $(shell git rev-list --count --all 2>/dev/null || echo 0)
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BRANCH   ?= $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:00Z)
IMAGE    ?= unpackerr:dev

UN_WEBSERVER_LISTEN_ADDR ?= 127.0.0.1:5656
UN_WEBSERVER_UI_PASSWORD ?= admin:supersecret123

BUILD_FLAGS = -tags osusergo,netgo
VERSION_LDFLAGS := -X \"golift.io/version.Branch=$(BRANCH) ($(COMMIT))\" \
	-X \"golift.io/version.BuildDate=$(DATE)\" \
	-X \"golift.io/version.BuildUser=$(shell whoami 2>/dev/null || echo unknown)\" \
	-X \"golift.io/version.Revision=$(REVISION)\" \
	-X \"golift.io/version.Version=$(VERSION)\"

.DEFAULT_GOAL := build

.PHONY: all build generate dev docker clean

all: build

generate:
	go generate ./...

build: generate
	go build -trimpath $(BUILD_FLAGS) -o unpackerr -ldflags "-w -s $(VERSION_LDFLAGS) $(EXTRA_LDFLAGS)"

dev:
	USEGUI=false \
	UN_WEBSERVER_LISTEN_ADDR="$(UN_WEBSERVER_LISTEN_ADDR)" \
	UN_WEBSERVER_UI_PASSWORD="$(UN_WEBSERVER_UI_PASSWORD)" \
	UN_DEBUG=true \
	go run . $(ARGS)

docker:
	docker build \
		--build-arg VERSION="$(VERSION)" \
		--build-arg REVISION="$(REVISION)" \
		--build-arg BRANCH="$(BRANCH)" \
		--build-arg COMMIT="$(COMMIT)" \
		--build-arg BUILD_DATE="$(DATE)" \
		--build-arg BUILD_USER="$(shell whoami 2>/dev/null || echo unknown)" \
		-t "$(IMAGE)" .

clean:
	rm -f unpackerr unpackerr.*.{macos,freebsd,linux,exe}
