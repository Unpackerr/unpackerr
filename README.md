# Unpackerr

-   formerly `unpacker-poller`
-   formerly `deluge-unpacker`

## About

This application runs as a daemon on your download host. It checks for completed
downloads and extracts them so Radarr and/or Sonarr and/or Lidarr may import them.

There are a handful of options out there for extracting and deleting files after
your client downloads them. I just didn't care for any of them, so I wrote my own. I
wanted a small single-binary with reasonable logging that can extract downloaded
archives and clean up the mess after they've been imported.

## Installation

-   **Note**: Requires access to your download location.
    Make sure you set the `path` variables correctly in the configuration.

### Docker

This project builds automatically in [Docker Cloud](https://hub.docker.com/r/golift/unpackerr)
and creates [ready-to-use multi-architecture images](https://hub.docker.com/r/golift/unpackerr/tags).
The `latest` tag is always a tagged release in GitHub. The `master` tag corresponds
to the `master` branch in GitHub and may be broken.

Use the methods below to install using Docker.

#### unRAID (Docker)

-   Unpackerr is available in the
    [Community Applications](https://github.com/selfhosters/unRAID-CA-templates/blob/master/templates/unpackerr.xml)
    on unRAID.

#### Docker Config File

-   Copy the [example config file](examples/unpackerr.conf.example) from this repo.
-   Then grab the image from docker hub and run it using an overlay for the config file.

```shell
docker pull golift/unpackerr
docker run -d -v /mnt/HostDownloads:/downloads -v /your/config/unpackerr.conf:/etc/unpackerr/unpackerr.conf golift/unpackerr
docker logs <container id from docker run>
```


#### Docker Env Variables

-   Instead of a config file, you may configure the docker container
    with environment variables.
-   Any variable not provided takes the default.
-   Must provide URL and API key for Sonarr or Radarr or Lidarr or any combination.
-   You may provide multiple sonarr, radarr or lidarr instances using

    `UN_SONARR_1_URL`, `UN_SONARR_2_URL`, etc.

|Config Name|Variable Name|Default / Note|
|---|---|---|
debug|`UN_DEBUG`|`false` / Turns on more logs
interval|`UN_INTERVAL`|`4m` / How often apps are polled, recommend `2m`-`10m`
timeout|`UN_TIMEOUT`|`10s` / Global API Timeouts (all apps default)
delete_delay|`UN_DELETE_DELAY`|`10m` / Extracts are deleted this long long after import|
start_delay|`UN_START_DELAY`|`1m` / Files are queued at least this long before extraction|
retry_delay|`UN_RETRY_DELAY`|`5m` / Failed extractions are retried after at least this long|
parallel|`UN_PARALLEL`|`1` / Concurrent extractions, only recommend `1`
sonarr.url|`UN_SONARR_0_URL`|No Default. Something like: `http://localhost:8989`
sonarr.api_key|`UN_SONARR_0_API_KEY`|No Default. Provide URL and API key if you use Sonarr
sonarr.path|`UN_SONARR_0_PATH`|`/downloads` Path where content is downloaded for Sonarr|
radarr.url|`UN_RADARR_0_URL`|No Default. Something like: `http://localhost:7878`
radarr.api_key|`UN_RADARR_0_API_KEY`|No Default. Provide URL and API key if you use Radarr
radarr.path|`UN_RADARR_0_PATH`|`/downloads` Path where content is downloaded for Radarr|
lidarr.url|`UN_LIDARR_0_URL`|No Default. Something like: `http://localhost:8686`
lidarr.api_key|`UN_LIDARR_0_API_KEY`|No Default. Provide URL and API key if you use Lidarr
lidarr.path|`UN_LIDARR_0_PATH`|`/downloads` Path where content is downloaded for Lidarr|


-   Example:

```shell
docker pull golift/unpackerr
docker run -d -v /mnt/HostDownloads:/downloads -e "UN_SONARR_0_URL=http://localhost:8989" -e "UN_SONARR_0_API_KEY=kjsdkasjdaksdj" golift/unpackerr
docker logs <container id from docker run>
```

### Linux and FreeBSD Install

-   Download a package from the [Releases](https://github.com/davidnewhall/unpackerr/releases) page.
-   Install it, edit config, start it.

Example of the above in shell form:

```shell
wget -qO- https://raw.githubusercontent.com/davidnewhall/unpackerr/master/scripts/install.sh | sudo sh

nano /etc/unpackerr/unpackerr.conf         # linux
vi /usr/local/etc/unpackerr/unpackerr.conf # freebsd

sudo systemctl restart unpackerr    # linux
service unpackerr start             # freebsd
```

### macOS Install

-   Use homebrew.
-   Edit config file at `/usr/local/etc/unpackerr/unpackerr.conf`
-   Start it.
-   Like this:

```shell
brew install golift/mugs/unpackerr
vi /usr/local/etc/unpackerr/unpackerr.conf
brew services start unpackerr
```

## Troubleshooting

Make sure your Downloads location matches on all your applications!

Log files:

-   Linux: `/var/log/messages` or `/var/log/syslog` (w/ default syslog)
-   FreeBSD: `/var/log/syslog` (w/ default syslog)
-   macOS: `/usr/local/var/log/unpackerr.log`

If transfers are in a Warning or Error state they will not be extracted.
Try the Force Recheck option if you use Deluge.

Still having problems?
[Let me know!](https://github.com/davidnewhall/unpackerr/issues/new)

## Logic

The application kicks up a go routine for each of Radarr
and Sonarr (if you include configs for them). These go routines just poll their
respective applications for transfers/queued items. The items are stored. The
interval of these pollers is set in the config file. 2-10 minutes is good.

Another go routine checks (the internal data) for completed downloads. When it
finds an item in Sonarr or Radarr the download
location is checked for a `.rar` file. If an extractable archive exists, and
**Sonarr/Radarr have `status=Completed` from your download client** this application will
extract the file. Files are extracted to a temporary folder, and then moved back
into the download location for Completed Download Handling to import them. When
the item falls out of the (Radarr/Sonarr) queue, the extracted files are deleted.

## TODO

Honestly I don't have a lot of time for this app and these things are just a wish list.
I'm surprised making this with work with Radarr
and Sonarr v3 has been _easy_. If these tweaks stay easy, I'll keep making them, and
keep making this app useful. I didn't expect so many people to want to use this, but I'm
happy it's working so well!

-   Add code for tagged downloads. Allow extracting things besides radarr/sonarr.
-   Integrate `prometheus`.
-   Tests. Maybe. Would likely have to refactor things into better interfaces.
-   Save and reload state. If you shut if off before it deletes something, it never gets deleted.

## Contributing

Yes, please.

## License

[MIT](LICENSE) - Copyright (c) 2018 David Newhall II
