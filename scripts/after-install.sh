#!/bin/bash

# This file is used by deb and rpm packages.
# FPM adds this as the after-install script.

if [ -x "/bin/systemctl" ]; then
  # Reload and restart - this starts the application as user nobody.
  /bin/systemctl daemon-reload
  /bin/systemctl enable deluge-unpacker
  /bin/systemctl restart deluge-unpacker
fi
