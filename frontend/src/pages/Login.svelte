<script lang="ts">
  import {
    Button,
    Card,
    CardBody,
    CardFooter,
    CardHeader,
    Col,
    Container,
    Form,
    Input,
    Label,
    Row,
    Spinner,
  } from '@sveltestrap/sveltestrap'
  import { _ } from '../lib/i18n/Translate.svelte'
  import { login } from '../lib/auth.svelte'
  import icon from '../assets/icon.png'

  let username = $state('admin')
  let password = $state('')
  let loading = $state(false)
  let error = $state('')

  async function onsubmit(e: Event) {
    e.preventDefault()
    if (!password) {
      error = $_('phrases.EnterPassword')
      return
    }
    loading = true
    error = ''
    error = await login(username, password)
    loading = false
  }
</script>

<Container>
  <Row
    class="justify-content-center align-items-center"
    style="min-height: 100vh"
  >
    <Col xs="auto">
      <Card style="width: 22rem" class="shadow">
        <CardHeader class="navbar-unpackerr text-white">
          <Row class="align-items-center g-2">
            <Col xs="auto"><img src={icon} alt="" class="brand-logo" /></Col>
            <Col><span class="fw-semibold">Unpackerr</span></Col>
          </Row>
        </CardHeader>
        <CardBody>
          <Form on:submit={onsubmit}>
            <Label for="username" class="form-label mb-1"
              >{$_('pages.login.Username')}</Label
            >
            <Input
              id="username"
              type="text"
              bind:value={username}
              autocomplete="username"
            />
            <Label for="password" class="form-label mb-1 mt-3"
              >{$_('pages.login.Password')}</Label
            >
            <Input
              id="password"
              type="password"
              class="mb-3"
              bind:value={password}
              autocomplete="current-password"
            />
            <Button
              type="submit"
              color="primary"
              class="w-100"
              disabled={loading}
            >
              {#if loading}<Spinner size="sm" />{/if}
              <span class="ms-1">{$_('buttons.Login')}</span>
            </Button>
          </Form>
        </CardBody>
        {#if error}
          <CardFooter class="text-danger small" role="alert" aria-live="assertive"
            >{error}</CardFooter
          >
        {/if}
      </Card>
    </Col>
  </Row>
</Container>
