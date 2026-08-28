#!/usr/bin/env bash
# GoReleaser copies nfpms.release into nFPM with no templates and no ${ENV}
# expansion (nFPM only expands env when loading its own YAML CLI config).
# The 1081 unstable debs shipped Version: 0.15.3~1081-${PKG_RELEASE} and
# Packagecloud refused to parse them.
#
# Tags: insert `release: REVISION` so Debian/RPM are 0.15.3-REVISION like v0.15.2.
# --nightly / local snapshots: no-op. .Version is already unique; setting
# release would double it (0.15.3~REVISION-REVISION).
set -euo pipefail

file=${1:-.goreleaser.yaml}
marker='nfpm-release: Linux split inserts `release: REVISION` on tags only.'
rel=${PKG_RELEASE-}

if [[ ! -f ${file} ]]; then
  echo "missing ${file}" >&2
  exit 1
fi
if ! grep -qF "${marker}" "${file}"; then
  echo "${file} has no nFPM release marker" >&2
  exit 1
fi

if [[ -z ${rel} ]]; then
  echo "nFPM release omitted (--nightly version already unique)"
  exit 0
fi

if [[ ! ${rel} =~ ^[0-9]+$ ]]; then
  echo "refusing nFPM release ${rel}" >&2
  exit 1
fi

python3 - "${file}" "${marker}" "${rel}" <<'PY'
import pathlib, sys
path = pathlib.Path(sys.argv[1])
marker, rel = sys.argv[2], sys.argv[3]
text = path.read_text()
if f"\n    release: {rel}\n" in text and marker in text:
    print(f"nFPM release={rel} (already set)")
    raise SystemExit(0)
needle = None
for line in text.splitlines(True):
    if marker in line:
        needle = line
        break
if needle is None:
    raise SystemExit("nFPM release marker line missing")
insert = needle + f"    release: {rel}\n"
if text.count(needle) != 1:
    raise SystemExit("nFPM release marker is not unique")
path.write_text(text.replace(needle, insert, 1))
print(f"nFPM release={rel}")
PY
