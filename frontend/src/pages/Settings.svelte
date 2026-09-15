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
  import HooksForm from './settings/HooksForm.svelte'

  type SettingsTab = ConfigSection

  const hookTabs: ConfigSection[] = ['webhooks', 'cmdhooks']

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
  const heading = $derived(
    $_('pages.settings.Heading', {
      values: {
        settings: $_('pages.settings.Title'),
        page: $_('pages.settings.' + (isStarr ? 'starrs' : current) + '.label'),
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
          {$_('pages.settings.' + id + '.label')}
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
{:else if isHook}
  {#key current}
    <HooksForm section={current as ConfigSection} />
  {/key}
{:else if isStarr}
  {#key current}
    <StarrForm section={current as ConfigSection} />
  {/key}
{/if}
