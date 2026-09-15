// Minimal hash router. Routes look like #/history, #/trust, or #/settings/sonarr.

import { dirty, wouldLoseChanges } from './dirty.svelte'

export const router = $state({ path: parse() })

/** Skip the hashchange guard while navigate() itself is writing the hash. */
let passing = false

function parse(): string {
  const hash = window.location.hash.replace(/^#/, '') || '/'
  return alias(hash)
}

function alias(path: string): string {
  if (path === '/settings/auth') return '/trust'
  if (path === '/settings/whisparr') return '/settings/radarr'
  if (path === '/history' || path === '/dashboard') return '/'
  return path
}

function keepHash(path: string): string {
  return path === '/' ? '#/' : '#' + path
}

function syncHash(path: string) {
  const hash = window.location.hash.replace(/^#/, '') || '/'
  if (hash !== path) history.replaceState(null, '', keepHash(path))
}

export function startRouter() {
  const first = parse()
  syncHash(first)
  router.path = first

  window.addEventListener('hashchange', () => {
    const next = parse()
    if (!passing && wouldLoseChanges(next) && next !== router.path) {
      dirty.pending = next
      history.replaceState(null, '', keepHash(router.path))
      return
    }

    dirty.pending = null
    syncHash(next)
    router.path = next
  })
}

function apply(next: string) {
  dirty.pending = null
  passing = true
  try {
    if ((window.location.hash.replace(/^#/, '') || '/') === next) {
      router.path = next
      return
    }

    window.location.hash = next
    router.path = next
  } finally {
    passing = false
  }
}

export function navigate(path: string, force = false) {
  const next = alias(path.startsWith('/') ? path : '/' + path)
  if (!force && wouldLoseChanges(next) && next !== router.path) {
    dirty.pending = next
    return
  }

  apply(next)
}

export function discardAndGo() {
  const to = dirty.pending
  dirty.pending = null

  if (to != null) navigate(to, true)
}

export function stayHere() {
  dirty.pending = null
}

/** Intercept in-app hash links so unsaved changes can be confirmed first. */
export function hashLinkClick(e: MouseEvent, path: string) {
  if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return
  e.preventDefault()
  navigate(path)
}

/** segments returns the path split on '/', dropping empties. */
export function segments(): string[] {
  return router.path.split('/').filter(Boolean)
}
