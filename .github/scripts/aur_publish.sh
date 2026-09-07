#!/usr/bin/env bash
# Push the AUR source package after a tagged merge.
#
# GoReleaser --split fatals aur_sources (no source tarball yet: "no linux
# archives found"). continue --merge creates that tarball and runs AUR
# Publish, but Publish only uploads PKGBUILD artifacts that split never
# produced, so AUR is a silent no-op. This script is the publisher: same
# place in the merge job as packagecloud.
#
# Usage: aur_publish.sh [dist]
# Env: AUR_DEPLOY_KEY (required to push), VERSION, TAG, CHANNEL, DRY_RUN=1
set -euo pipefail

dir="${1:-dist}"
pkgname=unpackerr
pkgrel="${PKGREL:-1}"
pkgdesc='Extracts downloads so Radarr, Sonarr, Lidarr or Readarr may import them.'
url='https://unpackerr.zip'
repo="${GITHUB_REPOSITORY:-Unpackerr/unpackerr}"
aur_git='ssh://aur@aur.archlinux.org/unpackerr.git'

if [ "${CHANNEL:-}" = nightly ] || [ "${CHANNEL:-}" = unstable ]; then
  echo "skipping AUR for CHANNEL=${CHANNEL}"
  exit 0
fi

version="${VERSION:-}"
if [ -z "${version}" ] && [[ "${GITHUB_REF:-}" == refs/tags/v* ]]; then
  version="${GITHUB_REF#refs/tags/v}"
fi
if [ -z "${version}" ]; then
  shopt -s nullglob
  metas=("${dir}/metadata.json" "${dir}"/*/metadata.json)
  shopt -u nullglob
  for meta in "${metas[@]}"; do
    [ -f "${meta}" ] || continue
    version="$(jq -r '.version // empty' "${meta}" 2>/dev/null || true)"
    [ -n "${version}" ] && [ "${version}" != unknown ] && [ "${version}" != unstable ] && break
    version=
  done
fi
if [ -z "${version}" ] || [ "${version}" = unknown ] || [ "${version}" = unstable ]; then
  echo "refusing AUR publish with VERSION=${version:-<empty>}" >&2
  exit 1
fi
# Tagged .Version is the semver tag (0.16.0). Nightly already returned above.
if [[ ! ${version} =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "refusing AUR pkgver '${version}' (want x.y.z)" >&2
  exit 1
fi

tag="${TAG:-v${version}}"
tarball="${dir}/${pkgname}-${version}.tar.gz"
if [ -f "${tarball}" ]; then
  echo "using ${tarball}"
elif [ -n "${GITHUB_ACTIONS:-}" ]; then
  echo "missing ${tarball}; continue --merge should have created it at dist root" >&2
  ls -l "${dir}"/*.tar.gz 2>/dev/null || ls -l "${dir}" >&2 || true
  exit 1
else
  echo "missing ${tarball}; downloading ${tag} from GitHub" >&2
  mkdir -p "${dir}"
  curl -fsSL -o "${tarball}" \
    "https://github.com/${repo}/releases/download/${tag}/${pkgname}-${version}.tar.gz"
fi

sha256() {
  if command -v sha256sum >/dev/null; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}
sum="$(sha256 "${tarball}")"
source_url="https://github.com/${repo}/releases/download/${tag}/${pkgname}-${version}.tar.gz"
pkgver="${version//-/_}"

stage="$(mktemp -d "${TMPDIR:-/tmp}/unpackerr-aur.XXXXXX")"
cleanup() {
  rm -rf "${stage}"
  if [ -n "${keyfile:-}" ]; then
    rm -f "${keyfile}"
  fi
}
trap cleanup EXIT

cp -f init/systemd/unpackerr.install "${stage}/unpackerr.install"

# PKGBUILD functions keep ${pkgname}/${pkgver}/${pkgdir} for makepkg.
cat > "${stage}/PKGBUILD" <<EOF
# Maintainer: David Newhall II <captain at golift dot io>
# Maintainer: Donald Webster <fryfrog at gmail dot com>

pkgname='${pkgname}'
pkgver=${pkgver}
pkgrel=${pkgrel}
pkgdesc='${pkgdesc}'
url='${url}'
arch=('x86_64' 'aarch64' 'arm' 'armv6h' 'armv7h' 'i686' 'pentium4')
license=('MIT')
provides=('unpackerr')
makedepends=('go' 'gzip')
optdepends=(
  'transmission-cli: torrent downloader (CLI and daemon)'
  'transmission-gtk: torrent downloader (GTK+)'
  'transmission-qt: torrent downloader (Qt)'
  'deluge: torrent downloader'
  'rtorrent: torrent downloader'
)
backup=('etc/unpackerr/unpackerr.conf')
install=unpackerr.install
source=("${pkgname}-${pkgver}.tar.gz::${source_url}")
sha256sums=('${sum}')

prepare() {
  cd "\${pkgname}-\${pkgver}"
  mkdir -p build
}

build() {
  cd "\${pkgname}-\${pkgver}"
  export GOFLAGS="-buildmode=pie -trimpath -modcacherw"
  LDFLAGS="-w -s -X golift.io/version.Version=\${pkgver} \\
    -X golift.io/version.Revision=\${pkgrel} \\
    -X golift.io/version.BuildDate=\$(date -u +%Y-%m-%dT%H:%M:00Z) \\
    -X golift.io/version.BuildUser=\$(whoami || echo unknown) \\
    -X \\"golift.io/version.Branch=\${pkgver} [aur]\\""
  go build -o unpackerr -ldflags "\${LDFLAGS}" .
  go run github.com/davidnewhall/md2roff@v0.0.1 --manual unpackerr --version "\${pkgver}" --date "\$(date -u +%Y-%m-%d)" examples/MANUAL.md
  go run github.com/davidnewhall/md2roff@v0.0.1 --manual unpackerr --version "\${pkgver}" --date "\$(date -u +%Y-%m-%d)" README.md
  gzip -9nf examples/MANUAL
  mv examples/MANUAL.gz unpackerr.1.gz
}

package() {
  cd "\${pkgname}-\${pkgver}"
  install -d -m 755 "\${pkgdir}/usr/share/licenses/\${pkgname}" "\${pkgdir}/usr/share/doc/\${pkgname}" "\${pkgdir}/etc/\${pkgname}"
  install -D -m 755 unpackerr "\${pkgdir}/usr/bin/unpackerr"
  install -D -m 644 examples/unpackerr.conf.example "\${pkgdir}/etc/unpackerr/unpackerr.conf"
  install -D -m 644 examples/unpackerr.conf.example "\${pkgdir}/etc/unpackerr/unpackerr.conf.example"
  install -D -m 644 LICENSE "\${pkgdir}/usr/share/licenses/unpackerr/LICENSE"
  install -D -m 644 examples/MANUAL.html "\${pkgdir}/usr/share/doc/\${pkgname}/unpackerr-manual.html"
  install -D -m 644 README.html "\${pkgdir}/usr/share/doc/\${pkgname}/README.html"
  install -D -m 644 examples/docker-compose.yml "\${pkgdir}/usr/share/doc/\${pkgname}/docker-compose.yml"
  install -D -m 644 examples/unpackerr.conf.example "\${pkgdir}/usr/share/doc/\${pkgname}/unpackerr.conf.example"
  install -D -m 644 unpackerr.1.gz "\${pkgdir}/usr/share/man/man1/unpackerr.1.gz"
  install -D -m 644 init/systemd/unpackerr.service "\${pkgdir}/usr/lib/systemd/system/unpackerr.service"
  echo 'u unpackerr - "unpackerr daemon"' > unpackerr.sysusers
  install -D -m 644 unpackerr.sysusers "\${pkgdir}/usr/lib/sysusers.d/unpackerr.conf"
  install -D -m 644 init/systemd/unpackerr.tmpfiles "\${pkgdir}/usr/lib/tmpfiles.d/unpackerr.conf"
}
EOF

{
  echo "pkgbase = ${pkgname}"
  echo "	pkgdesc = ${pkgdesc}"
  echo "	pkgver = ${pkgver}"
  echo "	pkgrel = ${pkgrel}"
  echo "	url = ${url}"
  echo "	arch = x86_64"
  echo "	arch = aarch64"
  echo "	arch = arm"
  echo "	arch = armv6h"
  echo "	arch = armv7h"
  echo "	arch = i686"
  echo "	arch = pentium4"
  echo "	license = MIT"
  echo "	optdepends = transmission-cli: torrent downloader (CLI and daemon)"
  echo "	optdepends = transmission-gtk: torrent downloader (GTK+)"
  echo "	optdepends = transmission-qt: torrent downloader (Qt)"
  echo "	optdepends = deluge: torrent downloader"
  echo "	optdepends = rtorrent: torrent downloader"
  echo "	makedepends = go"
  echo "	makedepends = gzip"
  echo "	provides = unpackerr"
  echo "	backup = etc/unpackerr/unpackerr.conf"
  echo "	install = unpackerr.install"
  echo "	source = ${source_url}"
  echo "	sha256sums = ${sum}"
  echo
  echo "pkgname = ${pkgname}"
} > "${stage}/.SRCINFO"

echo "AUR ${pkgname} ${pkgver}-${pkgrel} sha256=${sum}"

if [ "${DRY_RUN:-}" = 1 ]; then
  mkdir -p "${dir}/aur"
  cp -f "${stage}/PKGBUILD" "${stage}/.SRCINFO" "${stage}/unpackerr.install" "${dir}/aur/"
  echo "DRY_RUN=1 wrote ${dir}/aur/PKGBUILD"
  exit 0
fi

if [ -z "${AUR_DEPLOY_KEY:-}" ]; then
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    echo "AUR_DEPLOY_KEY unset; refusing to skip AUR push in CI" >&2
    exit 1
  fi
  echo "AUR_DEPLOY_KEY unset; wrote files without pushing:" >&2
  mkdir -p "${dir}/aur"
  cp -f "${stage}/PKGBUILD" "${stage}/.SRCINFO" "${stage}/unpackerr.install" "${dir}/aur/"
  ls -l "${dir}/aur"
  exit 0
fi

keyfile="$(mktemp "${TMPDIR:-/tmp}/unpackerr-aur-key.XXXXXX")"
printf '%s\n' "${AUR_DEPLOY_KEY}" | tr -d '\r' > "${keyfile}"
chmod 600 "${keyfile}"

export GIT_SSH_COMMAND="ssh -i ${keyfile} -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -F /dev/null"

clone="${stage}/repo"
git clone --depth 1 "${aur_git}" "${clone}"
cp -f "${stage}/PKGBUILD" "${stage}/.SRCINFO" "${stage}/unpackerr.install" "${clone}/"
git -C "${clone}" config user.name goreleaserbot
git -C "${clone}" config user.email bot@goreleaser.com
git -C "${clone}" add PKGBUILD .SRCINFO unpackerr.install
if git -C "${clone}" diff --cached --quiet; then
  echo "AUR already at ${pkgver}-${pkgrel}"
  exit 0
fi
git -C "${clone}" commit -m "Update unpackerr to ${tag}"
git -C "${clone}" push origin HEAD:master
echo "pushed AUR unpackerr ${pkgver}-${pkgrel}"
