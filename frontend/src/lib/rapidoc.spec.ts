import { describe, expect, it } from 'vitest'

describe('rapidoc', () => {
  it('registers a rapi-doc element with setApiKey', async () => {
    await import('rapidoc')

    const ctor = customElements.get('rapi-doc')
    expect(typeof ctor).toBe('function')
    if (!ctor) return

    // Do not connect the element. jsdom has no FontFace, which rapidoc uses on connect.
    const el = new ctor() as HTMLElement & {
      setApiKey?: (id: string, value: string) => void
    }
    expect(typeof el.setApiKey).toBe('function')
  })
})
