#!/usr/bin/env bash
# Import Developer ID + App Store Connect API key into a temporary keychain
# for GoReleaser Pro macos_native (codesign + notarytool).
set -euo pipefail

if [ -z "${MACOS_SIGN_P12:-}" ] || [ -z "${MACOS_SIGN_PASSWORD:-}" ]; then
  echo "MACOS_SIGN_P12 / MACOS_SIGN_PASSWORD unset" >&2
  exit 1
fi
if [ -z "${MACOS_NOTARY_KEY:-}" ] || [ -z "${MACOS_NOTARY_KEY_ID:-}" ] || [ -z "${MACOS_NOTARY_ISSUER_ID:-}" ]; then
  echo "MACOS_NOTARY_KEY / MACOS_NOTARY_KEY_ID / MACOS_NOTARY_ISSUER_ID unset" >&2
  exit 1
fi

tmp="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
cert="${tmp}/unpackerr.p12"
key="${tmp}/unpackerr.p8"
keychain="${tmp}/unpackerr.keychain-db"
profile="${MACOS_NOTARY_PROFILE_NAME:-unpackerr}"
password="${KEYCHAIN_PASSWORD:-$(openssl rand -base64 32)}"

# Quill accepted "path or base64". GitHub secrets are either PEM text or
# one-line (or wrapped) base64. macOS openssl base64 -d without -A yields
# empty files for long lines; -A treats the whole buffer as one line.
write_secret() {
  local dest=$1 envname=$2 raw=$3
  local s compact mod
  s="${raw//$'\r'/}"
  s="${s#"${s%%[![:space:]]*}"}"
  s="${s%"${s##*[![:space:]]}"}"
  if [ -z "${s}" ]; then
    echo "${envname} empty" >&2
    exit 1
  fi
  if [[ "${s}" == *"BEGIN "* ]]; then
    printf '%s\n' "${s}" > "${dest}"
  else
    compact="$(printf '%s' "${s}" | tr -d '[:space:]')"
    mod=$(( ${#compact} % 4 ))
    case "${mod}" in
      0) ;;
      2) compact="${compact}==" ;;
      3) compact="${compact}=" ;;
      *)
        echo "${envname} base64 decode failed: length ${#compact} mod 4 = ${mod}" >&2
        exit 1
        ;;
    esac
    if ! printf '%s' "${compact}" | openssl base64 -d -A > "${dest}"; then
      echo "${envname} base64 decode failed" >&2
      exit 1
    fi
  fi
  if [ ! -s "${dest}" ]; then
    echo "${envname} decoded to empty (${#raw} input chars)" >&2
    exit 1
  fi
  echo "${envname}: ${#raw} chars -> $(wc -c < "${dest}" | tr -d ' ') bytes"
}

write_secret "${cert}" MACOS_SIGN_P12 "${MACOS_SIGN_P12}"
write_secret "${key}" MACOS_NOTARY_KEY "${MACOS_NOTARY_KEY}"
chmod 600 "${cert}" "${key}"
echo "p12 $(wc -c < "${cert}" | tr -d ' ') bytes ($(file -b "${cert}"))"
echo "p8 $(wc -c < "${key}" | tr -d ' ') bytes ($(file -b "${key}"))"

security delete-keychain "${keychain}" 2>/dev/null || true
security create-keychain -p "${password}" "${keychain}"
security set-keychain-settings -lut 21600 "${keychain}"
security unlock-keychain -p "${password}" "${keychain}"
security import "${cert}" -P "${MACOS_SIGN_PASSWORD}" -A -t cert -f pkcs12 -k "${keychain}"
security set-key-partition-list -S apple-tool:,apple: -k "${password}" "${keychain}"
security list-keychain -d user -s "${keychain}"

identity="${MACOS_SIGN_IDENTITY:-}"
if [ -z "${identity}" ]; then
  identity="$(security find-identity -v -p codesigning "${keychain}" | awk -F '"' '/Developer ID Application/{print $2; exit}')"
fi
if [ -z "${identity}" ]; then
  echo "no Developer ID Application identity in ${keychain}" >&2
  security find-identity -v -p codesigning "${keychain}" >&2 || true
  exit 1
fi

xcrun notarytool store-credentials "${profile}" \
  --key "${key}" \
  --key-id "${MACOS_NOTARY_KEY_ID}" \
  --issuer "${MACOS_NOTARY_ISSUER_ID}" \
  --keychain "${keychain}"

{
  echo "KEYCHAIN_PATH=${keychain}"
  echo "MACOS_SIGN_IDENTITY=${identity}"
  echo "MACOS_NOTARY_PROFILE_NAME=${profile}"
} >> "${GITHUB_ENV}"

echo "keychain ready: ${identity}"
