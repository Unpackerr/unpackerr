#!/bin/bash

# This file is used by deb and rpm packages.
# FPM adds this as the after-install script.

# Make a user and group for this app.
useradd --system --user-group --no-create-home --home-dir /tmp --shell /bin/false unpackerr

if [ -x "/bin/systemctl" ]; then
  # Reload and restart - this starts the application as user nobody.
  /bin/systemctl daemon-reload
  /bin/systemctl enable unpackerr
  /bin/systemctl restart unpackerr
fi
