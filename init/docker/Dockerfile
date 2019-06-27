#
# building static go binary with golang container
#
FROM golang:stretch as builder

RUN mkdir -p $GOPATH/pkg/mod $GOPATH/bin $GOPATH/src

RUN apt-get update \
  && apt-get install -y curl  \
  && curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh \
  && mkdir -p $GOPATH/src/github.com/davidnewhall

  COPY . $GOPATH/src/github.com/davidnewhall/unpacker-poller
  WORKDIR $GOPATH/src/github.com/davidnewhall/unpacker-poller

RUN make dep \
  && CGO_ENABLED=0 make linux

#
# To use this container run the following command:
#
# docker run -d -v /your/config/up.conf:/etc/unpacker-poller/up.conf golift/unpacker-poller
#
FROM scratch

COPY --from=builder /go/src/github.com/davidnewhall/unpacker-poller/unpacker-poller.linux /unpacker-poller
COPY --from=builder /go/src/github.com/davidnewhall/unpacker-poller/examples/up.conf.example /etc/unpacker-poller/up.conf

VOLUME [ "/etc/unpacker-poller"]

ENTRYPOINT [ "/unpacker-poller" ]
