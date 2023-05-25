#!/bin/bash -x

# Deploys a new homebrew formula file to a github homebrew formula repo.
# Run by GitHub Actions when a new release is created on GitHub.

source settings.sh

NAME="unpackerr"
SOURCE_PATH=https://github.com/Unpackerr/unpackerr/archive/v${VERSION}.tar.gz
echo "==> Using URL: $SOURCE_PATH"
SHA256=$(curl -sL $SOURCE_PATH | openssl dgst -r -sha256 | awk '{print $1}')

push_it() {
  pushd release_repo
  git add .
  git commit -m "Update ${NAME} on Release: v${VERSION}-${ITERATION}"
  git push
  popd
  rm -rf release_repo
}

# Make an id_rsa file with our secret.
mkdir -p $HOME/.ssh
KEY_FILE="$(mktemp -u $HOME/.ssh/XXXXX)"
echo "${DEPLOY_KEY}" > "${KEY_FILE}"
chmod 600 "${KEY_FILE}"
# Configure ssh to use this secret on a custom github hostname.
GITHUB_HOST="github.$(basename $KEY_FILE)"
printf "%s\n" \
  "Host $GITHUB_HOST" \
  "  HostName github.com" \
  "  IdentityFile ${KEY_FILE}" \
  "  StrictHostKeyChecking no" \
  "  LogLevel ERROR" | tee -a $HOME/.ssh/config

git config --global user.email "${NAME}@auto.releaser"
git config --global user.name "${NAME}-auto-releaser"

rm -rf release_repo
git clone git@${GITHUB_HOST}:golift/homebrew-mugs.git release_repo
mkdir -p release_repo/Formula

# Creating formula from template using sed.
sed -e "s/{{Version}}/${VERSION}/g" \
  -e "s/{{Iter}}/${ITERATION}/g" \
  -e "s/{{SHA256}}/${SHA256}/g" \
  -e "s/{{Desc}}/${DESC}/g" \
  -e "s%{{SOURCE_PATH}}%${SOURCE_PATH}%g" \
  init/homebrew/service.rb.tmpl | tee release_repo/Formula/${NAME}.rb

push_it
