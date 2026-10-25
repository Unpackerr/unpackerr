import { describe, expect, it } from 'vitest'
import {
  defaultAuthHeader,
  headerPickerOptions,
  parseStoredPassword,
  reservedUsername,
} from './ui-auth'

describe('parseStoredPassword', () => {
  it('reads noauth, header, and password forms', () => {
    expect(parseStoredPassword('noauth')).toEqual({
      mode: 'noauth',
      username: 'admin',
      header: defaultAuthHeader,
    })
    expect(parseStoredPassword('webauth:X-Remote-User')).toEqual({
      mode: 'header',
      username: 'admin',
      header: 'X-Remote-User',
    })
    expect(parseStoredPassword('!!cryptd!!dave:abc')).toEqual({
      mode: 'password',
      username: 'dave',
      header: defaultAuthHeader,
    })
    expect(parseStoredPassword('!!cryptd!!$2a$10$hash')).toEqual({
      mode: 'password',
      username: 'admin',
      header: defaultAuthHeader,
    })
  })
})

describe('header auth helpers', () => {
  it('rejects reserved usernames', () => {
    expect(reservedUsername('webauth')).toBe(true)
    expect(reservedUsername('dave:extra')).toBe(true)
    expect(reservedUsername('dave')).toBe(false)
  })

  it('labels request headers with a sample value', () => {
    expect(
      headerPickerOptions({ 'X-Remote-User': ['ada'], Empty: [''], Missing: null }),
    ).toEqual([
      { value: 'X-Remote-User', name: 'X-Remote-User (ada)' },
      { value: 'Empty', name: 'Empty' },
      { value: 'Missing', name: 'Missing' },
    ])
  })
})
