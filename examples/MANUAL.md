unpackerr(1) -- Unpack compressed files for importing by Starr applications.
===

SYNOPSIS
---

`unpackerr -c /etc/unpackerr/unpackerr.conf`

DESCRIPTION
---
*   This application extracts downloaded archives then makes sure
    a Starr app imports the extracted files before deleting them.

*   Provides the ability to extract items that are copied or moved 
    into a 'watch' folder. Example config file has those settings.

*   Other configuration settings are available in the config file.

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
        Event IDs (not all of these are used in webhooks): 0 = all
        1 = queued, 2 = extracting, 3 = extract failed, 4 = extracted
        5 = imported, 6 = deleting, 7 = delete failed, 8 = deleted

    -v, --version
        Display version and exit.

    -h, --help
        Display usage and exit.


GO DURATION
---
This application uses Go Time Durations for intervals, like polling and timeout.
The format is an integer followed by a time unit. 
Appending multiple time units sums them. 
Some valid time units are:

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
