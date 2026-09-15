import { get } from 'svelte/store'
import { _ } from 'svelte-i18n'
import { api, setApiKey, setUnauthorizedHandler } from './api'
import { deriveKDF } from './kdf'
import { info } from './toast'
import type { AuthInfo } from './types'
import { loadHelp } from './help.svelte'
import { loadEnv } from './env.svelte'
import { clearRestart } from './restart.svelte'
import { live } from './socket.svelte'

// profile is the reactive auth state shared across the app.
export const profile = $state({
  loaded: false, // becomes true after the first /api/auth/me attempt
  authed: false,
  info: null as AuthInfo | null,
})

function apply(info: AuthInfo) {
  profile.info = info
  profile.authed = true
  setApiKey(info.apiKey || '')
  live.connect()
  void loadHelp()
  void loadEnv()
}

function clear() {
  live.disconnect()
  profile.info = null
  profile.authed = false
  setApiKey('')
  clearRestart()
}

let bouncing = false

function sessionEndedMessage(): string {
  try {
    const text = get(_)('phrases.SessionEnded')
    if (text && text !== 'phrases.SessionEnded') return text
  } catch {
    // i18n may not be ready yet
  }
  return 'Session ended. Log in again.'
}

setUnauthorizedHandler(() => {
  if (!profile.authed || bouncing) return
  bouncing = true
  clear()
  info(sessionEndedMessage())
  bouncing = false
})

/** restore re-fetches identity via the session cookie or an existing key. */
export async function restore(): Promise<void> {
  const res = await api.get<AuthInfo>('auth/me')
  if (res.ok && res.body?.permissions) apply(res.body)
  else clear()
  profile.loaded = true
}

/** login derives the KDF hex in the browser and posts only { name, kdf }. */
export async function login(
  username: string,
  password: string,
): Promise<string> {
  const name = username.trim() || 'admin'
  let kdf: string
  try {
    kdf = await deriveKDF(name, password)
  } catch (error) {
    return 'could not hash password: ' + (error as Error).message
  }

  const res = await api.post<AuthInfo>('auth/login', {
    name: username.trim(),
    kdf,
  })
  if (!res.ok) return (res.body as { error?: string })?.error || 'login failed'

  apply(res.body)
  return ''
}

export async function logout(): Promise<void> {
  await api.post('auth/logout', {})
  clear()
}

/** has returns true when the current identity holds a permission (admin => all). */
export function has(perm: string): boolean {
  const perms = profile.info?.permissions ?? []
  return perms.includes('*') || perms.includes(perm)
}
