import { api } from './api'

/** UN_* suffixes that cnfg wrote at startup (DEBUG, SONARR_0_URL). */
export const env = $state({
  pairs: {} as Record<string, string>,
})

export async function loadEnv() {
  const res = await api.get<Record<string, string>>('config/env')
  if (res.ok && res.body) env.pairs = res.body
}

/** True when this field's envVar (suffix after UN_) is in GET /api/config/env. */
export function envHas(pattern?: string): boolean {
  return envMatchKeys(pattern).length > 0
}

/** Joined env values for matching keys; omitted when redacted or unset. */
export function envLiveValue(pattern?: string): string | undefined {
  // Login secret is never shown; GET /api/config/env blanks it even for *.
  if ((pattern ?? '').toUpperCase().includes('UI_PASSWORD')) return undefined

  const keys = envMatchKeys(pattern)
  if (!keys.length) return undefined

  const vals = keys.map((k) => env.pairs[k]).filter((v) => v !== '')
  if (!vals.length) return undefined

  return vals.join('\n')
}

function envMatchKeys(pattern?: string): string[] {
  if (!pattern) return []

  // Keep slug case (SONARR_uhd_URL). Do not uppercase.
  const p = pattern
  const keys = Object.keys(env.pairs)

  if (p.includes('*') || p.includes('#')) {
    const re = globRe(p)
    return keys.filter((k) => re.test(k))
  }

  const base = p.endsWith('_') ? p.slice(0, -1) : p
  return keys.filter((k) => k === base || k === p || isIndexChild(k, base))
}

function isIndexChild(key: string, base: string): boolean {
  if (!key.startsWith(base + '_')) return false
  return /^\d+/.test(key.slice(base.length + 1))
}

function globRe(pattern: string): RegExp {
  let src = '^'

  for (const ch of pattern) {
    if (ch === '*') src += '[^_]+'
    else if (ch === '#') src += '\\d+'
    else src += ch.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  }

  src += '(?:_.*)?$'
  return new RegExp(src)
}
