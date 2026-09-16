<script lang="ts">
  import { onMount } from 'svelte'
  import { getUrlbase, getApiKey } from '../lib/api'
  import { _ } from '../lib/i18n/Translate.svelte'
  import PageIntro from '../components/PageIntro.svelte'
  import { theme } from '../lib/theme.svelte'

  let ready = $state(false)

  const specUrl = () => {
    const base = getUrlbase().replace(/\/$/, '')
    return `${base}/api/openapi.json`
  }

  onMount(async () => {
    await import('rapidoc')
    ready = true
  })

  function injectApiKey(node: HTMLElement) {
    const apply = () => {
      const key = getApiKey()
      if (!key) return
      const el = node as HTMLElement & { setApiKey?: (id: string, v: string) => void }
      el.setApiKey?.('apiKey', key)
    }
    node.addEventListener('spec-loaded', apply)
    return () => node.removeEventListener('spec-loaded', apply)
  }
</script>

<h4 class="mb-2">{$_('pages.docs.Title')}</h4>
<PageIntro id="pages.docs" />

{#if ready}
  <rapi-doc
    {@attach injectApiKey}
    spec-url={specUrl()}
    render-style="read"
    theme={theme.resolved}
    primary-color="#0f7bc4"
    show-header="false"
    allow-server-selection="false"
    allow-authentication="true"
    style="height: calc(100vh - 160px); width: 100%"
  ></rapi-doc>
{:else}
  <p class="text-muted">{$_('phrases.LoadingApi')}</p>
{/if}
