#!/usr/bin/env bash
# Build FreeBSD pkgng .txz with fpm after `goreleaser --split`.
#
# nFPM has no freebsd target. The old Makefile used `fpm -s dir -t freebsd`.
# v0.15.2 GitHub assets are that output. This wrapper stages the same layout,
# calls fpm, and appends Archive entries to dist/freebsd/artifacts.json so
# merge uploads unpackerr-<version>.{amd64,i386,armhf,arm64}.txz.
set -euo pipefail

DIST=${1:?usage: freebsd_txz.sh dist/freebsd}
REPO=$(cd "$(dirname "$0")/../.." && pwd)
DIST=$(cd "${DIST}" && pwd)

need() { command -v "$1" >/dev/null || { echo "missing $1" >&2; exit 1; }; }
need fpm
need jq
need python3

# fpm's freebsd.rb invokes `tar --transform`. BSD tar (macOS) cannot.
if ! tar --version 2>/dev/null | grep -q 'GNU tar'; then
  if command -v gtar >/dev/null; then
    _gnutar_bin=$(mktemp -d)
    ln -s "$(command -v gtar)" "${_gnutar_bin}/tar"
    export PATH="${_gnutar_bin}:${PATH}"
  else
    echo "fpm -t freebsd needs GNU tar" >&2
    exit 1
  fi
fi

artifacts="${DIST}/artifacts.json"
metadata="${DIST}/metadata.json"
[[ -f ${artifacts} ]] || { echo "missing ${artifacts}" >&2; exit 1; }
[[ -f ${metadata} ]] || { echo "missing ${metadata}" >&2; exit 1; }

VERSION=$(jq -r '.version // empty' "${metadata}")
if [[ -z ${VERSION} || ${VERSION} == unknown || ${VERSION} == unstable ]]; then
  echo "refusing version '${VERSION}' from ${metadata}" >&2
  exit 1
fi

# Filename arch (install.sh / check.go) / fpm -a / pkg uname -p.
# fpm maps unknown -a to getconf LONG_BIT: v0.15.2 i386+armhf are FreeBSD:13:64
# because the Makefile passed -a 386 / -a arm. Pass names fpm knows, then
# rewrite the ABI CPU when fpm still emits the wrong one (arm64→aarch64).
arch_tuple() {
  case "$1" in
    amd64) echo "amd64 amd64 amd64" ;;
    386) echo "i386 i386 i386" ;;
    arm) echo "armhf amd64 armv7" ;;
    arm64) echo "arm64 aarch64 aarch64" ;;
    *) echo "unsupported freebsd goarch $1" >&2; return 1 ;;
  esac
}

rewrite_abi() {
  local pkg=$1 abi=$2 tmp list
  tmp=$(mktemp -d)
  list=$(mktemp)
  tar --transform 's|^/||' -xJf "${pkg}" -C "${tmp}"
  python3 - "${tmp}" "${abi}" <<'PY'
import json, sys
from pathlib import Path
root, arch = Path(sys.argv[1]), sys.argv[2]
for name in ("+COMPACT_MANIFEST", "+MANIFEST"):
    path = root / name
    data = json.loads(path.read_text())
    if data.get("arch") == arch:
        continue
    data["arch"] = arch
    path.write_text(json.dumps(data, separators=(",", ":")) + "\n")
PY
  {
    printf '%s\n' +COMPACT_MANIFEST +MANIFEST
    (cd "${tmp}" && find usr -type f | sort)
  } > "${list}"
  tar --owner=0 --group=0 --numeric-owner -Jcf "${pkg}" -C "${tmp}" \
    --files-from "${list}" --transform 's|^\([^+]\)|/\1|'
  rm -rf "${tmp}" "${list}"
}

stage() {
  local binary=$1 dest=$2
  mkdir -p \
    "${dest}/usr/local/bin" \
    "${dest}/usr/local/etc/unpackerr" \
    "${dest}/usr/local/etc/rc.d" \
    "${dest}/usr/local/share/man/man1" \
    "${dest}/usr/local/share/doc/unpackerr"
  install -m 755 "${binary}" "${dest}/usr/local/bin/unpackerr"
  install -m 755 "${REPO}/init/bsd/freebsd.rc.d" "${dest}/usr/local/etc/rc.d/unpackerr"
  install -m 644 "${REPO}/examples/unpackerr.conf.example" "${dest}/usr/local/etc/unpackerr/unpackerr.conf"
  install -m 644 "${REPO}/examples/unpackerr.conf.example" "${dest}/usr/local/etc/unpackerr/unpackerr.conf.example"
  [[ -f ${REPO}/unpackerr.1.gz ]] || { echo "missing ${REPO}/unpackerr.1.gz" >&2; exit 1; }
  install -m 644 "${REPO}/unpackerr.1.gz" "${dest}/usr/local/share/man/man1/unpackerr.1.gz"
  install -m 644 "${REPO}/LICENSE" "${dest}/usr/local/share/doc/unpackerr/LICENSE"
  # Same extras as the v0.15.2 fpm glob, minus examples.go.
  local src dst
  for src in \
    "${REPO}/examples/MANUAL.md" \
    "${REPO}/examples/MANUAL.html" \
    "${REPO}/examples/docker-compose.yml" \
    "${REPO}/examples/unpackerr.conf.example" \
    "${REPO}/README.html"
  do
    [[ -f ${src} ]] || continue
    dst=$(basename "${src}")
    [[ ${src} == */MANUAL.html ]] && dst=unpackerr_manual.html
    install -m 644 "${src}" "${dest}/usr/local/share/doc/unpackerr/${dst}"
  done
}

arts=()
names=()
seen=()
while IFS= read -r art; do
  goarch=$(jq -r '.goarch // empty' <<<"${art}")
  goarm=$(jq -r '.goarm // empty' <<<"${art}")
  [[ ${goarch} == arm && ${goarm} == 6 ]] && continue
  [[ " ${seen[*]} " == *" ${goarch} "* ]] && continue
  seen+=("${goarch}")

  path=$(jq -r '.path // empty' <<<"${art}")
  binary=${path}
  [[ -f ${binary} ]] || binary="${DIST}/${path}"
  [[ -f ${binary} ]] || { echo "freebsd binary missing: ${path}" >&2; exit 1; }

  read -r pkgarch fpm_a abi_cpu <<<"$(arch_tuple "${goarch}")"
  dest="${DIST}/unpackerr-${VERSION}.${pkgarch}.txz"
  tmp=$(mktemp -d)
  stage "${binary}" "${tmp}"
  rm -f "${dest}"
  fpm -s dir -t freebsd \
    --name unpackerr \
    -v "${VERSION}" \
    -a "${fpm_a}" \
    --license MIT \
    --url https://unpackerr.zip \
    --maintainer "David Newhall II <captain at golift dot io>" \
    --description "Extracts downloads so Radarr, Sonarr, Lidarr or Readarr may import them." \
    --freebsd-origin https://github.com/Unpackerr/unpackerr \
    --freebsd-osversion '*' \
    --before-install "${REPO}/init/systemd/before-install.sh" \
    --after-install "${REPO}/init/systemd/after-install.sh" \
    --before-remove "${REPO}/init/systemd/before-remove.sh" \
    -C "${tmp}" \
    -p "${dest}" \
    .
  rm -rf "${tmp}"
  rewrite_abi "${dest}" "FreeBSD:*:${abi_cpu}"
  echo "wrote ${dest##*/} from ${binary} ($(wc -c <"${dest}" | tr -d ' ') bytes)" >&2
  arts+=("${art}")
  names+=("${dest##*/}")
done < <(jq -c '.[] | select(.type=="Binary" and .goos=="freebsd")' "${artifacts}")

for need_arch in amd64 386 arm arm64; do
  [[ " ${seen[*]} " == *" ${need_arch} "* ]] || {
    echo "freebsd txz missing goarch ${need_arch}; built ${seen[*]}" >&2
    exit 1
  }
done

artifacts_tmp=$(mktemp)
cp "${artifacts}" "${artifacts_tmp}"
for i in "${!arts[@]}"; do
  jq --arg name "${names[$i]}" --argjson art "${arts[$i]}" \
    '. + [{
      name: $name,
      path: $name,
      goos: "freebsd",
      goarch: $art.goarch,
      goarm: $art.goarm,
      goamd64: $art.goamd64,
      type: "Archive",
      extra: (($art.extra // {}) + {ID: "freebsd-pkg", Format: "txz"})
    }]' "${artifacts_tmp}" > "${artifacts_tmp}.new"
  mv "${artifacts_tmp}.new" "${artifacts_tmp}"
done
mv "${artifacts_tmp}" "${artifacts}"
echo "appended ${#names[@]} txz archives to ${artifacts}" >&2
