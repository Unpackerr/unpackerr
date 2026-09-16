<script lang="ts">
  import { Alert } from '@sveltestrap/sveltestrap'
  import { _ } from '../lib/i18n/Translate.svelte'
  import { restart } from '../lib/restart.svelte'
</script>

{#if restart.pending}
  <Alert
    color="warning"
    class="mb-3"
    isOpen={restart.pending}
    dismissible
    toggle={() => (restart.pending = false)}
  >
    <strong>{$_('phrases.RestartPendingTitle')}</strong>
    {$_('phrases.RestartPending')}
  </Alert>
{/if}
