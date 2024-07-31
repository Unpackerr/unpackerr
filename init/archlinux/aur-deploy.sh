#!/bin/bash -x

# Deploys a new aur PKGBUILD file to an arch linux aur github repo.
# Run by GitHub Actions when a new release is created on GitHub.

source settings.sh

SOURCE_PATH=https://github.com/Unpackerr/unpackerr/archive/v${VERSION}.tar.gz
echo "==> Using URL: $SOURCE_PATH"
SHA=$(curl -sL $SOURCE_PATH | sha512sum | awk '{print $1}')

push_it() {
  pushd release_repo
  git add .
  git commit -m "Update unpackerr on Release: v${VERSION}-${ITERATION}"
  git push
  popd
  rm -rf release_repo
}

# Make an id_rsa file with our secret.
mkdir -p $HOME/.ssh
KEY_FILE="$(mktemp -u $HOME/.ssh/XXXXX)"
chmod 600 "${KEY_FILE}"
echo "${DEPLOY_KEY}" > "${KEY_FILE}"
# Configure ssh to use this secret on a custom hostname.
AUR_HOST="arch.$(basename $KEY_FILE)"
printf "%s\n" \
  "Host $AUR_HOST" \
  "  HostName aur.archlinux.org" \
  "  IdentityFile ${KEY_FILE}" \
  "  StrictHostKeyChecking no" \
  "  LogLevel ERROR" | tee -a $HOME/.ssh/config

git config --global user.email "unpackerr@auto.releaser"
git config --global user.name "unpackerr-auto-releaser"

rm -rf release_repo
git clone aur@${AUR_HOST}:unpackerr.git release_repo

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
