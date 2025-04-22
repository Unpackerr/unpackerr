#!/bin/sh

# This file is used by deb, rpm and BSD packages.
# FPM adds this as the after-install script.

OS="$(uname -s)"

if [ "${OS}" = "Linux" ]; then
  # Make a user and group for this app, but only if it does not already exist.
  id unpackerr >/dev/null 2>&1  || \
    useradd --system --user-group --no-create-home --home-dir /tmp --shell /bin/false unpackerr
elif [ "${OS}" = "OpenBSD" ]; then
  id unpackerr >/dev/null 2>&1  || \
    useradd  -g =uid -d /tmp -s /bin/false unpackerr
elif [ "${OS}" = "FreeBSD" ]; then
  id unpackerr >/dev/null 2>&1  || \
    pw useradd unpackerr -d /tmp -w no -s /bin/false
else
  echo "Unknown OS: ${OS}, please add system user unpackerr manually."
fi

if [ -x "/bin/systemctl" ]; then
  # Reload and restart - this starts the application as user nobody.
  /bin/systemctl daemon-reload
  /bin/systemctl enable unpackerr
  /bin/systemctl restart unpackerr
fi
