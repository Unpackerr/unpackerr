# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32
# Source-build image for local `make docker` and forks.
# Official Unpackerr/unpackerr releases copy a prebuilt binary via
# init/docker/Dockerfile.goreleaser.

FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS builder

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

FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

RUN apk add --no-cache ca-certificates openssl tzdata curl jq

COPY --from=builder --chmod=755 /unpackerr /unpackerr

ENV TZ=UTC

EXPOSE 5656
ENTRYPOINT ["/unpackerr"]
