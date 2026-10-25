import { describe, expect, it } from 'vitest'
import {
  httpURLError,
  requiredCommandError,
  requiredPathError,
  roleNameDuplicateError,
  roleNameError,
  slugDuplicateError,
  slugError,
  starrAPIKeyError,
} from './validate'

describe('httpURLError', () => {
  it('rejects empty and non-http values', () => {
    expect(httpURLError('')).toBe('URL must begin with http:// or https://')
    expect(httpURLError('baby')).toBe('URL must begin with http:// or https://')
    expect(httpURLError('http://')).toBe('That is not a valid URL.')
  })

  it('accepts an http url with a host', () => {
    expect(httpURLError('https://radarr.example/sonarr')).toBe('')
  })
})

describe('starrAPIKeyError', () => {
  it('accepts a filepath reference and a 32-character key', () => {
    expect(starrAPIKeyError('filepath:/run/secrets/radarr')).toBe('')
    expect(starrAPIKeyError('a'.repeat(32))).toBe('')
  })

  it('rejects a short key', () => {
    expect(starrAPIKeyError('short')).toBe('API key must be at least 32 characters.')
  })
})

describe('required fields and slugs', () => {
  it('requires a path and a command', () => {
    expect(requiredPathError('  ')).toBe('Path must not be empty.')
    expect(requiredPathError('/data')).toBe('')
    expect(requiredCommandError('')).toBe('Command must not be empty.')
  })

  it('checks slug charset and duplicates', () => {
    expect(slugError('tv shows')).toBe(
      'Key must be letters, digits, underscore, or hyphen.',
    )
    expect(slugError('tv_shows')).toBe('')
    expect(slugDuplicateError('main', ['other', 'main'])).toBe(
      'That key is already used.',
    )
    expect(slugDuplicateError('main', ['other'])).toBe('')
  })

  it('reserves admin and rejects a duplicate role', () => {
    expect(roleNameError('')).toBe('')
    expect(roleNameError('admin')).toBe(
      'The built-in admin role cannot be redefined.',
    )
    expect(roleNameError('friends')).toBe('')
    expect(roleNameDuplicateError('friends', ['friends'])).toBe(
      'That role name is already used.',
    )
  })
})
