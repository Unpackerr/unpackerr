FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

RUN apk add --no-cache ca-certificates openssl tzdata

ARG TARGETPLATFORM
# Actions zip artifacts store every file as 0644; without --chmod the image
# entrypoint is not executable (OCI permission denied).
COPY --chmod=755 ${TARGETPLATFORM}/unpackerr /unpackerr

ENV TZ=UTC

ENTRYPOINT ["/unpackerr"]
