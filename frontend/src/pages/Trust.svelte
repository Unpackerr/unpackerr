<script lang="ts">
  import { _ } from '../lib/i18n/Translate.svelte'
  import { has } from '../lib/auth.svelte'
  import { configPerm } from '../lib/perms'
  import WebserverForm from './settings/WebserverForm.svelte'
</script>

<h4 class="mb-3">{$_('pages.trust.Title')}</h4>

{#if !has(configPerm('webserver', 'read'))}
  <p class="text-muted">{$_('phrases.YouDoNotHavePermission')}</p>
{:else}
  <WebserverForm pane="auth" />
{/if}
