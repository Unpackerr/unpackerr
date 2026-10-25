import { render, screen, waitFor } from '@testing-library/svelte'
import { describe, expect, it, vi } from 'vitest'
import { setApiKey, setUrlbase } from '../lib/api'
import ApiDocs from './ApiDocs.svelte'

vi.mock('rapidoc', () => {
  if (!customElements.get('rapi-doc')) {
    customElements.define(
      'rapi-doc',
      class extends HTMLElement {
        setApiKey() {}
      },
    )
  }

  return {}
})

describe('ApiDocs', () => {
  it('points rapidoc at this server OpenAPI spec and injects the API key', async () => {
    setUrlbase('/')
    setApiKey('secret-key')
    render(ApiDocs)

    expect(screen.getByRole('heading', { name: 'API documentation' })).toBeTruthy()

    await waitFor(() => {
      expect(document.querySelector('rapi-doc')).toBeTruthy()
    })

    const doc = document.querySelector('rapi-doc') as HTMLElement & {
      setApiKey?: (id: string, value: string) => void
    }
    expect(doc.getAttribute('spec-url')).toBe('/api/openapi.json')
    expect(doc.getAttribute('render-style')).toBe('read')
    expect(doc.getAttribute('allow-authentication')).toBe('true')

    const setKey = vi.fn()
    doc.setApiKey = setKey
    doc.dispatchEvent(new Event('spec-loaded'))
    expect(setKey).toHaveBeenCalledWith('apiKey', 'secret-key')
  })
})
