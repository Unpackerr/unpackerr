# This file is read in by metadata.sh.
# These values are not generally user configurable and this file is overwritten on upgrades.
# Override values in here by setting them in .metadata.sh; do not change this file.
##########

# Dynamic. Recommend not changing.
VVERSION=$(git describe --abbrev=0 --tags $(git rev-list --tags --max-count=1))
VERSION="$(echo $VVERSION | tr -d v | grep -E '^\S+$' || echo development)"
# This produces a 0 in some envirnoments (like Homebrew), but it's only used for packages.
ITERATION=$(git rev-list --count --all || echo 0)
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
COMMIT="$(git rev-parse --short HEAD || echo 0)"

GIT_BRANCH="$(git rev-parse --abbrev-ref HEAD || echo unknown)"
BRANCH="${TRAVIS_BRANCH:-${GIT_BRANCH}}"

# Defines docker manifest/build types.
BUILDS="linux:armhf:arm linux:arm64:arm64 linux:amd64:amd64 linux:i386:386"

export VVERSION VERSION ITERATION DATE BRANCH COMMIT BUILDS
