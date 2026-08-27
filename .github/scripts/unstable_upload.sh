#!/usr/bin/env bash
# Upload zip/dmg/gz artifacts to unstable.golift.io (same as the old Actions loop).
set -euo pipefail

dir="${1:-dist}"
if [ -z "${UNSTABLE_UPLOAD_KEY:-}" ]; then
  echo "UNSTABLE_UPLOAD_KEY unset; skipping unstable.golift.io upload" >&2
  exit 0
fi

shopt -s nullglob
files=("${dir}"/*.zip "${dir}"/*.dmg "${dir}"/*.gz)
if [ ${#files[@]} -eq 0 ]; then
  echo "no zip/dmg/gz artifacts in ${dir}" >&2
  exit 0
fi

version="${VERSION:-unknown}"
for file in "${files[@]}"; do
  [ -f "$file" ] || continue
  name="$(basename "$file")"
  echo "Uploading ${name}"
  curl -sS --fail-with-body --retry 5 --retry-all-errors --retry-delay 2 \
    -H "X-API-KEY: ${UNSTABLE_UPLOAD_KEY}" \
    "https://unstable.golift.io/upload.php?folder=unpackerr" -F "file=@${file}"
  curl -sS --fail-with-body --retry 5 --retry-all-errors --retry-delay 2 \
    -H "X-API-KEY: ${UNSTABLE_UPLOAD_KEY}" \
    "https://unstable.golift.io/upload.php?folder=unpackerr" \
    -F "file=${version};filename=${name}.txt;type=text/plain"
done
