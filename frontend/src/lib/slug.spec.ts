import { afterEach, describe, expect, it } from 'vitest'
import { env } from './env.svelte'
import {
  instanceMap,
  mapFromPairs,
  mapKeysInvalid,
  mergeEnvOnlyRows,
  omitEnvFields,
  slugify,
  slugifyPath,
  uniqueSlug,
  validSlug,
} from './slug'

describe('slugs', () => {
  afterEach(() => {
    env.pairs = {}
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
})
