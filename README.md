# Deluge Unpacker

-   formerly `unpacker-poller`

## About

This application runs as a daemon on your Deluge host. It checks for completed
downloads and extracts them so Radarr and/or Sonarr may import them.

There are a handful of options out there for extracting and deleting files after
Deluge downloads them. I just didn't care for any of them, so I wrote my own. I
wanted a small single-binary with reasonable logging that can extract downloaded
archives and clean up the mess after they've been imported. Why a separate binary
instead of a Deluge plugin? Because I like Go more than Python, but I do love Python!

## Installation

### Setup

**The download paths for Deluge, Sonarr and Radarr must all match!** In Docker,
I just map everything to `/downloads` (in all four containers). You need to make
sure all the apps _see_ the downloaded items in the same location. This is how
Deluge Unpacker (this app) finds and extracts things. **This _probably_ means all
the apps need to run on the same machine**; I've never tried anything else.

### Docker

Several methods for Docker are described below.

#### unRAID (Docker)

-   Deluge Unpacker is available in the Community Applications on unRAID.

#### Docker Config File

-   Copy the [example config file](examples/du.conf.example) from this repo.
-   Then grab the image from docker hub and run it using an overlay for the config file.

```shell
docker pull golift/deluge-unpacker
docker run -d -v /mnt/HostDownloads:/downloads -v /your/config/du.conf:/etc/deluge-unpacker/du.conf golift/deluge-unpacker
docker logs <container id from docker run>
```


#### Docker Env Variables

-   Instead of a config file, you may configure the docker container
    with environment variables.
-   Any variable not provided takes the default.
-   Must provide URL and API key for Sonarr or Radarr, or both.

|Config Name|Variable Name|Default / Note|
|---|---|---|
debug|DU_DEBUG|`false` / Turns on more logs
interval|DU_INTERVAL|`4m` / How often apps are polled, recommend `2m`-`10m`
timeout|DU_TIMEOUT|`10s` / Global API Timeouts (all apps default)
delete_delay|DU_DELETE_DELAY|`10m` / Extracts are deleted this long long after import
parallel|DU_PARALLEL|`1` / Concurrent extractions, only recommend `1`
deluge.url|DU_DELUGE_URL|`http://127.0.0.1:8112` / Deluge URL, **required**!
deluge.password|DU_DELUGE_PASSWORD|`deluge` / Deluge password **_must_** be set.
deluge.timeout|DU_DELUGE_TIMEOUT|`1m` / Deluge API can be slow with lots of downloads
sonarr.url|DU_SONARR_URL|No Default. Something like: `http://localhost:8989`
sonarr.api_key|DU_SONARR_API_KEY|No Default. Provide URL and API key if you use Sonarr
radarr.url|DU_RADARR_URL|No Default. Something like: `http://localhost:7878`
radarr.api_key|DU_RADARR_API_KEY|No Default. Provide URL and API key if you use Radarr

- Example:

```shell
docker pull golift/deluge-unpacker
docker run -d -v /mnt/HostDownloads:/downloads -e "DU_SONARR_URL=http://localhost:8989" -e "DU_SONARR_API_KEY=kjsdkasjdaksdj" golift/deluge-unpacker
docker logs <container id from docker run>
```

#### Alpine Docker Container

If you want a container that has a bit more to it, you can try a third party option.
The container provided by golift is from scratch so it has nothing more than a binary
and a config file (with our defaults).

- **[@madcatsu](https://github.com/madcatsu) maintains an
[Alpine Docker Container](https://hub.docker.com/r/madcatsu/deluge-unpacker-daemon)
for Deluge Unpacker.**

### Linux and FreeBSD Install

-   Download a package from the [Releases](https://github.com/davidnewhall/deluge-unpacker/releases) page.
-   Install it, edit config, start it.

Example of the above in shell form:

```shell
wget -qO- https://raw.githubusercontent.com/davidnewhall/deluge-unpacker/master/scripts/install.sh | sudo sh

nano /etc/deluge-unpacker/du.conf         # linux
vi /usr/local/etc/deluge-unpacker/du.conf # freebsd

sudo systemctl restart deluge-unpacker    # linux
service deluge-unpacker start             # freebsd
```

### macOS Install

-   Use homebrew.
-   Edit config file at `/usr/local/etc/deluge-unpacker/du.conf`
-   Start it.
-   Like this:

```shell
brew install golift/mugs/deluge-unpacker
vi /usr/local/etc/deluge-unpacker/du.conf
brew services start deluge-unpacker
```

## Troubleshooting

Make sure your Downloads location matches on all your applications!

Log files:

-   Linux: `/var/log/messages` or `/var/log/syslog` (w/ default syslog)
-   FreeBSD: `/var/log/syslog` (w/ default syslog)
-   macOS: `/usr/local/var/log/deluge-unpacker.log`
-   On macOS, Deluge log is at `~/.config/deluge/deluged.log`

If transfers are in a Warning or Error state they will not be extracted.
Try the Force Recheck option in Deluge.

Still having problems?
[Let me know!](https://github.com/davidnewhall/deluge-unpacker/issues/new)

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

## Notes

While writing this, I kept finding Deluge unresponsive. After finding and inspecting
the Deluge log file, I found that the app was running out of open files. Turns out
this was causing a lot of issues on my server. Check this out if you're
using a mac:
[http://blog.mact.me/2014/10/22/yosemite-upgrade-changes-open-file-limit](http://blog.mact.me/2014/10/22/yosemite-upgrade-changes-open-file-limit)

Deluge takes a while to reply with a lot of transfers. Set the timeout to 30+s.
I use 60s on my server and it seems to be okay with around 600-800 transfers.

## TODO

Honestly I don't have a lot of time for this app and these things are just a wish list.
I'm surprised making this work with Deluge 2 was simple and making it with work with Radarr
and Sonarr v3 has been "easy" too. If these tweaks stay easy, I'll keep making them, and
keep making this app useful. I didn't expect so many people to want to use this, but I'm
happy it's working so well!

-   Add code for tagged downloads. Allow extracting things besides radarr/sonarr.
-   Integrate `prometheus`.
-   Tests. Maybe. Would likely have to refactor things into better interfaces.

## Contributing

Yes, please.

## License

[MIT](LICENSE) - Copyright (c) 2018 David Newhall II
