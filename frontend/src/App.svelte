<script lang="ts">
  import { onMount } from 'svelte'
  import {
    Button,
    Col,
    Container,
    Modal,
    ModalBody,
    ModalFooter,
    ModalHeader,
    Row,
    Spinner,
  } from '@sveltestrap/sveltestrap'
  import { Toaster } from 'svelte-sonner'
  import { isLoading } from 'svelte-i18n'
  import { _ } from './lib/i18n/Translate.svelte'
  import { profile, restore } from './lib/auth.svelte'
  import { theme } from './lib/theme.svelte'
  import {
    router,
    startRouter,
    segments,
    stayHere,
    discardAndGo,
  } from './lib/router.svelte'
  import { dirty } from './lib/dirty.svelte'
  import { api } from './lib/api'
  import Nav from './components/Nav.svelte'
  import RestartBanner from './components/RestartBanner.svelte'
  import Login from './pages/Login.svelte'
  import Dashboard from './pages/Dashboard.svelte'
  import Settings from './pages/Settings.svelte'
  import Trust from './pages/Trust.svelte'
  import System from './pages/System.svelte'
  import Logs from './pages/logFiles/Index.svelte'
  import ApiDocs from './pages/ApiDocs.svelte'

  // Session ping 401s back to login after a restart even when the dashboard is not open.
  const sessionPollMs = 60_000

  onMount(() => {
    startRouter()
    restore()
    const id = setInterval(() => {
      if (profile.authed) void api.get('auth/me')
    }, sessionPollMs)
    return () => clearInterval(id)
  })

  const top = $derived(segments()[0] ?? '')
  const booting = $derived($isLoading || !profile.loaded)

  function beforeUnload(e: BeforeUnloadEvent) {
    if (!dirty.changed) return
    e.preventDefault()
    e.returnValue = ''
  }
</script>

<svelte:window onbeforeunload={beforeUnload} />

<Toaster
  position="top-center"
  theme={theme.resolved}
  richColors
  closeButton
  offset="4rem"
/>

{#if booting}
  <Container>
    <Row
      class="justify-content-center align-items-center"
      style="min-height: 60vh"
    >
      <Col xs="auto">
        <Spinner color="primary" />
      </Col>
    </Row>
  </Container>
{:else if !profile.authed}
  <Login />
{:else}
  <div class="app-shell">
    <Nav />
    <main class="app-main">
      <Container xxl class="page-wrap pt-2 pb-3">
        <RestartBanner />
        {#if top === ''}
          <Dashboard />
        {:else if top === 'settings'}
          <Settings />
        {:else if top === 'trust'}
          <Trust />
        {:else if top === 'system'}
          <System />
        {:else if top === 'logs'}
          <Logs />
        {:else if top === 'docs'}
          <ApiDocs />
        {:else}
          <p>{$_('phrases.NotFound', { values: { path: router.path } })}</p>
        {/if}
      </Container>
    </main>
  </div>
{/if}

<Modal isOpen={dirty.pending !== null} toggle={stayHere}>
  <ModalHeader toggle={stayHere}>{$_('phrases.UnsavedChanges')}</ModalHeader>
  <ModalBody>{$_('phrases.LeavePage')}</ModalBody>
  <ModalFooter>
    <Button color="secondary" type="button" onclick={stayHere}
      >{$_('buttons.Stay')}</Button
    >
    <Button color="danger" type="button" onclick={discardAndGo}
      >{$_('buttons.Leave')}</Button
    >
  </ModalFooter>
</Modal>
