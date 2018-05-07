# Unpacker Poller

# About

This application runs as a daemon on your Deluge host. It checks for completed
downloads and extracts them so Radarr and/or Sonarr may import them.

There are a handful of options out there for extracting and deleting files after
Deluge downloads them. I just didn't care for any of them, so I wrote my own. I
wanted a small single-binary with reasonable logging that can extract downloaded
archives and clean up the mess after they've been imported. Why a separate binary
instead of a Deluge plugin? Because I like Go more than Python and I wanted a fun
project to work on over a weekend.

## Logic

The application kicks up a go routine for Deluge and another for each of Radarr
and Sonarr (if you include configs for them). These go routines just poll their
respective applications for transfers/queued items. The items are stored. The
interval of these pollers is set in the config file. 2-10 minutes is good.

Another go routine checks (the internal data) for completed downloads. When it
finds an item in Deluge that matches an item in Sonarr or Radarr the download
location is checked for a `.rar` file. If an extractable archive exists, and
**Sonarr/Radarr have `status=Completed` from Deluge** this application will
extract the file. When the item falls out of the (Radarr/Sonarr) queue, the
extracted files are removed.

Tags are currently mentioned, but nothing uses them. I figured I would match tags
before I started getting data from the APIs. Once I realized I was able to match
`d.Name` with `q.Title` I didn't need to use tags. It all works out automagically.

## Installation

- Setup a working Go build environment.
- `make install` (mac) or `sudo make install` (linux)
- cross your fingers.

If you don't want to use the Makefile, manual installation is pretty easy.
- Build the app like any other Go app (or run `make`).
- Copy the binary to `/usr/local/bin`
- Make a config folder: `sudo mkdir /usr/local/etc/unpacker-poller`
- Copy the example config: `sudo cp up.conf.example /usr/local/etc/unpacker-poller/up.conf`
- On macOS, copy the launchd file: `cp startup/launchd/* ~/Library/LaunchAgents`
- On Linux, copy the systemd unit: `sudo cp startup/systemd/* /etc/systemd/system`

After the app is installed, update your deluge, sonarr and radarr configuration
in `/usr/local/etc/unpacker-poller/up.conf`. The app works without Sonarr or Radarr
configs, but you should have at least one to make it useful.

- Start the service, Linux: `sudo systemctl daemon-reload ; sudo systemctl start unapcker-poller`
- Start the service, macOS: `sudo launchctl load ~/Library/LaunchAgents/com.github.davidnewhall.unpacker-poller.plist`

## Troubleshooting

The `http_` config options are for basic http auth. Most users will need to
leave these blank. I was using them to test my connection through an authenticated
nginx proxy. I did not test with basic auth enabled in Sonarr/Radarr. They may
or may not work for that. If you need different features, open an Issue and let me
know. Generally, you'll point all endpoints at localhost, without nginx and without
basic auth.

- Log file is (hopefully) at `/usr/local/var/log/unpacker-poller.log`
- On macOS, Deluge log is at `~/.config/deluge/deluged.log`
- I haven't tested any of this on Linux, feedback welcomed.

If transfers are in a Warning or Error state they will not be extracted. Try
the Force Recheck option in Deluge.

## Notes

While writing this, I kept finding Deluge unresponsive. After finding and inspecting
the Deluge log file, I found that the app was running out of open files. Turns out
this was causing a lot of issues on my server. I have a Mac. Check this out if you're
in the same boat: http://blog.mact.me/2014/10/22/yosemite-upgrade-changes-open-file-limit

## License

[MIT](MIT-LICENSE) - Copyright (c) 2018 David Newhall II
