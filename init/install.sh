#!/bin/bash
##########################################################################################
#
# This simple script to install the latest package.
#
# Use it like this, pick curl or wget:  (sudo is optional)
# ----
#   curl -sL https://raw.githubusercontent.com/Unpackerr/unpackerr/main/init/install.sh | sudo bash
#   wget -qO- https://raw.githubusercontent.com/Unpackerr/unpackerr/main/init/install.sh | sudo bash
# ----
#
# - If you're on RedHat/CentOS/Fedora, installs the latest rpm package.
# - If you're on Debian/Ubuntu/Knoppix, installs the latest deb package.
# - If you're on Arch Linux, installs the latest zst (pacman) package.
# - If you're on FreeBSD, installs the latest txz package.
##########################################################################################


# Set the repo name correctly.
REPO=Unpackerr/unpackerr
PACKAGE=$(echo "$REPO" | cut -d/ -f 2)

##########################################################################################
############# Nothing else needs to be changed. Unless you're fixing things! #############
##########################################################################################

LATEST="https://api.github.com/repos/${REPO}/releases/latest"
ISSUES="https://github.com/${REPO}/issues/new"
ARCH=$(uname -m)
OS=$(uname -s)
P=" ==>"

echo "<-------------------------------------------------->"

if [[ $OS = Darwin ]]; then
  echo "${P} This script does not work on macOS. Download a DMG here: https://github.com/${REPO}/releases/latest"
  exit 1
fi

# This file contains ID and possibly ID_LIKE.
[[ ! -f /etc/os-release ]] || . /etc/os-release

# $ARCH is passed into grep -E to find the right file.
if [[ $ARCH = x86_64 ]] || [[ $ARCH = amd64 ]]; then
  ARCH="x86_64|amd64"
elif [[ $ARCH = *386* ]] || [[ $ARCH = *686* ]]; then
  ARCH="i386"
elif [[ $ARCH = *arm64* ]] || [[ $ARCH = *armv8* ]] || [[ $ARCH = *aarch64* ]]; then
  ARCH="arm64"
elif [[ $ARCH = *armv6* ]] || [[ $ARCH = *armv7* ]]; then
  ARCH="armhf"
else
  echo "${P} [ERROR] Unknown Architecture: ${ARCH}"
  echo "${P} $(uname -a) | $(head -n 1 /etc/issue): ${ID}"
  echo "${P} Please report this error, along with the above OS details:"
  echo "     ${ISSUES}"
  exit 1
fi

if [[ $1 = deb ]] || [[ $1 = rpm ]] || [[ $1 = txz ]] || [[ $1 = zst ]] || [[ $1 = pkg.tar.zst ]]; then
  [[ $1 = zst ]] && FILE=pkg.tar.zst || FILE="$1"
elif pacman --version > /dev/null 2>&1 && [[ $ID = arch ]] ; then
  FILE=pkg.tar.zst
elif rpm --version > /dev/null 2>&1 && [[ $ID != debian ]] && [[ $ID_LIKE != debian ]]; then
  FILE=rpm
elif dpkg --version > /dev/null 2>&1; then
  FILE=deb
elif pkg --version > /dev/null 2>&1; then
  FILE=txz
fi

if [[ -z ${FILE} ]]; then
  echo "${P} [ERROR] No pacman (arch), pkg (freebsd), dpkg (debian) or rpm (redhat) package managers found; not sure what package to download!"
  echo "${P} $(uname -a) | $(head -n 1 /etc/issue): ${ID}"
  echo "${P} If you feel this is a mistake, please report this along with the above OS details:"
  echo "     ${ISSUES}"
  exit 1
fi

# curl or wget?
if curl --version > /dev/null 2>&1; then
  CMD="curl -sL"
elif wget --version > /dev/null 2>&1; then
  CMD="wget -qO-"
else
  echo "${P} [ERROR] Could not locate curl nor wget - please install one to download packages!"
  exit 1
fi

# Grab latest release file from github.
PAYLOAD=$($CMD ${LATEST})
URL=$(echo "$PAYLOAD" | grep -E "browser_download_url.*(${ARCH})\.${FILE}\"" | cut -d\" -f 4)
TAG=$(echo "$PAYLOAD" | grep 'tag_name' | cut -d\" -f4 | tr -d v)

if [[ -z ${TAG} ]] || [[ -z ${URL} ]]; then
  echo "${P} [ERROR] Missing latest release for '${FILE}' file ($OS/${ARCH}) at ${LATEST}"
  echo "${P} $(uname -a) | $(head -n 1 /etc/issue): ${ID}"
  echo "${P} Please report this error, along with the above OS details:"
  echo "     ${ISSUES}"
  exit 1
fi

case "${FILE}" in
  "rpm")
    INSTALLER="rpm -Uvh"
    INSTALLED=$(rpm -q --last --info "${PACKAGE}" 2>/dev/null | grep Version | cut -d: -f2 | cut -d- -f1 | awk '{print $1}')
    ;;
  "deb")
    INSTALLED=$(dpkg -s "${PACKAGE}" 2>/dev/null | grep ^Version | cut -d: -f2 | cut -d- -f1 | awk '{print $1}')
    dpkg -s "${PACKAGE}" 2>/dev/null | grep -qE 'Status.*installed' || INSTALLED=""
    INSTALLER="dpkg --force-confdef --force-confold --install"
    ;;
  "txz")
    INSTALLER="pkg install --yes"
    INSTALLED=$(pkg info" ${PACKAGE}" 2>/dev/null | grep Version | cut -d: -f2 | cut -d- -f1 | awk '{print $1}')
    ;;
  "pkg.tar.zst")
    INSTALLER="pacman --noconfirm --upgrade"
    INSTALLED=$(pacman --query "${PACKAGE}" 2>/dev/null | awk '{print $2}' | cut -d- -f1)
    EXTRAS="$CMD https://golift.io/gpg | pacman-key --add -
      pacman-key --lsign-key B93DD66EF98E54E2EAE025BA0166AD34ABC5A57C"
    ;;
  *) echo "${P} Unknown file type: ${FILE}" ; exit 1 ;;
esac

# https://stackoverflow.com/questions/4023830/how-to-compare-two-strings-in-dot-separated-version-format-in-bash
vercomp () {
  if [[ -z $1 ]]; then
    return 3
  elif [[ "$1" = "$2" ]]; then
    return 0
  fi

  local IFS=.
  local i ver1=("$1") ver2=("$2")
  # fill empty fields in ver1 with zeros
  for ((i=${#ver1[@]}; i<${#ver2[@]}; i++)); do
    ver1[i]=0
  done

  for ((i=0; i<${#ver1[@]}; i++)); do
    if [[ -z ${ver2[i]} ]]; then
      # fill empty fields in ver2 with zeros
      ver2[i]=0
    elif ((10#${ver1[i]} > 10#${ver2[i]})); then
      return 1
    elif ((10#${ver1[i]} < 10#${ver2[i]})); then
      return 2
    fi
  done
  return 0
}

vercomp "$INSTALLED" "$TAG"
case $? in
  0) echo "${P} The installed version of ${PACKAGE} (${INSTALLED}) is current: ${TAG}" ; exit 0 ;;
  1) echo "${P} The installed version of ${PACKAGE} (${INSTALLED}) is newer than the current release (${TAG})" ; exit 0 ;;
  2) echo "${P} Upgrading ${PACKAGE} to ${TAG} from ${INSTALLED}." ;;
  3) echo "${P} Installing ${PACKAGE} version ${TAG}." ;;
esac

FILE=$(basename "${URL}")
TMPFILE=$(mktemp --tmpdir "XXXX-${FILE}")
echo "${P} Downloading: ${URL}"
echo "${P} To Location: ${TMPFILE}"

if ! $CMD "${URL}" > "${TMPFILE}"; then
  echo "${P} Error writing '${TMPFILE}' file! Fix that, and run this again."
  exit 1
fi

# Install it.
if (( $(id -u) == 0 )); then
  echo "${P} Downloaded. Installing the package!"
  echo "${P} Executing: ${EXTRAS}"
  eval "${EXTRAS}"
  echo "${P} Executing: ${INSTALLER} ${TMPFILE}"
  $INSTALLER "${TMPFILE}"
  echo "<-------------------------------------------------->"
else
  echo "${P} Downloaded! Install the package like this:"
  [[ -z ${EXTRAS} ]] || echo "     ${EXTRAS}"
  echo "     sudo $INSTALLER ${TMPFILE}"
fi
