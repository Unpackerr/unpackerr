import { webcrypto } from 'node:crypto'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { deriveKDF } from './kdf'

// PBKDF2-HMAC-SHA-256(password, salt="unpackerr:"+username, 210000, 32) hex.
// Same vector as DeriveKDF("admin", "correct-horse") in the Go server.
const golden = 'fbfffdac847f92f87916a82393819002e03631b4492e5cb045ff59dd259d96d9'

describe('deriveKDF', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('matches the Go digest with SubtleCrypto', async () => {
    // jsdom's crypto has no subtle, so the default path is the Noble fallback.
    vi.stubGlobal('crypto', webcrypto)
    const deriveBits = vi.spyOn(webcrypto.subtle, 'deriveBits')

    await expect(deriveKDF('admin', 'correct-horse')).resolves.toBe(golden)
    expect(deriveBits).toHaveBeenCalled()
  })

  it('matches the Go digest with the @noble/hashes fallback', async () => {
    const real = globalThis.crypto
    vi.stubGlobal('crypto', {
      getRandomValues: real.getRandomValues.bind(real),
      randomUUID: real.randomUUID.bind(real),
    })
    expect(globalThis.crypto.subtle).toBeUndefined()

    await expect(deriveKDF('admin', 'correct-horse')).resolves.toBe(golden)
  })

  it('changes the digest when the username changes', async () => {
    const admin = await deriveKDF('admin', 'correct-horse')
    const other = await deriveKDF('other', 'correct-horse')
    expect(other).not.toBe(admin)
  })
})
