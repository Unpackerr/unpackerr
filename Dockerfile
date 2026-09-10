# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32
# Source-build image for local `make docker` and forks.
# Official Unpackerr/unpackerr releases copy a prebuilt binary via
# init/docker/Dockerfile.goreleaser.

FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY pkg pkg
COPY examples examples
COPY init/config init/config
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

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

RUN apk add --no-cache ca-certificates openssl tzdata curl jq

COPY --from=builder --chmod=755 /unpackerr /unpackerr

ENV TZ=UTC

EXPOSE 5656
ENTRYPOINT ["/unpackerr"]
