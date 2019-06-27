#!/bin/bash -x

# Deploys a new unpacker-poller.rb formula file to golift/homebrew-tap.
# Requires SSH credentials in ssh-agent to work.
# Run by Travis-CI when a new release is created on GitHub.

if [ -z "$VERSION" ]; then
  VERSION=$TRAVIS_TAG
fi
if [ -z "$VERSION" ]; then
  VERSION=$(grep -E 'archive/v.*tar.gz\s*"' unpacker-poller.rb | grep -Eo 'v([0-9]+\.[0-9]+\.[0-9]*)')
fi

rm -rf homebrew-mugs
git config --global user.email "unpacker@auto.releaser"
git config --global user.name "unpacker-auto-releaser"
git clone git@github.com:golift/homebrew-mugs.git

cp unpacker-poller.rb homebrew-mugs/Formula
pushd homebrew-mugs
git commit -m "Update unpacker-poller on Release: ${VERSION}" Formula/unpacker-poller.rb
git push
popd
