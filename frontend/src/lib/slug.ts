import { env } from './env.svelte'

/** Map keys: letters, digits, underscore, hyphen. Same charset as webserver roles. */
export const slugPattern = /^[A-Za-z0-9_-]+$/

export type InstanceRow<T> = {
  id: string
  slug: string
  locked: boolean
  autoSlug: boolean
  /** Live/env slug with no file document. Read-only; omitted from PUT. */
  envOnly?: boolean
  value: T
}

export function validSlug(s: string): boolean {
  return s.length > 0 && slugPattern.test(s)
}

export function slugify(raw: string): string {
  return raw
    .trim()
    .toLowerCase()
    .replace(/['"]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .replace(/-{2,}/g, '-')
}

export function slugifyPath(path: string): string {
  const trimmed = path.replace(/[/\\]+$/g, '')
  const parts = trimmed.split(/[/\\]/)
  return slugify(parts.at(-1) ?? '')
}

export function uniqueSlug(
  base: string,
  taken: Iterable<string>,
  fallback = 'instance',
): string {
  const used = new Set(taken)
  const slug = validSlug(base) ? base : fallback
  if (!used.has(slug)) return slug
  let n = 2
  while (used.has(`${slug}-${n}`)) n++
  return `${slug}-${n}`
}

/** Dual-read like the Go API: arrays become keys "0", "1", …. */
export function instanceMap<T>(data: unknown): Record<string, T> {
  if (data == null) return {}
  if (Array.isArray(data)) {
    const out: Record<string, T> = {}
    data.forEach((item, i) => {
      if (item != null) out[String(i)] = item as T
    })
    return out
  }
  if (typeof data === 'object') return { ...(data as Record<string, T>) }
  return {}
}

export function rowsFromMap<T>(map: Record<string, T>): InstanceRow<T>[] {
  return Object.entries(map).map(([slug, value]) => ({
    id: slug,
    slug,
    locked: true,
    autoSlug: false,
    value,
  }))
}

export function newRow<T>(value: T): InstanceRow<T> {
  return {
    id: crypto.randomUUID(),
    slug: '',
    locked: false,
    autoSlug: true,
    value,
  }
}

export function envField(prefix: string, slug: string, field: string): string {
  return `${prefix}_${slug}_${field}`
}

/** True when env sets this field as a whole, not only an index child. */
export function envOwnsExact(pattern?: string): boolean {
  if (!pattern) return false
  const p = pattern.endsWith('_') ? pattern.slice(0, -1) : pattern
  return Object.hasOwn(env.pairs, p) || Object.hasOwn(env.pairs, pattern)
}

export function omitEnvFields<T extends object>(
  prefix: string,
  slug: string,
  row: T,
  fields: Record<string, string>,
): T {
  const out = { ...row } as T & Record<string, unknown>
  for (const [key, suffix] of Object.entries(fields)) {
    if (envOwnsExact(envField(prefix, slug, suffix))) delete out[key]
  }
  return out
}

/** Live slugs missing from the file document: locked stubs, no live values copied. */
export function mergeEnvOnlyRows<T>(
  fileRows: InstanceRow<T>[],
  liveSlugs: Iterable<string>,
  blank: () => T,
): InstanceRow<T>[] {
  const have = new Set(fileRows.map((r) => r.slug).filter(Boolean))
  const extra: InstanceRow<T>[] = []
  for (const slug of liveSlugs) {
    if (!slug || have.has(slug)) continue
    extra.push({
      id: slug,
      slug,
      locked: true,
      autoSlug: false,
      envOnly: true,
      value: blank(),
    })
    have.add(slug)
  }
  return extra.length ? [...fileRows, ...extra] : fileRows
}

export const STARR_ENV_FIELDS: Record<string, string> = {
  name: 'NAME',
  url: 'URL',
  apiKey: 'API_KEY',
  httpPass: 'HTTP_PASS',
  httpUser: 'HTTP_USER',
  username: 'USERNAME',
  password: 'PASSWORD',
  path: 'PATH',
  paths: 'PATHS',
  protocols: 'PROTOCOLS',
  delete_orig: 'DELETE_ORIG',
  delete_delay: 'DELETE_DELAY',
  syncthing: 'SYNCTHING',
  valid_ssl: 'VALID_SSL',
  timeout: 'TIMEOUT',
  maxBytes: 'MAX_BYTES',
  split_flac: 'SPLIT_FLAC',
}

export const FOLDER_ENV_FIELDS: Record<string, string> = {
  path: 'PATH',
  interval: 'INTERVAL',
  extract_path: 'EXTRACT_PATH',
  delete_original: 'DELETE_ORIGINAL',
  delete_files: 'DELETE_FILES',
  disable_log: 'DISABLE_LOG',
  move_back: 'MOVE_BACK',
  delete_after: 'DELETE_AFTER',
  extract_isos: 'EXTRACT_ISOS',
  disableRecursion: 'DISABLE_RECURSION',
  maxNested: 'MAX_NESTED',
  extrasMaxDepth: 'EXTRAS_MAX_DEPTH',
  allowSymlinks: 'ALLOW_SYMLINKS',
  maxBytes: 'MAX_BYTES',
  maxFiles: 'MAX_FILES',
  maxRatio: 'MAX_RATIO',
  exclude_paths: 'EXCLUDE_PATH_',
  wait_extensions: 'WAIT_EXTENSION_',
  skip_empty: 'SKIP_EMPTY',
}

export const HOOK_ENV_FIELDS: Record<string, string> = {
  name: 'NAME',
  url: 'URL',
  command: 'COMMAND',
  contentType: 'CONTENT_TYPE',
  templatePath: 'TEMPLATE_PATH',
  template: 'TEMPLATE',
  timeout: 'TIMEOUT',
  shell: 'SHELL',
  ignoreSsl: 'IGNORE_SSL',
  silent: 'SILENT',
  events: 'EVENTS',
  exclude: 'EXCLUDE',
  nickname: 'NICKNAME',
  token: 'TOKEN',
  channel: 'CHANNEL',
}
