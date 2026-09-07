#!/usr/bin/env bash
# Fail closed when a publishing secret is empty. GitHub maps missing secrets
# to "" so `if: secrets.FOO != ''` is not a substitute.
set -euo pipefail

CHANNEL="${CHANNEL:-}"
if [ -z "${CHANNEL}" ]; then
  echo "CHANNEL is empty" >&2
  exit 1
fi

missing=0
need() {
  local name=$1
  local val=${!name-}
  if [ -z "${val}" ]; then
    echo "missing secret: ${name}" >&2
    missing=1
  fi
}

need GORELEASER_PRO_KEY
need GPG_SIGNING_KEY
# Hub is part of every channel (GHCR + golift/unpackerr). Absence used to
# skip login and still go green; that is no longer allowed.
need DOCKERHUB_PASSWORD
need CODESIGN_URL
need CODESIGN_CLIENT_CERT
need CODESIGN_CLIENT_KEY

# Nightly still builds linux/windows/freebsd and publishes Docker.
# It skips Darwin, GitHub Releases, AUR, and packagecloud.
if [ "${CHANNEL}" != nightly ]; then
  need MACOS_SIGN_P12
  need MACOS_SIGN_PASSWORD
  need MACOS_NOTARY_KEY
  need MACOS_NOTARY_KEY_ID
  need MACOS_NOTARY_ISSUER_ID
  need PACKAGECLOUD_TOKEN
fi

if [ "${CHANNEL}" = release ]; then
  need AUR_DEPLOY_KEY
fi

if [ "${CHANNEL}" = unstable ]; then
  need UNSTABLE_UPLOAD_KEY
fi

if [ "${missing}" -ne 0 ]; then
  echo "refusing to publish CHANNEL=${CHANNEL} with empty secrets" >&2
  exit 1
fi

echo "CHANNEL=${CHANNEL}: required secrets are present"
