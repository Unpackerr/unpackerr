import { readCookie, writeCookie } from './util'

export type ThemeMode = 'light' | 'dark'

const cookieName = 'un-theme'

function prefersDark(): boolean {
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

function parseMode(raw: string): ThemeMode {
  if (raw === 'light' || raw === 'dark') return raw
  return prefersDark() ? 'dark' : 'light'
}

class Theme {
  mode = $state<ThemeMode>('light')
  resolved = $state<'light' | 'dark'>('light')

  constructor() {
    this.mode = parseMode(readCookie(cookieName))
    this.apply()
    writeCookie(cookieName, this.mode)
  }

  set(mode: ThemeMode) {
    this.mode = mode
    writeCookie(cookieName, mode)
    this.apply()
  }

  toggle() {
    this.set(this.mode === 'dark' ? 'light' : 'dark')
  }

  private apply() {
    this.resolved = this.mode
    document.documentElement.setAttribute('data-bs-theme', this.mode)
    document.documentElement.setAttribute('data-un-theme', this.mode)
  }
}

export const theme = new Theme()
