<script lang="ts">
  import { onMount } from 'svelte'
  import {
    Navbar,
    NavbarBrand,
    Nav,
    NavItem,
    NavLink,
    Collapse,
    NavbarToggler,
    Dropdown,
    DropdownItem,
    DropdownMenu,
    DropdownToggle,
  } from '@sveltestrap/sveltestrap'
  import { _ } from '../lib/i18n/Translate.svelte'
  import { locale } from '../lib/i18n/locale.svelte'
  import { theme } from '../lib/theme.svelte'
  import { router, hashLinkClick } from '../lib/router.svelte'
  import { profile, logout, has } from '../lib/auth.svelte'
  import { configPerm, systemPerm } from '../lib/perms'
  import { STARR_SECTIONS } from '../lib/types'
  import Icon from './Icon.svelte'
  import Lock from 'phosphor-svelte/lib/Lock'
  import Monitor from 'phosphor-svelte/lib/Monitor'
  import Scroll from 'phosphor-svelte/lib/Scroll'
  import Sun from 'phosphor-svelte/lib/Sun'
  import Moon from 'phosphor-svelte/lib/Moon'
  import SignOut from 'phosphor-svelte/lib/SignOut'
  import icon from '../assets/icon.png'

  const md = 768
  let wide = $state(typeof window !== 'undefined' && window.innerWidth >= md)
  let open = $state(typeof window !== 'undefined' && window.innerWidth >= md)

  function syncWidth(width: number) {
    const nowWide = width >= md
    if (nowWide === wide) return
    wide = nowWide
    open = nowWide
  }

  onMount(() => syncWidth(window.innerWidth))

  function onResize() {
    syncWidth(window.innerWidth)
  }

  let userOpen = $state(false)
  let langOpen = $state(false)
  let settingsOpen = $state(false)

  function closeMenus() {
    settingsOpen = false
    userOpen = false
    langOpen = false
    if (!wide) open = false
  }

  function go(e: MouseEvent, path: string) {
    hashLinkClick(e, path)
    closeMenus()
  }

  function toggleUser() {
    userOpen = !userOpen
    if (userOpen) {
      settingsOpen = false
    } else {
      langOpen = false
    }
  }

  function toggleSettings() {
    settingsOpen = !settingsOpen
  }

  const langName = $derived(
    locale.list.find((l) => l.id === locale.current)?.name ?? locale.current,
  )

  const firstStarr = $derived(
    STARR_SECTIONS.find((id) => has(configPerm(id, 'read'))) ?? 'sonarr',
  )
  const firstHook = $derived(
    (['webhooks', 'cmdhooks'] as const).find((id) =>
      has(configPerm(id, 'read')),
    ) ?? 'webhooks',
  )
  const settingItems = $derived(
    [
      {
        href: '/settings/general',
        label: $_('pages.settings.general.label'),
        show: has(configPerm('general', 'read')),
      },
      {
        href: '/settings/webserver',
        label: $_('pages.settings.webserver.label'),
        show: has(configPerm('webserver', 'read')),
      },
      {
        href: '/settings/' + firstStarr,
        label: $_('pages.settings.starrs.label'),
        show: STARR_SECTIONS.some((id) => has(configPerm(id, 'read'))),
      },
      {
        href: '/settings/folders',
        label: $_('pages.settings.folders.label'),
        show: has(configPerm('folders', 'read')),
      },
      {
        href: '/settings/' + firstHook,
        label: $_('pages.settings.hooks.label'),
        show:
          has(configPerm('webhooks', 'read')) ||
          has(configPerm('cmdhooks', 'read')),
      },
    ].filter((item) => item.show),
  )

  function settingActive(href: string): boolean {
    const path = router.path
    if (
      href.startsWith('/settings/') &&
      STARR_SECTIONS.includes(
        href.slice('/settings/'.length) as (typeof STARR_SECTIONS)[number],
      )
    ) {
      return STARR_SECTIONS.some((id) => path === '/settings/' + id)
    }
    if (href.includes('webhooks') || href.includes('cmdhooks')) {
      return path === '/settings/webhooks' || path === '/settings/cmdhooks'
    }
    if (href === '/settings/general') {
      return path === '/settings' || path === '/settings/general'
    }
    return path === href
  }
</script>

<svelte:window onresize={onResize} />

<Navbar class="navbar-unpackerr" dark expand="md" container="fluid">
  <NavbarBrand href="#/" on:click={(e) => go(e, '/')}>
    <img src={icon} alt="" class="brand-logo me-2" />
    <span class="fw-semibold text-white">Unpackerr</span>
  </NavbarBrand>
  <NavbarToggler on:click={() => (open = !open)} />
  <Collapse isOpen={open} navbar>
    <Nav class="me-auto" navbar>
      {#if wide && settingItems.length}
        <Dropdown nav isOpen={settingsOpen} toggle={toggleSettings} autoClose="outside">
          <DropdownToggle
            nav
            caret
            class={router.path.startsWith('/settings') ? 'active' : ''}
          >
            {$_('nav.Settings')}
          </DropdownToggle>
          <DropdownMenu>
            {#each settingItems as item (item.href)}
              <DropdownItem
                active={settingActive(item.href)}
                on:click={(e) => go(e, item.href)}
              >
                {item.label}
              </DropdownItem>
            {/each}
          </DropdownMenu>
        </Dropdown>
      {/if}
    </Nav>
    {#if wide}
      <Nav navbar>
        <Dropdown nav isOpen={userOpen} toggle={toggleUser} autoClose="outside">
          <DropdownToggle
            nav
            caret
            class={router.path === '/trust' ||
            router.path === '/system' ||
            router.path === '/logs' ||
            router.path.startsWith('/logs/')
              ? 'active'
              : ''}
            title={profile.info?.username ?? ''}
          >
            {profile.info?.username ?? ''}
          </DropdownToggle>
          <DropdownMenu end class="user-menu">
            {#if has(configPerm('webserver', 'read'))}
              <DropdownItem
                active={router.path === '/trust'}
                on:click={(e) => go(e, '/trust')}
              >
                <span class="menu-ico"><Icon i={Lock} /></span>
                {$_('pages.trust.Title')}
              </DropdownItem>
            {/if}
            {#if has(systemPerm('info', 'read'))}
              <DropdownItem
                active={router.path === '/system'}
                on:click={(e) => go(e, '/system')}
              >
                <span class="menu-ico"><Icon i={Monitor} /></span>
                {$_('nav.System')}
              </DropdownItem>
            {/if}
            {#if has(systemPerm('logs', 'read'))}
              <DropdownItem
                active={router.path === '/logs' || router.path.startsWith('/logs/')}
                on:click={(e) => go(e, '/logs')}
              >
                <span class="menu-ico"><Icon i={Scroll} /></span>
                {$_('nav.LogFiles')}
              </DropdownItem>
            {/if}
            <DropdownItem divider />
            <li class="dropdown-submenu">
              <button
                type="button"
                class="dropdown-item dropdown-toggle"
                onclick={() => (langOpen = !langOpen)}
              >
                <span class="menu-ico"
                  >{locale.flags[locale.current] ?? ''}</span
                >
                {langName}
              </button>
              {#if langOpen}
                <ul class="dropdown-menu show">
                  {#each locale.list as loc (loc.id)}
                    <DropdownItem
                      active={locale.current === loc.id}
                      on:click={() => {
                        locale.set(loc.id)
                        langOpen = false
                      }}
                    >
                      <span class="menu-ico">{locale.flags[loc.id]}</span>
                      {loc.name}
                    </DropdownItem>
                  {/each}
                </ul>
              {/if}
            </li>
            <DropdownItem on:click={() => theme.toggle()}>
              <span class="menu-ico"
                ><Icon i={theme.mode === 'dark' ? Moon : Sun} /></span
              >
              {theme.mode === 'dark'
                ? $_('nav.ThemeDark')
                : $_('nav.ThemeLight')}
            </DropdownItem>
            <DropdownItem divider />
            <DropdownItem on:click={logout}>
              <span class="menu-ico"><Icon i={SignOut} /></span>
              {$_('buttons.Logout')}
            </DropdownItem>
          </DropdownMenu>
        </Dropdown>
      </Nav>
    {:else}
      {#if settingItems.length}
        <Nav class="nav-section" navbar>
          {#each settingItems as item (item.href)}
            <NavItem>
              <NavLink
                href={'#' + item.href}
                active={settingActive(item.href)}
                on:click={(e) => go(e, item.href)}
              >
                {item.label}
              </NavLink>
            </NavItem>
          {/each}
        </Nav>
      {/if}
      <Nav class="nav-section nav-raised" navbar>
        {#if has(configPerm('webserver', 'read'))}
          <NavItem>
            <NavLink
              href="#/trust"
              active={router.path === '/trust'}
              on:click={(e) => go(e, '/trust')}
            >
              <span class="menu-ico"><Icon i={Lock} c1="#fff" d1="#fff" /></span
              >
              {$_('pages.trust.Title')}
            </NavLink>
          </NavItem>
        {/if}
        {#if has(systemPerm('info', 'read'))}
          <NavItem>
            <NavLink
              href="#/system"
              active={router.path === '/system'}
              on:click={(e) => go(e, '/system')}
            >
              <span class="menu-ico"
                ><Icon i={Monitor} c1="#fff" d1="#fff" /></span
              >
              {$_('nav.System')}
            </NavLink>
          </NavItem>
        {/if}
        {#if has(systemPerm('logs', 'read'))}
          <NavItem>
            <NavLink
              href="#/logs"
              active={router.path === '/logs' || router.path.startsWith('/logs/')}
              on:click={(e) => go(e, '/logs')}
            >
              <span class="menu-ico"
                ><Icon i={Scroll} c1="#fff" d1="#fff" /></span
              >
              {$_('nav.LogFiles')}
            </NavLink>
          </NavItem>
        {/if}
      </Nav>
      <Nav class="nav-section nav-raised" navbar>
        <NavItem>
          <button
            type="button"
            class="nav-link"
            onclick={() => (langOpen = !langOpen)}
          >
            <span class="menu-ico">{locale.flags[locale.current] ?? ''}</span>
            {langName}
          </button>
        </NavItem>
        {#if langOpen}
          {#each locale.list as loc (loc.id)}
            <NavItem>
              <button
                type="button"
                class={[
                  'nav-link',
                  'ps-4',
                  locale.current === loc.id && 'active',
                ]}
                onclick={() => {
                  locale.set(loc.id)
                  langOpen = false
                }}
              >
                <span class="menu-ico">{locale.flags[loc.id]}</span>
                {loc.name}
              </button>
            </NavItem>
          {/each}
        {/if}
        <NavItem>
          <button type="button" class="nav-link" onclick={() => theme.toggle()}>
            <span class="menu-ico"
              ><Icon
                i={theme.mode === 'dark' ? Moon : Sun}
                c1="#fff"
                d1="#fff"
              /></span
            >
            {theme.mode === 'dark' ? $_('nav.ThemeDark') : $_('nav.ThemeLight')}
          </button>
        </NavItem>
        <NavItem>
          <button type="button" class="nav-link" onclick={logout}>
            <span class="menu-ico"
              ><Icon i={SignOut} c1="#fff" d1="#fff" /></span
            >
            {$_('buttons.Logout')}
          </button>
        </NavItem>
      </Nav>
    {/if}
  </Collapse>
</Navbar>

<style>
  :global(.navbar-unpackerr .user-menu .dropdown-item) {
    display: flex;
    align-items: center;
  }

  .menu-ico {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.25em;
    min-width: 1.25em;
    height: 1.25em;
    margin-right: 0.5rem;
    flex-shrink: 0;
    line-height: 1;
    overflow: hidden;
  }

  .dropdown-submenu {
    position: relative;
  }

  .dropdown-submenu > .dropdown-menu {
    top: 0;
    right: 100%;
    left: auto;
    margin-top: 0;
  }

  :global(.navbar-unpackerr .user-menu) {
    overflow: visible;
  }

  :global(.navbar-unpackerr .nav-section) {
    border-top: 1px solid rgba(255, 255, 255, 0.22);
    margin-top: 0.4rem;
    padding-top: 0.35rem;
  }

  :global(.navbar-unpackerr .nav-raised .nav-link) {
    display: flex;
    align-items: center;
  }

  :global(.navbar-unpackerr .nav-raised button.nav-link) {
    background: transparent;
    border: 0;
    width: 100%;
    text-align: start;
  }

  @media (max-width: 767.98px) {
    .dropdown-submenu > .dropdown-menu {
      position: static;
      right: auto;
      box-shadow: none;
      border: 0;
      padding-top: 0;
    }
  }
</style>
