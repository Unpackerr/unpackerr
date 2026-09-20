<script lang="ts">
  import { Nav, NavItem, NavLink } from '@sveltestrap/sveltestrap'
  import { _ } from '../lib/i18n/Translate.svelte'
  import { segments, hashLinkClick } from '../lib/router.svelte'
  import { has } from '../lib/auth.svelte'
  import { configPerm } from '../lib/perms'
  import type { ConfigSection } from '../lib/types'
  import { STARR_SECTIONS } from '../lib/types'
  import PageIntro from '../components/PageIntro.svelte'
  import GeneralForm from './settings/GeneralForm.svelte'
  import WebserverForm from './settings/WebserverForm.svelte'
  import StarrForm from './settings/StarrForm.svelte'
  import StarrPollInterval from './settings/StarrPollInterval.svelte'
  import FoldersForm from './settings/FoldersForm.svelte'
  import PayloadForm from './settings/PayloadForm.svelte'
  import HooksForm from './settings/HooksForm.svelte'

  type SettingsTab = ConfigSection

  const hookTabs: ConfigSection[] = ['hooks', 'webhooks', 'cmdhooks']

  const current = $derived((segments()[1] as SettingsTab) ?? 'general')
  const starrTabs = $derived(
    STARR_SECTIONS.filter((id) => has(configPerm(id, 'read'))),
  )
  const hookVisible = $derived(
    hookTabs.filter((id) => has(configPerm(id, 'read'))),
  )
  const isStarr = $derived(STARR_SECTIONS.includes(current as ConfigSection))
  const isHook = $derived(hookTabs.includes(current as ConfigSection))
  const innerTabs = $derived(isStarr ? starrTabs : isHook ? hookVisible : [])
  function tabLabel(id: ConfigSection): string {
    if (id === 'hooks') return $_('pages.settings.hooks.tab')
    return $_('pages.settings.' + id + '.label')
  }
  const heading = $derived(
    $_('pages.settings.Heading', {
      values: {
        settings: $_('pages.settings.Title'),
        page: isStarr ? $_('pages.settings.starrs.label') : tabLabel(current),
      },
    }),
  )
</script>

<h4 class="mb-3">{heading}</h4>

{#if isStarr}
  <StarrPollInterval />
{/if}

{#if innerTabs.length > 1}
  <Nav tabs class="settings-nav mb-3 flex-wrap">
    {#each innerTabs as id (id)}
      <NavItem>
        <NavLink
          href={'#/settings/' + id}
          active={current === id}
          onclick={(e) => hashLinkClick(e, '/settings/' + id)}
        >
          {tabLabel(id)}
        </NavLink>
      </NavItem>
    {/each}
  </Nav>
{/if}

<PageIntro id={'pages.settings.' + current} />

{#if !has(configPerm(current, 'read'))}
  <p class="text-muted">{$_('phrases.YouDoNotHavePermission')}</p>
{:else if current === 'general'}
  <GeneralForm />
{:else if current === 'webserver'}
  <WebserverForm />
{:else if current === 'folders'}
  <FoldersForm />
{:else if current === 'hooks'}
  <PayloadForm />
{:else if isHook}
  {#key current}
    <HooksForm section={current as ConfigSection} />
  {/key}
{:else if isStarr}
  {#key current}
    <StarrForm section={current as ConfigSection} />
  {/key}
{/if}
