#!/usr/bin/env bash

# This is invoked by the Makefile to create a simple docker image ready to go.

source settings.sh

docker buildx build --load --pull --tag unpackerr \
    --platform linux/amd64 \
    --build-arg "BUILD_DATE=${DATE}" \
    --build-arg "COMMIT=${COMMIT}" \
    --build-arg "BRANCH=${BRANCH}" \
    --build-arg "VERSION=${VERSION}" \
    --build-arg "ITERATION=${ITERATION}" \
    --build-arg "LICENSE=${LICENSE}" \
    --build-arg "DESC=${DESC}" \
    --build-arg "VENDOR=${VENDOR}" \
    --build-arg "AUTHOR=${MAINT}" \
    --build-arg "SOURCE_URL=${SOURCE_URL}" \
    --file init/docker/Dockerfile .