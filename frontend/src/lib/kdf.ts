// Client-side key derivation, mirroring the Go DeriveKDF:
// PBKDF2-HMAC-SHA-256(password, salt="unpackerr:"+username, 210000, 32 bytes) hex.
//
// This is NOT a substitute for TLS. It keeps a reverse proxy from logging the
// real password and avoids sending the plaintext over the wire. Only the hex
// digest is ever posted to /api/auth/login.
//
// crypto.subtle exists only in a secure context (HTTPS or localhost). HTTP on a
// LAN hostname has no SubtleCrypto, so we fall back to @noble/hashes.

import { pbkdf2Async } from '@noble/hashes/pbkdf2.js'
import { sha256 } from '@noble/hashes/sha2.js'
import { bytesToHex } from '@noble/hashes/utils.js'

const ITERATIONS = 210000
const KEY_LEN_BYTES = 32
const SALT_PREFIX = 'unpackerr:'

function toHex(buffer: ArrayBuffer): string {
  return Array.from(new Uint8Array(buffer))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}

function hasSubtle(): boolean {
  return typeof globalThis.crypto?.subtle?.importKey === 'function'
}

async function deriveSubtle(username: string, password: string): Promise<string> {
  const enc = new TextEncoder()
  const keyMaterial = await crypto.subtle.importKey(
    'raw',
    enc.encode(password),
    'PBKDF2',
    false,
    ['deriveBits'],
  )

  const bits = await crypto.subtle.deriveBits(
    {
      name: 'PBKDF2',
      salt: enc.encode(SALT_PREFIX + username),
      iterations: ITERATIONS,
      hash: 'SHA-256',
    },
    keyMaterial,
    KEY_LEN_BYTES * 8,
  )

  return toHex(bits)
}

async function deriveNoble(username: string, password: string): Promise<string> {
  const key = await pbkdf2Async(sha256, password, SALT_PREFIX + username, {
    c: ITERATIONS,
    dkLen: KEY_LEN_BYTES,
  })

  return bytesToHex(key)
}

/** deriveKDF returns the hex PBKDF2 digest the backend expects. */
export async function deriveKDF(
  username: string,
  password: string,
): Promise<string> {
  if (hasSubtle()) {
    return deriveSubtle(username, password)
  }

  return deriveNoble(username, password)
}
