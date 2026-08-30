#!/usr/bin/env bash
# Developer ID codesign for the raw Darwin binaries, run by GoReleaser as a
# post-build hook (one call per arch, before universal_binaries lipos them).
#
# Why: the Homebrew cask installs the darwin tar.gz, not the DMG (the cask
# pipe cannot consume DMG artifacts). Without this the cask ships an
# unsigned binary. Each lipo'd slice keeps its signature, so the universal
# binary in the tar.gz is signed. Notarization is only required for
# quarantined files (browser/Mail downloads); brew's downloader does not set
# com.apple.quarantine, so a signed-and-hardened binary installs cleanly.
# Quarantine-prone installs should keep using the notarized Unpackerr.dmg.
#
# MACOS_SIGN_IDENTITY / KEYCHAIN_PATH come from .github/scripts/macos_keychain.sh
# in the split-darwin job. Locally (or on a host without a signing keychain)
# this is a no-op so `goreleaser release --split` still works.
set -euo pipefail

bin="${1:?usage: signbin.sh <path-to-binary>}"
identity="${MACOS_SIGN_IDENTITY:-}"

if [ -z "${identity}" ]; then
  echo "MACOS_SIGN_IDENTITY unset; skipping codesign of ${bin}"
  exit 0
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

codesign --force --verbose \
  --timestamp \
  --options runtime \
  --entitlements "${script_dir}/entitlements.plist" \
  --sign "${identity}" \
  "${bin}"

codesign --verify --verbose=2 "${bin}"
