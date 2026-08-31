#!/bin/sh

# This file is used by deb, rpm and BSD packages.
# FPM/nFPM adds this as the after-install script.
#
# Chown the packaged config files only on a first install. Upgrades must not
# reset an admin who overrode User=/Group= and chowned the config to match.
# Name the packaged files instead of chown -R so extra files in the directory
# (password lists, drop-in confs) keep the ownership the admin set.

OS="$(uname -s)"

if [ "${OS}" = "Linux" ]; then
  confdir=/etc/unpackerr
  logdir=/var/log/unpackerr
else
  confdir=/usr/local/etc/unpackerr
  logdir=/usr/local/var/log/unpackerr
fi

# nFPM: deb $1=configure $2=oldver-or-empty; rpm $1=1 install / $1=2 upgrade.
first_install=0
if [ "${1:-}" = "1" ] || [ "${1:-}" = "install" ]; then
  first_install=1
elif [ "${1:-}" = "configure" ] && [ -z "${2:-}" ]; then
  first_install=1
fi

if [ "${first_install}" = 1 ] && [ -d "${confdir}" ]; then
  chown unpackerr: "${confdir}"
  [ -f "${confdir}/unpackerr.conf" ] && chown unpackerr: "${confdir}/unpackerr.conf"
  [ -f "${confdir}/unpackerr.conf.example" ] && chown unpackerr: "${confdir}/unpackerr.conf.example"
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
