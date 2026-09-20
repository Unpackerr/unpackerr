import type { ConfigSection } from './types'

const SECTIONS: ConfigSection[] = [
  'general',
  'webserver',
  'sonarr',
  'radarr',
  'lidarr',
  'readarr',
  'folders',
  'hooks',
  'webhooks',
  'cmdhooks',
]

export type PermVerb = 'read' | 'write'

/** Built-in role that grants every permission. Cannot be redefined. */
export const RoleAdmin = 'admin'

/** systemPerm is area:resource:verb, e.g. system:info:read. */
export function systemPerm(resource: string, verb: PermVerb): string {
  return `system:${resource}:${verb}`
}

/** configPerm is area:resource:verb, e.g. config:lidarr:write. */
export function configPerm(section: string, verb: PermVerb): string {
  return `config:${section}:${verb}`
}

// Every assignable permission, mirroring AllPermissions() minus the reserved '*'.
export const ALL_PERMISSIONS: string[] = [
  systemPerm('stats', 'read'),
  systemPerm('info', 'read'),
  systemPerm('queue', 'read'),
  systemPerm('queue', 'write'),
  systemPerm('history', 'read'),
  systemPerm('history', 'write'),
  systemPerm('metrics', 'read'),
  systemPerm('headers', 'read'),
  systemPerm('browse', 'read'),
  systemPerm('browse', 'write'),
  systemPerm('logs', 'read'),
  ...SECTIONS.flatMap((s) => [configPerm(s, 'read'), configPerm(s, 'write')]),
]

/** newApiKey returns a random 64-char URL-safe key (48 random bytes, base64url). */
export function newApiKey(): string {
  const raw = new Uint8Array(48)
  crypto.getRandomValues(raw)
  let bin = ''
  for (const b of raw) bin += String.fromCharCode(b)

  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}
