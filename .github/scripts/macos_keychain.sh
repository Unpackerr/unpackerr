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
# one-line base64. macOS openssl base64 -d without -A yields empty files
# for long lines, and PEM is not base64 at all.
write_secret() {
  local dest=$1 envname=$2
  python3 - "${dest}" "${envname}" <<'PY'
import base64, os, pathlib, sys

dest, envname = sys.argv[1], sys.argv[2]
raw = os.environ.get(envname, "")
if not raw.strip():
    sys.stderr.write(f"{envname} empty\n")
    sys.exit(1)
s = raw.strip().replace("\r", "")
if "BEGIN " in s:
    data = (s if s.endswith("\n") else s + "\n").encode()
else:
    compact = "".join(s.split())
    compact += "=" * ((4 - len(compact) % 4) % 4)
    try:
        data = base64.b64decode(compact)
    except Exception as exc:
        sys.stderr.write(f"{envname} base64 decode failed: {exc}\n")
        sys.exit(1)
if not data:
    sys.stderr.write(f"{envname} decoded to empty ({len(raw)} input chars)\n")
    sys.exit(1)
pathlib.Path(dest).write_bytes(data)
print(f"{envname}: {len(raw)} chars -> {len(data)} bytes")
PY
}

write_secret "${cert}" MACOS_SIGN_P12
write_secret "${key}" MACOS_NOTARY_KEY
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
