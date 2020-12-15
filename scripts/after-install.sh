#!/bin/bash

# This file is used by deb and rpm packages.
# FPM adds this as the after-install script.
# Edit this file as needed for your application.
# This file is only installed if FORMULA is set to service.

# Make a user and group for this app.
useradd --system --user-group --no-create-home --home-dir /tmp --shell /bin/false {{BINARY}}

if [ -x "/bin/systemctl" ]; then
  # Reload and restart - this starts the application as user nobody.
  /bin/systemctl daemon-reload
  /bin/systemctl enable {{BINARY}}
  /bin/systemctl restart {{BINARY}}
fi
