import { get } from 'svelte/store'
import { _ } from 'svelte-i18n'
import { RoleAdmin } from './perms'
import { validSlug } from './slug'

export const STARR_API_KEY_MIN = 32

function t(key: string, values?: Record<string, string | number>): string {
  try {
    const text = values ? get(_)(key, { values }) : get(_)(key)
    if (text && text !== key) return text
  } catch {
    // i18n may not be ready
  }

  return key
}

/** URL must be http(s) with a hostname. Rejects junk like "baby". */
export function httpURLError(value: unknown): string {
  if (typeof value !== 'string' || !value.trim()) {
    return t('phrases.URLMustBeginWithHttp')
  }

  const v = value.trim()
  if (!v.startsWith('http://') && !v.startsWith('https://')) {
    return t('phrases.URLMustBeginWithHttp')
  }

  try {
    const u = new URL(v)
    if (!u.hostname) return t('phrases.URLInvalid')
  } catch {
    return t('phrases.URLInvalid')
  }

  return ''
}

/** Starr API keys are 32+ characters. Unchanged filepath: or already-valid keys pass. */
export function starrAPIKeyError(value: unknown, original?: string): string {
  const v = typeof value === 'string' ? value : ''
  if (
    original !== undefined &&
    v === original &&
    (v.startsWith('filepath:') || v.length >= STARR_API_KEY_MIN)
  ) {
    return ''
  }

  if (v.length < STARR_API_KEY_MIN) {
    return t('phrases.APIKeyMustBeCountCharactersOrLonger', {
      count: STARR_API_KEY_MIN,
    })
  }

  return ''
}

export function requiredPathError(value: unknown): string {
  if (typeof value !== 'string' || !value.trim())
    return t('phrases.PathMustNotBeEmpty')
  return ''
}

export function requiredCommandError(value: unknown): string {
  if (typeof value !== 'string' || !value.trim())
    return t('phrases.CommandMustNotBeEmpty')
  return ''
}

/** Instance map key: letters, digits, underscore, or hyphen. */
export function slugError(value: unknown): string {
  const v = typeof value === 'string' ? value.trim() : ''
  if (!validSlug(v)) return t('phrases.SlugInvalid')
  return ''
}

export function slugDuplicateError(slug: string, others: Iterable<string>): string {
  for (const other of others) {
    if (other === slug) return t('phrases.SlugDuplicate')
  }
  return ''
}

/** Custom webserver role name: same charset as slugs; `admin` is reserved. Empty is ok (dropped on save). */
export function roleNameError(value: unknown): string {
  const v = typeof value === 'string' ? value.trim() : ''
  if (!v) return ''
  if (v === RoleAdmin) return t('phrases.RoleNameReserved')
  return slugError(v)
}

export function roleNameDuplicateError(
  name: string,
  others: Iterable<string>,
): string {
  const n = name.trim()
  if (!n) return ''
  for (const other of others) {
    if (other.trim() === n) return t('phrases.RoleNameDuplicate')
  }
  return ''
}
