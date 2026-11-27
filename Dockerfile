# syntax=docker/dockerfile:1@sha256:572cebbc2b15e6f84c077742952674889cb63cd5e9b80abdffcbe587b6f19183
# Source-build image for local `make docker` and forks.
# Official Unpackerr/unpackerr releases copy a prebuilt binary via
# init/docker/Dockerfile.goreleaser.

FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder

# Node is required: go generate ./frontend runs npm ci + vite (embedded SPA).
RUN apk add --no-cache git nodejs npm

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY frontend/package.json frontend/package-lock.json ./frontend/
RUN npm ci --prefix frontend

COPY main.go ./
COPY pkg pkg
COPY examples examples
COPY init/config init/config
COPY frontend frontend
RUN go generate ./...

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION=development
ARG REVISION=0
ARG BRANCH=unknown
ARG COMMIT=unknown
ARG BUILD_DATE
ARG BUILD_USER=docker

ENV CGO_ENABLED=0
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}

RUN GOARM="${TARGETVARIANT#v}" go build -trimpath -tags osusergo,netgo \
    -ldflags "-s -w \
    -X golift.io/version.Version=${VERSION} \
    -X golift.io/version.Revision=${REVISION} \
    -X \"golift.io/version.Branch=${BRANCH} (${COMMIT})\" \
    -X golift.io/version.BuildDate=${BUILD_DATE} \
    -X golift.io/version.BuildUser=${BUILD_USER}" \
    -o /unpackerr

FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

RUN apk add --no-cache ca-certificates openssl tzdata curl jq

COPY --from=builder --chmod=755 /unpackerr /unpackerr

ENV TZ=UTC

EXPOSE 5656
ENTRYPOINT ["/unpackerr"]
