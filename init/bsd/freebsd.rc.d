#!/bin/sh
#
# FreeBSD rc.d startup script for unpackerr.
#
# PROVIDE: unpackerr
# REQUIRE: networking syslog
# KEYWORD:
#
# Add the following line to /etc/rc.conf or use `sysrc` to enable unpackerr.
# ${unpackerr_enable="YES"}
# Optionally there are other parameters:
# ${unpackerr_user="unpackerr"}
# ${unpackerr_group="unpackerr"}
# ${unpackerr_config="/usr/local/etc/unpackerr/unpackerr.conf"}

. /etc/rc.subr

name="unpackerr"
rcvar="unpackerr_enable"
unpackerr_command="/usr/local/bin/${name}"
pidfile="/var/run/${name}/pid"
# Suck in optional exported override variables. See: https://unpackerr.zip/docs/install/configuration
# ie. add something like the following to this file: export UN_DEBUG=true
unpackerr_env_file="/usr/local/etc/defaults/${name}"

load_rc_config ${name}
: ${unpackerr_enable:=NO}
: ${unpackerr_user:="unpackerr"}
: ${unpackerr_group:="unpackerr"}
: ${unpackerr_config:="/usr/local/etc/unpackerr/unpackerr.conf"}

# This runs `daemon` as the `unpackerr_user` user using `chroot`.
command="/usr/sbin/daemon"
command_args="-P ${pidfile} -r -t ${name} -T ${name} -l daemon ${unpackerr_command} -c ${unpackerr_config}"

start_precmd=${name}_precmd
unpackerr_precmd() {
  # Make a place for the pid file.
  mkdir -p $(dirname ${pidfile})
  chown -R $unpackerr_user $(dirname ${pidfile})
}

# Go!
run_rc_command "$1"
