#!/bin/sh

# This file is used by deb, rpm and BSD packages.
# FPM adds this as the after-install script.

OS="$(uname -s)"

logdir='/var/log/unpackerr'
[[ "$(uname -s)" = "Linux" ]] || logdir='/usr/local/var/log/unpackerr'

if [ ! -d "${logdir}" ]; then
  mkdir "${logdir}"
  chown unpackerr: "${logdir}"
  chmod 0755 "${logdir}"
fi

if [ -x "/bin/systemctl" ]; then
  # Reload and restart - this starts the application as user nobody.
  /bin/systemctl daemon-reload
  /bin/systemctl enable unpackerr
  /bin/systemctl restart unpackerr
fi
