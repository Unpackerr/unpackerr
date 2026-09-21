export const HOOK_TEMPLATE_NAMES = [
  'notifiarr',
  'discord',
  'gotify',
  'pushover',
  'slack',
  'telegram',
  'ntfy',
  'apprise',
  'mattermost',
] as const

export type HookTemplateName = (typeof HOOK_TEMPLATE_NAMES)[number]

export type DetectedHookTemplate = HookTemplateName | 'custom'

export function hookTemplateFileName(name: string): string {
  return `unpackerr-webhook-${name}.tmpl`
}

export function defaultHookTemplate(
  name: string | undefined,
): HookTemplateName {
  const detected = detectHookTemplate(name, '', '')
  if (detected === 'custom') return 'notifiarr'
  return detected
}

export function detectHookTemplate(
  name: string | undefined,
  url: string | undefined,
  tmplPath: string | undefined,
): DetectedHookTemplate {
  const n = (name ?? '').trim().toLowerCase()
  if (n === 'default') return 'notifiarr'
  if ((HOOK_TEMPLATE_NAMES as readonly string[]).includes(n)) {
    return n as HookTemplateName
  }
  if ((tmplPath ?? '').trim() !== '') return 'custom'
  return sniffHookURL(url ?? '')
}

function sniffHookURL(raw: string): HookTemplateName {
  const lower = raw.toLowerCase()
  let host = ''
  let path = ''
  try {
    const parsed = new URL(raw)
    host = parsed.hostname.toLowerCase()
    path = parsed.pathname.toLowerCase()
  } catch {
    // substring checks still run on the raw URL
  }

  if (lower.includes('discordnotifier.com') || lower.includes('notifiarr.com')) {
    return 'notifiarr'
  }
  if (lower.includes('discord.com') || lower.includes('discordapp.com')) {
    return 'discord'
  }
  if (lower.includes('api.telegram.org')) return 'telegram'
  if (lower.includes('hooks.slack.com')) return 'slack'
  if (lower.includes('pushover.net')) return 'pushover'
  if (lower.includes('gotify')) return 'gotify'
  if (host === 'ntfy.sh' || host.includes('ntfy')) return 'ntfy'
  if (host.includes('apprise') || path.includes('/notify')) return 'apprise'
  if (path.includes('/hooks/')) return 'mattermost'
  return 'notifiarr'
}

export type HookFormField =
  | 'nickname'
  | 'channel'
  | 'token'
  | 'contentType'
  | 'update'

export type HookFormSource = 'named' | 'url' | 'file' | 'default'

export type HookFormProfile = {
  template: DetectedHookTemplate
  source: HookFormSource
  canUpdate: boolean
  fields: readonly HookFormField[]
}

const PROFILE_FIELDS: Record<DetectedHookTemplate, readonly HookFormField[]> = {
  notifiarr: [],
  discord: ['nickname', 'update'],
  telegram: ['nickname', 'update'],
  slack: ['nickname', 'channel'],
  pushover: ['token', 'channel', 'nickname'],
  gotify: ['nickname'],
  ntfy: ['token', 'nickname'],
  apprise: ['nickname'],
  mattermost: ['nickname', 'channel'],
  custom: ['nickname', 'channel', 'token', 'contentType'],
}

// hookFormProfile mirrors Go Detect() and lists the extras the settings form shows.
export function hookFormProfile(
  template: string | undefined,
  url: string | undefined,
  tmplPath: string | undefined,
): HookFormProfile {
  const named = (template ?? '').trim().toLowerCase()
  const detected = detectHookTemplate(named || undefined, url, tmplPath)
  const namedBuiltin =
    named === 'default' ||
    (HOOK_TEMPLATE_NAMES as readonly string[]).includes(named)
  let source: HookFormSource
  if (namedBuiltin) source = 'named'
  else if ((tmplPath ?? '').trim() !== '') source = 'file'
  else if (detected !== 'notifiarr' && (url ?? '').trim() !== '') source = 'url'
  else source = 'default'

  return {
    template: detected,
    source,
    canUpdate: detected === 'discord' || detected === 'telegram',
    fields: PROFILE_FIELDS[detected],
  }
}

export function hookShowsField(
  profile: HookFormProfile,
  field: HookFormField,
): boolean {
  return profile.fields.includes(field)
}
