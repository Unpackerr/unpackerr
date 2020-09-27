# Each line must have an export clause.
# This file is parsed and sourced by the Makefile, Docker and Homebrew builds.
# Powered by Application Builder: https://github.com/golift/application-builder

# Must match the repo name to make things easy. Otherwise, fix some other paths.
BINARY="unpackerr"
# github username
GHUSER="davidnewhall"
# Github repo containing homebrew formula repo.
HBREPO="golift/homebrew-mugs"
MAINT="David Newhall II <david at sleepers dot pro>"
VENDOR="Go Lift"
DESC="Extracts downloads so Radarr or Sonarr may import them."
GOLANGCI_LINT_ARGS="--enable-all -D gci -D goerr113"
# Example must exist at examples/$CONFIG_FILE.example
CONFIG_FILE="unpackerr.conf"
LICENSE="MIT"
# FORMULA is either 'service' or 'tool'. Services run as a daemon, tools do not.
# This affects the homebrew formula (launchd) and linux packages (systemd).
FORMULA="service"

export BINARY GHUSER HBREPO MAINT VENDOR DESC GOLANGCI_LINT_ARGS CONFIG_FILE LICENSE FORMULA

# The rest is mostly automatic.
# Fix the repo if it doesn't match the binary name.
# Provide a better URL if one exists.

# Used for source links and wiki links.
SOURCE_URL="https://${GHUSER}/${BINARY}/"
# Used for documentation links.
URL="${SOURCE_URL}"

# Dynamic. Recommend not changing.
VVERSION=$(git describe --abbrev=0 --tags $(git rev-list --tags --max-count=1))
VERSION="$(echo $VVERSION | tr -d v | grep -E '^\S+$' || echo development)"
# This produces a 0 in some envirnoments (like Homebrew), but it's only used for packages.
ITERATION=$(git rev-list --count --all || echo 0)
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
COMMIT="$(git rev-parse --short HEAD || echo 0)"

GIT_BRANCH="$(git rev-parse --abbrev-ref HEAD || echo unknown)"
BRANCH="${TRAVIS_BRANCH:-${GIT_BRANCH}}"

# Used by homebrew downloads.
#SOURCE_PATH=https://codeload.${IMPORT_PATH}/tar.gz/v${VERSION}
SOURCE_PATH=https://golift.io/${BINARY}/archive/v${VERSION}.tar.gz

export SOURCE_URL URL VERSION_PATH VVERSION VERSION ITERATION DATE BRANCH COMMIT SOURCE_PATH
