#!/bin/sh

# This file is used by deb, rpm and BSD packages.
# FPM adds this as the after-install script.

OS="$(uname -s)"

logdir='/var/log/unpackerr'
[ "${OS}" = "Linux" ] || logdir='/usr/local/var/log/unpackerr'

if [ -d /usr/local/etc/unpackerr ]; then
  chown -R unpackerr: /usr/local/etc/unpackerr
fi

if [ -d /etc/unpackerr ]; then
  chown -R unpackerr: /etc/unpackerr
fi

if [ ! -d "${logdir}" ]; then
  mkdir "${logdir}"
fi
chown unpackerr: "${logdir}"
chmod 0755 "${logdir}"

if [ -x "/bin/systemctl" ]; then
  # Reload and restart - this starts the application as user nobody.
  /bin/systemctl daemon-reload
  /bin/systemctl enable unpackerr
  /bin/systemctl restart unpackerr
fi
