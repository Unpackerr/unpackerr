<script lang="ts">
  import type { Snippet } from 'svelte'
  import { _ } from '../lib/i18n/Translate.svelte'

  /** Translation prefix, e.g. `pages.settings.webhooks` → `pages.settings.webhooks.intro`. */
  let { id, children }: { id: string; children?: Snippet } = $props()

  const key = $derived(`${id}.intro`)
  const text = $derived($_(key))
</script>

{#if text && text !== key}
  <p class="text-muted mb-3">
    {text}{#if children}<span class="ms-2 d-inline-block"
        >{@render children()}</span
      >{/if}
  </p>
{/if}
