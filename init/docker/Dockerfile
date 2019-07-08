#
# building static go binary with golang container
#
ARG ARCH=amd64
ARG OS=linux
ARG BUILD_DATE=0
ARG COMMIT=0
ARG VERSION=development

FROM golang:stretch as builder
ARG ARCH
ARG OS

RUN mkdir -p $GOPATH/pkg/mod $GOPATH/bin $GOPATH/src $GOPATH/src/github.com/davidnewhall

RUN apt-get update \
  && apt-get install -y curl  \
  && curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

COPY . $GOPATH/src/github.com/davidnewhall/unpacker-poller
WORKDIR $GOPATH/src/github.com/davidnewhall/unpacker-poller

RUN dep ensure --vendor-only \
  && CGO_ENABLED=0 make unpacker-poller.${ARCH}.${OS}

FROM scratch
ARG ARCH
ARG OS
ARG BUILD_DATE
ARG COMMIT
ARG VERSION

# Build-time metadata as defined at https://github.com/opencontainers/image-spec/blob/master/annotations.md
LABEL org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.title="Unpacker Poller" \
      org.opencontainers.image.documentation="https://github.com/davidnewhall/unpacker-poller/wiki" \
      org.opencontainers.image.description="Extracts Deluge downloads so Radarr or Sonarr may import them." \
      org.opencontainers.image.url="https://github.com/davidnewhall/unpacker-poller" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.source="https://github.com/davidnewhall/unpacker-poller" \
      org.opencontainers.image.vendor="Go Lift" \
      org.opencontainers.image.authors="David Newhall II <david at sleepers dot pro>" \
      org.opencontainers.image.architecture="${OS} ${ARCH}" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=builder /go/src/github.com/davidnewhall/unpacker-poller/unpacker-poller.${ARCH}.${OS} /unpacker-poller
COPY --from=builder /go/src/github.com/davidnewhall/unpacker-poller/examples/up.conf.example /etc/unpacker-poller/up.conf

VOLUME [ "/etc/unpacker-poller"]

ENTRYPOINT [ "/unpacker-poller" ]
