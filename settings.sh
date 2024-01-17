
MAINT="David Newhall II <captain at golift dot io>"
DESC="Extracts downloads so Radarr, Sonarr, Lidarr or Readarr may import them."
LICENSE="MIT"
# Used for source links in package metadata and docker labels.
SOURCE_URL="https://github.com/Unpackerr/unpackerr"
VENDOR="Go Lift <code@golift.io>"
export MAINT DESC LICENSE SOURCE_URL VENDOR

DATE="$(date -u +%Y-%m-%dT%H:%M:00Z)"
VERSION=$(git describe --abbrev=0 --tags $(git rev-list --tags --max-count=1) | tr -d v)
[ "$VERSION" != "" ] || VERSION=development
# This produces a 0 in some environments (like Homebrew), but it's only used for packages.
ITERATION=$(git rev-list --count --all || echo 0)
COMMIT="$(git rev-parse --short HEAD || echo 0)"
GIT_BRANCH="$(git rev-parse --abbrev-ref HEAD || echo unknown)"
BRANCH="${GIT_BRANCH:-${GITHUB_REF_NAME}}"
export DATE VERSION ITERATION COMMIT BRANCH

### Optional ###

# Import this signing key only if it's in the keyring.
if which gpg; then
    gpg --list-keys 2>/dev/null | grep -q B93DD66EF98E54E2EAE025BA0166AD34ABC5A57C
    [ "$?" != "0" ] || export SIGNING_KEY=B93DD66EF98E54E2EAE025BA0166AD34ABC5A57C
fi

# Make sure Docker builds work locally.
# These do not affect automated builds, just allow the docker build scripts to run from a local clone.
[ -n "$SOURCE_BRANCH" ] || export SOURCE_BRANCH=$BRANCH
[ -n "$DOCKER_TAG" ] || export DOCKER_TAG=$(echo $SOURCE_BRANCH | sed 's/^v*\([0-9].*\)/\1/')
[ -n "$DOCKER_REPO" ] || export DOCKER_REPO="golift/unpackerr"
[ -n "$IMAGE_NAME" ] || export IMAGE_NAME="${DOCKER_REPO}:${DOCKER_TAG}"
[ -n "$DOCKERFILE_PATH" ] || export DOCKERFILE_PATH="init/docker/Dockerfile"
