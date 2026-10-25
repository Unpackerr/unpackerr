import { cleanup } from '@testing-library/svelte'
import { afterEach } from 'vitest'
import { addMessages, init, waitLocale } from 'svelte-i18n'
import en from './lib/i18n/locales/en.json'

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }),
})

globalThis.ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as typeof ResizeObserver

globalThis.IntersectionObserver = class {
  readonly root = null
  readonly rootMargin = ''
  readonly thresholds: number[] = []
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords(): IntersectionObserverEntry[] {
    return []
  }
} as unknown as typeof IntersectionObserver

afterEach(() => {
  cleanup()
})

await addMessages('en', en)
await init({ fallbackLocale: 'en', initialLocale: 'en' })
await waitLocale()
