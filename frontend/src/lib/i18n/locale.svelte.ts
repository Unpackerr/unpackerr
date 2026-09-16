import {
  init,
  register,
  locale as lang,
  getLocaleFromNavigator,
} from 'svelte-i18n'
import { failure } from '../toast'
import { readCookie, writeCookie } from '../util'

export const Flags: Record<string, string> = {
  en: '🇺🇸',
  es: '🇪🇸',
  el: '🇬🇷',
  nl: '🇳🇱',
}

export const Locales: { id: string; name: string }[] = [
  { id: 'en', name: 'English' },
  { id: 'es', name: 'Español' },
  { id: 'el', name: 'Ελληνικά' },
  { id: 'nl', name: 'Nederlands' },
]

const cookieName = 'un-lang'
const fallbackLocale = 'en'
const supported = new Set(Object.keys(Flags))

function queryLang(): string {
  return new URLSearchParams(window.location.search).get('lang') || ''
}

function setQuery(name: string, value: string) {
  const url = new URL(window.location.href)
  url.searchParams.set(name, value)
  history.replaceState(null, '', url)
}

class Locale {
  private curr = $state(fallbackLocale)
  public readonly current = $derived(this.curr)
  public readonly list = Locales
  public readonly flags = Flags

  constructor() {
    const raw =
      queryLang() ||
      readCookie(cookieName) ||
      getLocaleFromNavigator() ||
      fallbackLocale
    const initial = raw.split('-')[0]
    this.init(supported.has(initial) ? initial : fallbackLocale, fallbackLocale)
  }

  public readonly set = async (newLocale: string | null) => {
    if (!newLocale) return
    try {
      newLocale = newLocale.split('-')[0]
      await register(
        newLocale,
        async () => await import(`./locales/${newLocale}.json`),
      )

      await lang.set(newLocale)
      this.curr = newLocale
      writeCookie(cookieName, newLocale)
      setQuery('lang', newLocale)
      document.documentElement.lang = newLocale
    } catch (e) {
      this.error(`Error registering selected locale ${newLocale}: ${e}`)
    }
  }

  private init = async (initial: string, fallback: string) => {
    try {
      await register(
        initial,
        async () => await import(`./locales/${initial}.json`),
      )

      if (initial !== fallback) {
        await register(
          fallback,
          async () => await import(`./locales/${fallback}.json`),
        )
      }

      await init({ fallbackLocale: fallback, initialLocale: initial })
      this.curr = initial
      writeCookie(cookieName, initial)
      document.documentElement.lang = initial
    } catch (e) {
      this.error(`Error registering browser locale ${initial}: ${e}`)
      try {
        await register(
          fallback,
          async () => await import(`./locales/${fallback}.json`),
        )
        await init({
          fallbackLocale: fallback,
          initialLocale: (this.curr = fallback),
        })
        document.documentElement.lang = fallback
      } catch (err) {
        this.error(`Error registering default locale ${fallback}: ${err}`)
      }
    }
  }

  private error = (message: string) => {
    console.error(message)
    failure(message)
  }
}

export const locale = new Locale()
