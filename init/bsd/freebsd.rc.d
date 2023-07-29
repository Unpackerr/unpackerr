#!/bin/sh
#
# FreeBSD rc.d startup script for unpackerr.
#
# PROVIDE: unpackerr
# REQUIRE: networking syslog
# KEYWORD:

. /etc/rc.subr

name="unpackerr"
real_name="unpackerr"
rcvar="unpackerr_enable"
unpackerr_command="/usr/local/bin/${real_name}"
unpackerr_user="unpackerr"
unpackerr_config="/usr/local/etc/${real_name}/unpackerr.conf"
pidfile="/var/run/${real_name}/pid"

# This runs `daemon` as the `unpackerr_user` user.
command="/usr/sbin/daemon"
command_args="-P ${pidfile} -r -t ${real_name} -T ${real_name} -l daemon ${unpackerr_command} -c ${unpackerr_config}"

load_rc_config ${name}
: ${unpackerr_enable:=no}

# Make a place for the pid file.
mkdir -p $(dirname ${pidfile})
chown -R $unpackerr_user $(dirname ${pidfile})

# Suck in optional exported override variables.
# ie. add something like the following to this file: export UP_POLLER_DEBUG=true
[ -f "/usr/local/etc/defaults/${real_name}" ] && . "/usr/local/etc/defaults/${real_name}"

# Go!
run_rc_command "$1"
