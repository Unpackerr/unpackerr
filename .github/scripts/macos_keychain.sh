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

printf '%s' "${MACOS_SIGN_P12}" | tr -d '\n' | openssl base64 -d -out "${cert}"
printf '%s' "${MACOS_NOTARY_KEY}" | tr -d '\n' | openssl base64 -d -out "${key}"
chmod 600 "${cert}" "${key}"
if [ ! -s "${cert}" ] || [ ! -s "${key}" ]; then
  echo "decoded P12 or notary .p8 is empty (secrets must be base64)" >&2
  exit 1
fi

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
  echo "KEYCHAIN_PASSWORD=${password}"
  echo "MACOS_SIGN_IDENTITY=${identity}"
  echo "MACOS_NOTARY_PROFILE_NAME=${profile}"
} >> "${GITHUB_ENV}"

echo "keychain ready: ${identity}"
