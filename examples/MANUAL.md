unpackerr(1) -- Utility Unpack compressed files for importing by Sonarr and Radarr.
===

SYNOPSIS
---

`unpackerr -c /etc/unpackerr/unpackerr.conf`

DESCRIPTION
---
*   This application extracts downloaded files and makes sure
    Radarr / Sonarr imported the extracted files before deleting them.

*   Other tunable and configurable options are available in the config file.

OPTIONS
---
`unpackerr [-c <config file>] [-h] [-v]`

    -c, --config <file_path>
        Provide a configuration file.
        Default: /etc/unpackerr/unpackerr.conf

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
*   [github.com/davidnewhall/unpackerr](https://github.com/davidnewhall/unpackerr)
*   /usr/local/bin/unpackerr
