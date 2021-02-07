# Application Builder Configuration File. Customized for: unpackerr
# Each line must have an export clause.
# This file is parsed and sourced by the Makefile, Docker and Homebrew builds.
# Powered by Application Builder: https://golift.io/application-builder

# Bring in dynamic repo/pull/source info.
source $(dirname "${BASH_SOURCE[0]}")/init/buildinfo.sh

# Must match the repo name to make things easy. Otherwise, fix some other paths.
BINARY="unpackerr"
REPO="davidnewhall/unpackerr"
# Github repo containing homebrew formula repo.
HBREPO="golift/homebrew-mugs"
MAINT="David Newhall II <david at sleepers dot pro>"
VENDOR="Go Lift"
DESC="Extracts downloads so Radarr, Sonarr, Lidarr or Readarr may import them."
GOLANGCI_LINT_ARGS="--enable-all -D dupl -D exhaustivestruct"
# Example must exist at examples/$CONFIG_FILE.example
CONFIG_FILE="unpackerr.conf"
LICENSE="MIT"
# FORMULA is either 'service' or 'tool'. Services run as a daemon, tools do not.
# This affects the homebrew formula (launchd) and linux packages (systemd).
FORMULA="service"

# Used for source links and wiki links.
SOURCE_URL="https://github.com/${GHUSER}/${REPO}/"

# This parameter is passed in as -X to go build. Used to override the Version variable in a package.
# Name the Version-containing library the same as the github repo, without dashes.
# The Makefile appends .Version and a few other variables for build-time data. See golift.io/version.
VERSION_PATH="golift.io/version"

# Used by homebrew downloads, references VERSION which comes from buildinfo.sh.
SOURCE_PATH=https://golift.io/${REPO}/archive/v${VERSION}.tar.gz

export BINARY HBREPO MAINT VENDOR DESC GOLANGCI_LINT_ARGS CONFIG_FILE
export LICENSE FORMULA SOURCE_URL VERSION_PATH SOURCE_PATH

# Optional
#export WINDOWS_LDFLAGS="-H windowsgui"
#export MACAPP="Unpackerr"
