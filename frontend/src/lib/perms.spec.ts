import { describe, expect, it } from 'vitest'
import { ALL_PERMISSIONS, RoleAdmin, configPerm, newApiKey, systemPerm } from './perms'

describe('permissions', () => {
  it('builds area:resource:verb ids and lists each one once', () => {
    expect(RoleAdmin).toBe('admin')
    expect(systemPerm('queue', 'write')).toBe('system:queue:write')
    expect(configPerm('sonarr', 'read')).toBe('config:sonarr:read')
    expect(ALL_PERMISSIONS).toContain('config:folders:write')
    expect(new Set(ALL_PERMISSIONS).size).toBe(ALL_PERMISSIONS.length)
    expect(ALL_PERMISSIONS).not.toContain('*')
  })

  it('returns a 64-character url-safe key', () => {
    const key = newApiKey()
    expect(key).toMatch(/^[A-Za-z0-9_-]{64}$/)
    expect(newApiKey()).not.toBe(key)
  })
})
