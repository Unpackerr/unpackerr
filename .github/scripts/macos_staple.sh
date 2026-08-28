#!/usr/bin/env bash
# Confirm Developer ID on Unpackerr.app, then staple the notarized DMG.
# codesign -dv writes to stderr; grep -q in a pipefail pipeline is a false fail
# (tee gets SIGPIPE after the first match). Dump to a file, then grep.
set -euo pipefail

root="${1:-dist/darwin}"
dump="${RUNNER_TEMP:-/tmp}/codesign-app.txt"

app="${root}/apps/unpackerr-app_darwinall/Unpackerr.app"
if [ ! -d "${app}" ]; then
  app=""
  while IFS= read -r p; do
    case "${p}" in
      */dmg/*) continue ;;
    esac
    app="${p}"
    break
  done < <(find "${root}" -type d -name 'Unpackerr.app')
fi
if [ ! -d "${app}" ]; then
  while IFS= read -r p; do
    app="${p}"
    break
  done < <(find "${root}" -type d -name 'Unpackerr.app')
fi
if [ ! -d "${app}" ]; then
  echo "Unpackerr.app missing under ${root}" >&2
  find "${root}" -type f >&2 || true
  exit 1
fi

codesign --verify --deep --strict "${app}"
codesign -dv --verbose=2 "${app}" >"${dump}" 2>&1
if ! grep -F "Developer ID Application" "${dump}" >/dev/null; then
  echo "Unpackerr.app is not Developer ID signed; macos_native skipped or failed" >&2
  cat "${dump}" >&2
  exit 1
fi
echo "signed ${app}"
cat "${dump}"

found=0
while IFS= read -r dmg; do
  [ -n "${dmg}" ] || continue
  found=1
  echo "stapling ${dmg}"
  ok=0
  for attempt in 1 2 3 4 5; do
    if xcrun stapler staple "${dmg}"; then
      ok=1
      break
    fi
    if [ "${attempt}" -eq 5 ]; then
      break
    fi
    echo "stapler attempt ${attempt} failed; waiting for Apple ticket"
    sleep 20
  done
  if [ "${ok}" -ne 1 ]; then
    exit 1
  fi
done < <(find "${root}" -type f -name '*.dmg' | sort)

if [ "${found}" -eq 0 ]; then
  echo "no DMG under ${root}" >&2
  find "${root}" -type f >&2 || true
  exit 1
fi
