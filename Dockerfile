# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
# Source-build image for local `make docker` and forks.
# Official Unpackerr/unpackerr releases copy a prebuilt binary via
# init/docker/Dockerfile.goreleaser.

FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS builder

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

FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

RUN apk add --no-cache ca-certificates openssl tzdata curl jq

COPY --from=builder --chmod=755 /unpackerr /unpackerr

ENV TZ=UTC

EXPOSE 5656
ENTRYPOINT ["/unpackerr"]
