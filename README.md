# Deluge Unpacker

-   formerly `unpacker-poller`

## About

This application runs as a daemon on your Deluge host. It checks for completed
downloads and extracts them so Radarr and/or Sonarr may import them.

There are a handful of options out there for extracting and deleting files after
Deluge downloads them. I just didn't care for any of them, so I wrote my own. I
wanted a small single-binary with reasonable logging that can extract downloaded
archives and clean up the mess after they've been imported. Why a separate binary
instead of a Deluge plugin? Because I like Go more than Python and I wanted a fun
project to work on over a weekend. At this point though, I'm weeks and weeks in.

## Logic

The application kicks up a go routine for Deluge and another for each of Radarr
and Sonarr (if you include configs for them). These go routines just poll their
respective applications for transfers/queued items. The items are stored. The
interval of these pollers is set in the config file. 2-10 minutes is good.

Another go routine checks (the internal data) for completed downloads. When it
finds an item in Deluge that matches an item in Sonarr or Radarr the download
location is checked for a `.rar` file. If an extractable archive exists, and
**Sonarr/Radarr have `status=Completed` from Deluge** this application will
extract the file. Files are extracted to a temporary folder, and then moved back
into the download location for Completed Download Handling to import them. When
the item falls out of the (Radarr/Sonarr) queue, the extracted files are removed.

Tags are currently mentioned, but nothing uses them. I figured I would match tags
before I started getting data from the APIs. Once I realized I was able to match
`d.Name` with `q.Title` I didn't need to use tags. It all works out automagically.

## Installation

#### Docker

-   Copy the [example config file](examples/du.conf.example) from this repo (or find it in the container).
-   Then grab the image from docker hub and run it using an overlay for the config file.
```
docker pull golift/deluge-unpacker
docker run -d -v /your/config/du.conf:/etc/deluge-unpacker/du.conf golift/deluge-unpacker
docker logs <container id from docker run>
```

#### Linux / FreeBSD

-   Download a package from the [Releases](https://github.com/davidnewhall/deluge-unpacker/releases) page.
-   These are new and barely tested. Feedback welcomed.
-   Install it, edit config, start it.
```shell
dpkg -i deluge-unpacker*.deb || rpm -Uvh deluge-unpacker*.rpm || pkg install deluge-unpacker*.txz
edit /etc/deluge-unpacker/du.conf
sudo systemctl start deluge-unpacker || service deluge-unpacker start
```

#### macOS

-   Use homebrew.
```shell
brew tap golift/mugs
brew install deluge-unpacker
```
-   Edit config file at `/etc/deluge-unpacker/du.conf`
-   Start it
```shell
brew services start deluge-unpacker
```

#### Manually

-   Setup a working Go build environment.
-   Build the app like any other Go app (or run `make`).
-   Copy the binary to `/usr/local/bin` (mac) or `/usr/bin` (linux)
-   Make a config folder: `sudo mkdir /usr/local/etc/deluge-unpacker` (mac) or `/etc/deluge-unpacker` (linux)
-   Copy the example config: `sudo cp du.conf.example /etc/deluge-unpacker/`
-   On macOS, copy the launchd file: `cp init/launchd/* ~/Library/LaunchAgents`
-   On Linux, copy the systemd unit: `sudo cp init/systemd/* /etc/systemd/system`

After the app is installed, update your deluge, sonarr and radarr configuration
in `/etc/deluge-unpacker/du.conf`. The app works without Sonarr or Radarr
configs, but you should have at least one to make it useful.

-   Start the service, Linux: `sudo systemctl daemon-reload ; sudo systemctl restart deluge-unpacker`
-   Start the service, macOS: `launchctl load ~/Library/LaunchAgents/com.github.davidnewhall.deluge-unpacker.plist`

## Troubleshooting

The `http_` config options are for basic http auth. Most users will need to
leave these blank. I was using them to test my connection through an authenticated
nginx proxy. I did not test with basic auth enabled in Sonarr/Radarr. They may
or may not work for that. If you need different features, open an Issue and let me
know. Generally, you'll point all endpoints at localhost, without nginx and without
basic auth.

-   Log file is (hopefully) at `/usr/local/var/log/deluge-unpacker.log` (it's in syslog or messages on Linux)
-   On macOS, Deluge log is at `~/.config/deluge/deluged.log`
-   This works on Linux, others use it, but I personally run it on a mac. Feedback welcomed.

If transfers are in a Warning or Error state they will not be extracted. Try
the Force Recheck option in Deluge.

## Notes

While writing this, I kept finding Deluge unresponsive. After finding and inspecting
the Deluge log file, I found that the app was running out of open files. Turns out
this was causing a lot of issues on my server. I have a Mac. Check this out if you're
in the same boat:
[http://blog.mact.me/2014/10/22/yosemite-upgrade-changes-open-file-limit](http://blog.mact.me/2014/10/22/yosemite-upgrade-changes-open-file-limit)

Deluge takes a while to reply with a lot of transfers. Set the timeout to 30+s.
I use 60s on my server and it seems to be okay with around 600-800 transfers.

## TODO

-   Add code for tagged downloads. Allow extracting things besides radarr/sonarr.
-   Integrate `prometheus`.
-   Tests. Maybe. Would likely have to refactor things into better interfaces.

## License

[MIT](LICENSE) - Copyright (c) 2018 David Newhall II
