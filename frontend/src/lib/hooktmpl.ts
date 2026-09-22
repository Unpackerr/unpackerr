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

  if (lower.includes('notifiarr.com')) return 'notifiarr'
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

export const NOTIFIARR_API_KEY_HEADER = 'X-Api-Key'

const NOTIFIARR_UNPACKERR_PATH = '/api/v1/notification/unpackerr'
const NOTIFIARR_API_KEY_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

function hostIsNotifiarr(host: string): boolean {
  const h = host.toLowerCase()
  return h === 'notifiarr.com' || h.endsWith('.notifiarr.com')
}

/** True for notifiarr.com webhook URLs. */
export function isNotifiarrHost(raw: string): boolean {
  try {
    return hostIsNotifiarr(new URL(raw.trim()).hostname)
  } catch {
    return false
  }
}

export function headerValue(
  headers: Record<string, string> | null | undefined,
  name: string,
): string {
  const want = name.toLowerCase()
  for (const [key, value] of Object.entries(headers ?? {})) {
    if (key.toLowerCase() === want) return value
  }
  return ''
}

export function omitHeader(
  headers: Record<string, string> | null | undefined,
  name: string,
): Record<string, string> {
  const want = name.toLowerCase()
  const out: Record<string, string> = {}
  for (const [key, value] of Object.entries(headers ?? {})) {
    if (key.toLowerCase() !== want) out[key] = value
  }
  return out
}

export function setHeader(
  headers: Record<string, string> | null | undefined,
  name: string,
  value: string,
): Record<string, string> {
  const out = omitHeader(headers, name)
  out[name] = value
  return out
}

/** Strip a Notifiarr path API key; return the canonical URL and the key. */
export function rewriteNotifiarrURL(raw: string): {
  url: string
  apiKey: string
} {
  const trimmed = (raw ?? '').trim()
  try {
    const parsed = new URL(trimmed)
    if (!hostIsNotifiarr(parsed.hostname)) return { url: raw, apiKey: '' }

    const path = parsed.pathname.replace(/\/+$/, '')
    const lower = path.toLowerCase()
    if (lower === NOTIFIARR_UNPACKERR_PATH) return { url: raw, apiKey: '' }
    if (!lower.startsWith(NOTIFIARR_UNPACKERR_PATH + '/')) {
      return { url: raw, apiKey: '' }
    }

    const rest = path.slice(NOTIFIARR_UNPACKERR_PATH.length + 1)
    if (!rest || rest.includes('/') || !NOTIFIARR_API_KEY_RE.test(rest)) {
      return { url: raw, apiKey: '' }
    }

    parsed.pathname = NOTIFIARR_UNPACKERR_PATH
    return { url: parsed.toString(), apiKey: rest }
  } catch {
    return { url: raw, apiKey: '' }
  }
}

export function applyNotifiarrURL(
  raw: string,
  headers?: Record<string, string> | null,
): { url: string; headers: Record<string, string> } {
  const { url, apiKey } = rewriteNotifiarrURL(raw)
  let next = { ...(headers ?? {}) }
  // Match Go: a header already set wins over the key embedded in the path.
  if (apiKey && headerValue(next, NOTIFIARR_API_KEY_HEADER).trim() === '') {
    next = setHeader(next, NOTIFIARR_API_KEY_HEADER, apiKey)
  }
  return { url, headers: next }
}

export function hookShowsApiKey(url?: string): boolean {
  return isNotifiarrHost(url ?? '')
}

/** Extra headers when a named template or custom file is set, or headers already exist. */
export function hookShowsHeaders(
  profile: HookFormProfile,
  headers?: Record<string, string> | null,
  url?: string,
): boolean {
  const extra = hookShowsApiKey(url)
    ? omitHeader(headers, NOTIFIARR_API_KEY_HEADER)
    : (headers ?? {})
  if (Object.keys(extra).length > 0) return true
  return profile.source === 'named' || profile.source === 'file'
}
