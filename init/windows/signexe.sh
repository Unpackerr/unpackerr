#!/usr/bin/env bash

set -e -o pipefail

# Authenticode-sign a Windows PE via golift/codesign (YubiKey-backed signerd).
# GitHub Actions uses golift/codesign@v1 after `make release WINDOWS_ZIP=0`.
# This script is for local `make windows` when CODESIGN_URL is set (SSH tunnel
# or a configured CLI).
#
# On macOS, never call /usr/bin/codesign (Apple's tool). Prefer CODESIGN_BIN,
# "$(go env GOPATH)/bin/codesign", then any other `codesign` on PATH.

function pick_codesign() {
  if [ -n "${CODESIGN_BIN:-}" ]; then
    echo "${CODESIGN_BIN}"
    return
  fi
  gopath="$(go env GOPATH 2>/dev/null || true)"
  if [ -n "${gopath}" ] && [ -x "${gopath}/bin/codesign" ]; then
    echo "${gopath}/bin/codesign"
    return
  fi
  # Apple ships /usr/bin/codesign. Skip it; any other PATH hit is the CLI.
  while IFS= read -r p; do
    case "$p" in
      /usr/bin/codesign|/bin/codesign) continue ;;
    esac
    echo "$p"
    return
  done < <(type -a -p codesign 2>/dev/null || true)
  return 1
}

function sign() {
  if [ -z "${CODESIGN_URL:-}" ]; then
    echo "Skipped signing ${FILE} (CODESIGN_URL unset) .." >&2
    exit 0
  fi

  bin="$(pick_codesign)" || {
    if [ -n "${GITHUB_ACTIONS:-}" ]; then
      echo "Skipped signing ${FILE} (codesign CLI not on PATH; Action signs later) .." >&2
      exit 0
    fi
    echo "CODESIGN_URL is set but golift codesign CLI not found (set CODESIGN_BIN)" >&2
    exit 1
  }

  CODESIGN_NAME="${CODESIGN_NAME:-Unpackerr}" \
  CODESIGN_WEBSITE="${CODESIGN_WEBSITE:-https://unpackerr.zip}" \
  "${bin}" -- "${FILE}"
  echo "Signed ${FILE} .." >&2
}

[ -z "$1" ] || FILE="$1" sign
