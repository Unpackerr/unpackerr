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
`unpackerr [-c <config file>] [-p <env prefix>] [-h] [-v]`

    -c, --config <file path>
        Provide a configuration file.
        Default: /etc/unpackerr/unpackerr.conf

    -p, --prefix <env prefix>
        This argument allows changing the environment variable prefix.
        This application parses environment variables into config data.
        The default prefix is UN, making env variables like UN_SONARR_URL.

    -w, --webhook <1,2,3,4,5,6,7,8>
        This sends a webhook of the type specified then exits. This is only
        for testing and development. This requires a valid webhook configured
        in a config file or from environment variables.

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
