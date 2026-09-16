export function readCookie(name: string): string {
  const match = document.cookie.match(
    new RegExp('(?:^|; )' + name + '=([^;]*)'),
  )
  return match ? decodeURIComponent(match[1]) : ''
}

export function writeCookie(name: string, value: string) {
  document.cookie =
    encodeURIComponent(name) +
    '=' +
    encodeURIComponent(value) +
    '; Path=/; SameSite=Lax; Max-Age=31536000'
}

/** Check if two values are equal (objects, arrays, primitives). */
export const deepEqual = (obj1: unknown, obj2: unknown): boolean => {
  if (
    typeof obj1 !== 'object' ||
    typeof obj2 !== 'object' ||
    obj1 === null ||
    obj2 === null
  ) {
    return obj1 === obj2
  }

  const a = obj1 as Record<string, unknown>
  const b = obj2 as Record<string, unknown>
  const keys1 = Object.keys(a)
  const keys2 = Object.keys(b)
  if (keys1.length !== keys2.length) return false

  for (const key of keys1) {
    if (
      !Object.prototype.hasOwnProperty.call(b, key) ||
      !deepEqual(a[key], b[key])
    ) {
      return false
    }
  }

  return true
}

/** Deep copy. Fine for our reasonably simple config payloads. */
export const deepCopy = <T>(obj: T): T => {
  if (typeof obj !== 'object' || obj === null) return obj
  if (Array.isArray(obj)) return obj.map((item) => deepCopy(item)) as T

  const out: Record<string, unknown> = {}
  for (const key of Object.keys(obj as object)) {
    out[key] = deepCopy((obj as Record<string, unknown>)[key])
  }

  return out as T
}

/** Inline help (`code` only). Used for the FormText description under a field. */
export function helpToInline(text: string): string {
  if (!text) return ''

  const s = text.trim()
  if (/<[a-z][\s\S]*>/i.test(s)) return s

  return escapeHtml(s).replace(/`([^`]+)`/g, '<code>$1</code>')
}

/** Turn YAML-wrapped help (and `code`) into HTML paragraphs. */
export function helpToHtml(text: string): string {
  if (!text) return ''

  return text
    .trim()
    .split(/\n\s*\n/)
    .map((p) => p.replace(/[ \t]*\n[ \t]*/g, ' ').trim())
    .filter(Boolean)
    .map((p) => {
      const html = /<[a-z][\s\S]*>/i.test(p)
        ? p
        : escapeHtml(p).replace(/`([^`]+)`/g, '<code>$1</code>')
      return `<p class="mt-2 mb-0">${html}</p>`
    })
    .join('')
}

/** File 0s inherits the global timeout (10s). Show and save 10s so the file stops inheriting. */
export function explicitTimeout(raw: string | undefined): string {
  return explicitDuration(raw, '10s')
}

/** File 0s inherits the global delete delay (5m). Show and save 5m so the file stops inheriting. */
export function explicitDeleteDelay(raw: string | undefined): string {
  return explicitDuration(raw, '5m')
}

function explicitDuration(raw: string | undefined, fallback: string): string {
  const s = (raw ?? '').trim()
  if (!s || s === '0' || s === '0s') return fallback
  return s
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}
