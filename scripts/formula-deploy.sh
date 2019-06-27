#!/bin/bash -x

# Deploys a new unpacker-poller.rb formula file to golift/homebrew-tap.
# Requires SSH credentials in ssh-agent to work.
# Run by Travis-CI when a new release is created on GitHub.

make unpacker-poller.rb
VERSION=$(grep -E 'archive/v.*tar.gz\s*"' unpacker-poller.rb | grep -Eo 'v([0-9]+\.[0-9]+\.[0-9]*)' | tr -d v)

rm -rf homebrew-mugs
git config --global user.email "unpacker@auto.releaser"
git config --global user.name "unpacker-auto-releaser"
git clone git@github.com:golift/homebrew-mugs.git

cp unpacker-poller.rb homebrew-mugs/Formula
pushd homebrew-mugs
git commit -m "Update unpacker-poller on Release: v${VERSION}" Formula/unpacker-poller.rb
git push
popd
