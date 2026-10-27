import { afterEach, describe, expect, it, vi } from 'vitest'
import { env } from './env.svelte'
import {
  instanceMap,
  mapFromPairs,
  mapKeysInvalid,
  mergeEnvOnlyRows,
  newRow,
  omitEnvFields,
  slugify,
  slugifyPath,
  uniqueSlug,
  validSlug,
} from './slug'

describe('slugs', () => {
  afterEach(() => {
    env.pairs = {}
    vi.unstubAllGlobals()
  })

  it('slugifies names and the last path segment', () => {
    expect(validSlug('a_b-1')).toBe(true)
    expect(validSlug('')).toBe(false)
    expect(slugify(`  My "Sonarr"!  `)).toBe('my-sonarr')
    expect(slugifyPath('/mnt/media/TV Shows/')).toBe('tv-shows')
  })

  it('picks a unique slug and keeps env-only rows', () => {
    expect(uniqueSlug('radarr', ['radarr', 'radarr-2'])).toBe('radarr-3')
    expect(uniqueSlug('bad slug', [])).toBe('instance')

    const rows = mergeEnvOnlyRows(
      [{ id: 'file', slug: 'file', locked: true, autoSlug: false, value: { n: 1 } }],
      ['file', 'live'],
      () => ({ n: 0 }),
    )
    expect(rows.map((row) => row.slug)).toEqual(['file', 'live'])
    expect(rows[1]?.envOnly).toBe(true)
    expect(rows[1]?.value).toEqual({ n: 0 })
  })

  it('reads arrays and objects as instance maps', () => {
    expect(instanceMap([{ a: 1 }, null, { a: 2 }])).toEqual({
      '0': { a: 1 },
      '2': { a: 2 },
    })
    expect(instanceMap(null)).toEqual({})
  })

  it('drops env-owned fields and rejects bad map keys', () => {
    env.pairs = { FOLDER_tv_PATH: '/data' }
    const row = { path: '/data', interval: '1m' }
    expect(omitEnvFields('FOLDER', 'tv', row, { path: 'PATH', interval: 'INTERVAL' })).toEqual({
      interval: '1m',
    })
    expect(row).toEqual({ path: '/data', interval: '1m' })

    expect(mapFromPairs([{ key: ' a ', value: '1' }, { key: '', value: '2' }])).toEqual({
      a: '1',
    })
    expect(mapKeysInvalid({ Ab: '1', ab: '2' })).toBe(true)
    expect(mapKeysInvalid({ ok: '1' })).toBe(false)
  })

  // Insecure contexts (http://<lan-ip>:5656) have no randomUUID or subtle;
  // getRandomValues is the only usable Crypto member. Issue #831 item 1:
  // clicking Add threw "crypto.randomUUID is not a function".
  it('newRow generates a v4 UUID without randomUUID', () => {
    const real = globalThis.crypto
    vi.stubGlobal('crypto', {
      getRandomValues: real.getRandomValues.bind(real),
    })
    expect(globalThis.crypto.randomUUID).toBeUndefined()

    const row = newRow({})
    const other = newRow({})
    const v4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
    expect(row.id).toMatch(v4)
    expect(other.id).toMatch(v4)
    expect(row.id).not.toBe(other.id)
  })

  it('newRow uses the native randomUUID when available', () => {
    const row = newRow({})
    const other = newRow({})
    const v4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
    expect(row.id).toMatch(v4)
    expect(other.id).toMatch(v4)
    expect(row.id).not.toBe(other.id)
  })
})
