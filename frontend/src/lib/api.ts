// Thin fetch wrapper for the Unpackerr API. The SPA authenticates with the
// session cookie (credentials: same-origin). The admin API key from login is
// kept only for Rapidoc "try it"; sending it on every fetch would keep the UI
// logged in after a restart because that key is persisted in the config file.

export const LoggedOut = new Error('logged out')
export const TimedOut = new Error('request timed out')

// urlbase lets a reverse-proxied SPA know its prefix. The backend drops a cookie;
// in dev we default to '/'.
function readCookie(name: string): string {
  const match = document.cookie.match(
    new RegExp('(?:^|; )' + name + '=([^;]*)'),
  )
  return match ? decodeURIComponent(match[1]) : ''
}

let urlbase = readCookie('urlbase') || '/'
let apiKey = ''
let onUnauthorized: (() => void) | undefined

export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

export function setApiKey(key: string) {
  apiKey = key
}

export function getApiKey(): string {
  return apiKey
}

export function setUrlbase(base: string) {
  urlbase = base || '/'
}

export function getUrlbase(): string {
  return urlbase
}

function rtrim(s: string, c: string): string {
  while (s.endsWith(c)) s = s.slice(0, -c.length)
  return s
}

function ltrim(s: string, c: string): string {
  while (s.startsWith(c)) s = s.slice(c.length)
  return s
}

export interface BackendResponse<T = any> {
  ok: boolean
  status: number
  body: T
}

async function fetchWithTimeout(
  url: string,
  options: RequestInit,
  timeout: number,
): Promise<Response> {
  const controller = new AbortController()
  const id = setTimeout(() => controller.abort(), timeout)
  options.signal = controller.signal
  try {
    return await fetch(url, options)
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError')
      throw TimedOut
    throw error
  } finally {
    clearTimeout(id)
  }
}

async function request<T = any>(
  uri: string,
  method = 'GET',
  body: unknown = null,
  timeout = 10000,
): Promise<BackendResponse<T>> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (body !== null) headers['Content-Type'] = 'application/json'

  const full = rtrim(urlbase, '/') + '/' + ltrim(uri, '/')

  try {
    const response = await fetchWithTimeout(
      full,
      {
        method,
        headers,
        credentials: 'same-origin',
        body: body === null ? undefined : JSON.stringify(body),
      },
      timeout,
    )

    let payload: any = null
    const text = await response.text()
    if (text) {
      try {
        payload = JSON.parse(text)
      } catch {
        payload = text
      }
    }

    if (response.status === 401) {
      // Failed login is an expected 401; do not bounce the session.
      if (!uri.includes('auth/login')) onUnauthorized?.()
      const msg = payload?.error || 'unauthorized'

      return { ok: false, status: 401, body: { error: msg } as T }
    }

    if (!response.ok) {
      const msg =
        payload?.error || response.statusText || `HTTP ${response.status}`
      return { ok: false, status: response.status, body: { error: msg } as T }
    }

    return { ok: true, status: response.status, body: payload as T }
  } catch (error) {
    // Timeouts and network errors stay as status:0. Do not log the user out;
    // the daemon may be restarting.
    const msg =
      error === TimedOut ? 'request timed out' : (error as Error).message
    return { ok: false, status: 0, body: { error: msg } as T }
  }
}

export const api = {
  get: <T = any>(uri: string) => request<T>('api/' + uri, 'GET'),
  post: <T = any>(uri: string, body: unknown = {}, timeout = 10000) =>
    request<T>('api/' + uri, 'POST', body, timeout),
  put: <T = any>(uri: string, body: unknown) =>
    request<T>('api/' + uri, 'PUT', body),
  del: <T = any>(uri: string) => request<T>('api/' + uri, 'DELETE'),
}
