<img src="https://raw.githubusercontent.com/wiki/davidnewhall/unpackerr/images/unpackerr-logo-text.png">

-   formerly `unpacker-poller`
-   formerly `deluge-unpacker`

## About

This application runs as a daemon on your download host. It checks for completed
downloads and extracts them so [Radarr](http://radarr.video), [Lidarr](http://lidarr.audio),
[Sonarr](http://sonarr.tv), and [Readarr](http://readarr.com) may import them.
There are a handful of options out there for extracting and deleting files after
your client downloads them. I just didn't care for any of them, so I wrote my own. I
wanted a small single-binary with reasonable logging that can extract downloaded
archives and clean up the mess after they've been imported.

## Installation

-   **Note**: Requires access to your download location.
    Make sure you set the `path` variables correctly in the configuration.
    Even if they're set incorrectly this app makes a best effort attempt to
    locate your downloads.

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

-   Copy the [example config file](https://github.com/davidnewhall/unpackerr/blob/master/examples/unpackerr.conf.example) from this repo.
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
debug|`UN_DEBUG`|`false` / Turns on more logs|
log_file|`UN_LOG_FILE`|None by default. Optionally provide a file path to write logs|
log_files|`UN_LOG_FILES`|`10` / Log files to keep after rotating. `0` disables rotation|
log_file_mb|`UN_LOG_FILE_MB`|`10` / Max size of log files in megabytes|
interval|`UN_INTERVAL`|`2m` / How often apps are polled, recommend `1m` to `5m`|
start_delay|`UN_START_DELAY`|`1m` / Files are queued at least this long before extraction|
retry_delay|`UN_RETRY_DELAY`|`5m` / Failed extractions are retried after at least this long|
max_retries|`UN_MAX_RETRIES`|`3` / Times to retry failed extractions. `0` = unlimited.|
parallel|`UN_PARALLEL`|`1` / Concurrent extractions, only recommend `1`|
file_mode|`UN_FILE_MODE`|`0644` / Extracted files are written with this mode|
dir_mode|`UN_DIR_MODE`|`0755` / Extracted folders are written with this mode|

##### Sonarr

|Config Name|Variable Name|Default / Note|
|---|---|---|
sonarr.url|`UN_SONARR_0_URL`|No Default. Something like: `http://localhost:8989`|
sonarr.api_key|`UN_SONARR_0_API_KEY`|No Default. Provide URL and API key if you use Sonarr|
sonarr.paths|`UN_SONARR_0_PATHS_0`|`/downloads` List of paths where content is downloaded for Sonarr|
sonarr.protocols|`UN_SONARR_0_PROTOCOLS`|`torrent` Protocols to process. Alt: `torrent,usenet`|
sonarr.timeout|`UN_SONARR_0_TIMEOUT`|`10s` / How long to wait for the app to respond|
sonarr.delete_orig|`UN_SONARR_0_DELETE_ORIG`|`false` / Delete archives after import? Recommend not setting this to true|
sonarr.delete_delay|`UN_SONARR_0_DELETE_DELAY`|`5m` / Extracts are deleted this long after import, `-1` to disable|

##### Radarr

|Config Name|Variable Name|Default / Note|
|---|---|---|
radarr.url|`UN_RADARR_0_URL`|No Default. Something like: `http://localhost:7878`|
radarr.api_key|`UN_RADARR_0_API_KEY`|No Default. Provide URL and API key if you use Radarr|
radarr.paths|`UN_RADARR_0_PATHS_0`|`/downloads` List of paths where content is downloaded for Radarr|
radarr.protocols|`UN_RADARR_0_PROTOCOLS`|`torrent` Protocols to process. Alt: `torrent,usenet`|
radarr.timeout|`UN_RADARR_0_TIMEOUT`|`10s` / How long to wait for the app to respond|
radarr.delete_orig|`UN_RADARR_0_DELETE_ORIG`|`false` / Delete archives after import? Recommend not setting this to true|
radarr.delete_delay|`UN_RADARR_0_DELETE_DELAY`|`5m` / Extracts are deleted this long after import, `-1` to disable|

##### Lidarr

|Config Name|Variable Name|Default / Note|
|---|---|---|
lidarr.url|`UN_LIDARR_0_URL`|No Default. Something like: `http://localhost:8686`|
lidarr.api_key|`UN_LIDARR_0_API_KEY`|No Default. Provide URL and API key if you use Lidarr|
lidarr.paths|`UN_LIDARR_0_PATHS_0`|`/downloads` List of paths where content is downloaded for Lidarr|
lidarr.protocols|`UN_LIDARR_0_PROTOCOLS`|`torrent` Protocols to process. Alt: `torrent,usenet`|
lidarr.timeout|`UN_LIDARR_0_TIMEOUT`|`10s` / How long to wait for the app to respond|
lidarr.delete_orig|`UN_LIDARR_0_DELETE_ORIG`|`false` / Delete archives after import? Recommend not setting this to true|
lidarr.delete_delay|`UN_LIDARR_0_DELETE_DELAY`|`5m` / Extracts are deleted this long after import, `-1` to disable|

##### Readarr

|Config Name|Variable Name|Default / Note|
|---|---|---|
readarr.url|`UN_READARR_0_URL`|No Default. Something like: `http://localhost:8787`|
readarr.api_key|`UN_READARR_0_API_KEY`|No Default. Provide URL and API key if you use Readarr|
readarr.paths|`UN_READARR_0_PATHS_0`|`/downloads` List of paths where content is downloaded for Readarr|
readarr.protocols|`UN_READARR_0_PROTOCOLS`|`torrent` Protocols to process. Alt: `torrent,usenet`|
readarr.timeout|`UN_READARR_0_TIMEOUT`|`10s` / How long to wait for the app to respond|
readarr.delete_orig|`UN_READARR_0_DELETE_ORIG`|`false` / Delete archives after import? Recommend not setting this to true|
readarr.delete_delay|`UN_READARR_0_DELETE_DELAY`|`5m` / Extracts are deleted this long after import, `-1` to disable|

##### Folder

Folders are a way to watch a folder for things to extract. You can use this to
monitor your download client's "move to" path if you're not using it with an *arr app.

|Config Name|Variable Name|Default / Note|
|---|---|---|
folder.path|`UN_FOLDER_0_PATH`|No Default; folder to watch for archives. **Not for Starr apps**|
folder.extract_path|`UN_FOLDER_0_EXTRACT_PATH`|Where to extract to. Default is the same as `path`|
folder.delete_after|`UN_FOLDER_0_DELETE_AFTER`|`10m` Delete extracted files and/or archives after this duration; `0` disables|
folder.delete_original|`UN_FOLDER_0_DELETE_ORIGINAL`|`false` Delete archives after successful extraction|
folder.delete_files|`UN_FOLDER_0_DELETE_FILES`|`false` Delete extracted files after successful extraction|
folder.move_back|`UN_FOLDER_0_MOVE_BACK`|`false` Move extracted items back into original folder|

##### Webhooks

This application can send a POST webhook to a URL when an extraction begins, and again
when it finishes. Configure 1 or more webhook URLs with the parameters below.
Works great with [notifiarr.com](https://notifiarr.com). You can use
[requestbin.com](https://requestbin.com/r/) to test and _see_ the payload.

|Config Name|Variable Name|Default / Note|
|---|---|---|
webhook.url|`UN_WEBHOOK_0_URL`|No Default; URL to send POST webhook to|
webhook.name|`UN_WEBHOOK_0_NAME`|Defaults to URL; provide an optional name to hide the URL in logs|
webhook.nickname|`UN_WEBHOOK_0_NICKNAME`|`Unpackerr` / Passed into templates for telegram, discord and slack hooks|
webhook.channel|`UN_WEBHOOK_0_CHANNEL`|`""` / Passed into templates for slack.com webhooks|
webhook.timeout|`UN_WEBHOOK_0_TIMEOUT`|Defaults to global timeout, usually `10s`|
webhook.silent|`UN_WEBHOOK_0_SILENT`|`false` / Hide successful POSTs from logs|
webhook.ignore_ssl|`UN_WEBHOOK_0_IGNORE_SSL`|`false` / Ignore invalid SSL certificates|
webhook.exclude|`UN_WEBHOOK_0_EXCLUDE`|`[]` / List of apps to exclude: radarr, sonarr, folders, etc|
webhook.events|`UN_WEBHOOK_0_EVENTS`|`[0]` / List of event IDs to send (shown below)|
webhook.template_path|`UN_WEBHOOK_0_TEMPLATE_PATH`|`""` / Instead of an internal template, provide your own|
webhook.content_type|`UN_WEBHOOK_0_CONTENT_TYPE`|`application/json` / Content-Type header sent to webhook|

Event IDs (not all of these are used in webhooks): `0` = all,
`1` = queued, `2` = extracting, `3` = extract failed, `4` = extracted,
`5` = imported, `6` = deleting, `7` = delete failed, `8` = deleted

###### Webhook Notes

1. _`Nickname` should equal the `chat_id` value in Telegram webhooks._
1. _`Channel` is used as destination channel for Slack. It's not used in others._
1. _`Nickname` and `Channel` may be used as custom values in custom templates._
1. _`Name` is only used in logs, but it's also available as a template value as `{{name}}`._

##### Example Usage

```shell
docker pull golift/unpackerr
docker run -d -v /mnt/HostDownloads:/downloads -e "UN_SONARR_0_URL=http://localhost:8989" -e "UN_SONARR_0_API_KEY=kjsdkasjdaksdj" golift/unpackerr
docker logs <container id from docker run>
```

#### More Dockers!

 If you want a container that has a bit more to it, you can try a third party option.
 The container provided by golift is from scratch so it has nothing more than a binary
 and a config file (with our defaults).

-   **[@madcatsu](https://github.com/madcatsu) maintains an
    [Alpine Docker Container](https://hub.docker.com/r/madcatsu/unpackerr-alpine-daemon)
    for Unpackerr.** ([repo](https://gitlab.com/madcatsu/docker-unpackerr-alpine-daemon))

-   **[@hotio](https://github.com/hotio) maintains a
    [Custom Docker Container](https://hub.docker.com/r/hotio/unpackerr)
    for Unpackerr.** ([repo](https://github.com/hotio/docker-unpackerr))

### Linux and FreeBSD Install

-   Download a package from the [Releases](https://github.com/davidnewhall/unpackerr/releases) page.
-   Install it, edit config, start it.

Example of the above in shell form:

```shell
wget -qO- https://golift.io/unpackerr/raw/master/scripts/install.sh | sudo bash

nano /etc/unpackerr/unpackerr.conf         # linux
vi /usr/local/etc/unpackerr/unpackerr.conf # freebsd

sudo systemctl restart unpackerr    # linux
service unpackerr start             # freebsd
```

On Linux, unpackerr runs as `user:group` `unpackerr:unpackerr`. You will need to give that
user or group read and write access to your archives. That may mean adding the `unpackerr`
user, for example, to the `debian-transmission` group.

On FreeBSD the app runs as `nobody`. That's not very good and will probably change in the future.

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

You can also use a GUI app on a Mac instead of CLI via Homebrew:

-   Download a `.dmg` file from [the Releases page](https://github.com/davidnewhall/unpackerr/releases).
-   Copy the `Unpackerr.app` to `/Applications`.
-   Run it. It starts in the menu bar as an icon.
-   Click the menu bar icon and select `Config` -> `Edit`.
-   Edit the config to suit your system and save.
-   Click the menu bar icon again and select `Config` -> `Reload`.
-   View the logs by clicking the menu bar icon and `Logs` -> `View`.
-   You can add it to login items to run it automatically when you login.

The `.app` and the Homebrew version are the same application, but one runs in GUI mode and one does not.

### Windows Install

-   Extract a `.exe.zip` file from [the Releases page](https://github.com/davidnewhall/unpackerr/releases) into a folder like `C:\Program Files\unpackerr\`.
-   Run the `unpackerr.amd64.exe` binary. This starts the app in the system tray.
-   Click the systray icon and select `Config` -> `Edit`.
-   Edit the config to suit your system and save.
-   Click the systray icon again and select `Config` -> `Reload`.
-   View the logs by clicking the systray icon and `Logs` -> `View`.
-   Make a shortcut to the application in your Startup menu to run it when you login.

## Troubleshooting

Make sure your Downloads location matches on all your applications!
[Find help on Discord](https://golift.io/discord).

Log files:

-   Linux: `/var/log/messages` or `/var/log/syslog` (w/ default syslog)
-   FreeBSD: `/var/log/syslog` (w/ default syslog)
-   macOS: `/usr/local/var/log/unpackerr.log` or `~/.unpackerr/unpackerr.log`
-   Windows: `~/.unpackerr/unpackerr.log`

If transfers are in a Warning or Error state they will not be extracted.
If Unpackerr prints information about transfers you do not see in your Starr app.

Still having problems?
[Let me know!](https://github.com/davidnewhall/unpackerr/issues/new)

## Logic

The application polls radarr, readarr, sonarr and lidarr at the interval configured.
The queued items are inspected for completeness. The interval of these pollers is set
in the config file. 1-10 minutes is generally sufficient.

When Unpackerr finds an item in Sonarr or Radarr or Lidarr the download location
is checked for a `.rar` file. If an extractable archive exists, and **Sonarr/Radarr/Lidarr
has `status=Completed` from your download client** Unpackerr will extract the file.
Files are extracted to a temporary folder, and then moved back into the download
location for Completed Download Handling to import them. When the item falls out of the
(Radarr/Sonarr/Lidarr) queue, the extracted files are deleted.

## Contributing

Yes, please.

## License

[MIT](LICENSE) - Copyright (c) 2018-2021 David Newhall II
