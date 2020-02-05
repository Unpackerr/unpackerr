deluge-unpacker(1) -- Utility to poll Deluge and Unpack files for tracking clients.
===

SYNOPSIS
---

`deluge-unpacker -c /etc/deluge-unpacker/du.conf`

DESCRIPTION
---
*   This application polls Deluge (and maybe other clients in the future),
to find finished transfers. It extracts the downloaded files, then polls
Radarr and Sonarr to make sure they've imported the extracted files before
deleting them.

*   Other tunable and configurable options are available in the config file.

OPTIONS
---
`deluge-unpacker [-c <config file>] [-h] [-v]`

    -c, --config <file_path>
        Provide a configuration file.
        Default: /etc/deluge-unpacker/du.conf

    -v, --version
        Display version and exit.

    -h, --help
        Display usage and exit.


GO DURATION
---
This application uses the Go Time Durations for a polling interval.
The format is an integer followed by a time unit. You may append
multiple time units to add them together. Some valid time units are:

     `ms` (millisecond)
     `s`  (second)
     `m`  (minute)
     `h`  (hour)

Example Use: `1m`, `5h`, `100ms`, `17s`, `1s45ms`, `1m3s`

AUTHOR
---
*   David Newhall II - 5/6/2018

LOCATION
---
*   [github.com/davidnewhall/deluge-unpacker](https://github.com/davidnewhall/deluge-unpacker)
*   /usr/local/bin/deluge-unpacker
