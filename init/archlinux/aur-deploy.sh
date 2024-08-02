#!/bin/bash -x

# Deploys a new aur PKGBUILD file to an arch linux aur github repo.
# Run by GitHub Actions when a new release is created on GitHub.

source settings.sh

SOURCE_PATH="https://github.com/Unpackerr/unpackerr/archive/v${VERSION}.tar.gz"
echo "==> Using URL: $SOURCE_PATH"
SHA=$(curl -sL "$SOURCE_PATH" | sha512sum | awk '{print $1}')

push_it() {
  git config user.email "unpackerr@github.releaser"
  git config user.name "unpackerr-github-releaser"
  pushd release_repo
  git add .
  git commit -m "Update unpackerr on Release: v${VERSION}-${ITERATION}"
  git push
  popd
  rm -rf release_repo
}

set -e

KEY_FILE=$(mktemp -u "$HOME"/.ssh/XXXXX)
echo "${DEPLOY_KEY}" > "${KEY_FILE}"
chmod 600 "${KEY_FILE}"
# Configure ssh to use this secret.
export GIT_SSH_COMMAND="ssh -i ${KEY_FILE} -o 'StrictHostKeyChecking no'"

rm -rf release_repo
git clone aur@aur.archlinux.org:unpackerr.git release_repo

sed -e "s/{{VERSION}}/${VERSION}/g" \
    -e "s/{{Iter}}/${ITERATION}/g" \
    -e "s/{{SHA}}/${SHA}/g" \
    -e "s/{{Desc}}/${DESC}/g" \
    -e "s%{{SOURCE_PATH}}%${SOURCE_PATH}%g" \
    init/archlinux/PKGBUILD.template | tee release_repo/PKGBUILD

sed -e "s/{{VERSION}}/${VERSION}/g" \
    -e "s/{{Iter}}/${ITERATION}/g" \
    -e "s/{{SHA}}/${SHA}/g" \
    -e "s/{{Desc}}/${DESC}/g" \
    -e "s%{{SOURCE_PATH}}%${SOURCE_PATH}%g" \
    init/archlinux/SRCINFO.template | tee release_repo/.SRCINFO

[ "$1" != "" ] || push_it
