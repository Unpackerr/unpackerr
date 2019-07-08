#
# building static go binary with golang container
#
FROM golang:stretch as builder
ARG ARCH=amd64
ARG OS=linux

RUN mkdir -p $GOPATH/pkg/mod $GOPATH/bin $GOPATH/src $GOPATH/src/github.com/davidnewhall

RUN apt-get update \
  && apt-get install -y curl  \
  && curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

COPY . $GOPATH/src/github.com/davidnewhall/unpacker-poller
WORKDIR $GOPATH/src/github.com/davidnewhall/unpacker-poller

RUN make dep --vendor-only \
  && CGO_ENABLED=0 make unpacker-poller.${ARCH}.${OS} \
  && mv unpacker-poller.${ARCH}.${OS} unpacker-poller
#
# To use this container run the following command:
#
# docker run -d -v /your/config/up.conf:/etc/unpacker-poller/up.conf golift/unpacker-poller
#
FROM scratch

COPY --from=builder /go/src/github.com/davidnewhall/unpacker-poller/unpacker-poller /unpacker-poller
COPY --from=builder /go/src/github.com/davidnewhall/unpacker-poller/examples/up.conf.example /etc/unpacker-poller/up.conf

VOLUME [ "/etc/unpacker-poller"]

ENTRYPOINT [ "/unpacker-poller" ]
