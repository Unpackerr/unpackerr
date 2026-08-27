FROM alpine:3.22

RUN apk add --no-cache ca-certificates openssl tzdata

ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/unpackerr /unpackerr

ENV TZ=UTC

ENTRYPOINT ["/unpackerr"]
