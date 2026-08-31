#!/bin/sh

# This file is used by deb, rpm and BSD packages.
# FPM/nFPM adds this as the after-install script.
#
# Chown /etc/unpackerr only on a first install. Upgrades must not reset an
# admin who overrode User=/Group= and chowned the config to match.

OS="$(uname -s)"

logdir='/var/log/unpackerr'
[ "${OS}" = "Linux" ] || logdir='/usr/local/var/log/unpackerr'

# nFPM: deb $1=configure $2=oldver-or-empty; rpm $1=1 install / $1=2 upgrade.
first_install=0
if [ "${1:-}" = "1" ] || [ "${1:-}" = "install" ]; then
  first_install=1
elif [ "${1:-}" = "configure" ] && [ -z "${2:-}" ]; then
  first_install=1
fi

if [ "${first_install}" = 1 ]; then
  if [ -d /usr/local/etc/unpackerr ]; then
    chown -R unpackerr: /usr/local/etc/unpackerr
  fi
  if [ -d /etc/unpackerr ]; then
    chown -R unpackerr: /etc/unpackerr
  fi
fi

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
