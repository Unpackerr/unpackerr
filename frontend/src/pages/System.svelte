<script lang="ts">
  import { onMount } from 'svelte'
  import {
    Badge,
    Button,
    Card,
    CardBody,
    CardTitle,
    Col,
    Row,
    Spinner,
    Table,
  } from '@sveltestrap/sveltestrap'
  import { _ } from '../lib/i18n/Translate.svelte'
  import PageIntro from '../components/PageIntro.svelte'
  import { api } from '../lib/api'
  import { profile } from '../lib/auth.svelte'
  import { hashLinkClick } from '../lib/router.svelte'
  import { dateTime } from '../lib/format'
  import { success, failure } from '../lib/toast'
  import type { SystemInfo } from '../lib/types'
  import unpackerrIcon from '../assets/icon.png'
  import githubIcon from '../assets/github.svg'
  import discordIcon from '../assets/discord.svg'
  import notifiarrIcon from '../assets/notifiarr.svg'
  import xtIcon from '../assets/xt.png'

  const links = [
    {
      href: 'https://unpackerr.zip',
      icon: unpackerrIcon,
      label: 'pages.system.Website',
      hint: 'unpackerr.zip',
    },
    {
      href: 'https://github.com/Unpackerr/unpackerr',
      icon: githubIcon,
      label: 'pages.system.GitHub',
      hint: 'github.com/Unpackerr/unpackerr',
      mono: true,
    },
    {
      href: 'https://golift.io/discord',
      icon: discordIcon,
      label: 'pages.system.Discord',
      hint: 'golift.io/discord',
    },
    {
      href: 'https://notifiarr.com',
      icon: notifiarrIcon,
      label: 'pages.system.Notifiarr',
      hint: 'notifiarr.com',
    },
    {
      href: 'https://unpackerr.zip/xt',
      icon: xtIcon,
      label: 'pages.system.Xt',
      hint: 'unpackerr.zip/xt',
    },
  ]

  let info = $state<SystemInfo | null>(null)
  let error = $state('')
  let loading = $state(true)
  let exportText = $state('')
  let exporting = $state(false)

  onMount(async () => {
    const res = await api.get<SystemInfo>('system')
    if (res.ok) info = res.body
    else
      error =
        (res.body as { error?: string })?.error ?? 'failed to load system info'
    loading = false
  })

  async function loadExport() {
    exporting = true
    const res = await api.get<{ text: string }>('system/export')
    if (res.ok) exportText = res.body?.text ?? ''
    else
      failure(
        (res.body as { error?: string })?.error ?? 'failed to load live export',
      )
    exporting = false
  }

  function hostLabel(sys: SystemInfo): string {
    const name = sys.hostname?.trim()
    const os = sys.goos?.trim()
    if (name && os) {
      return `${name} (${os})`
    }
    return name || os || $_('phrases.Empty')
  }

  async function copyExport() {
    try {
      await navigator.clipboard.writeText(exportText)
      success($_('phrases.Copied'))
    } catch {
      // clipboard may be blocked on non-HTTPS
    }
  }
</script>

<h4 class="mb-2">{$_('pages.system.Title')}</h4>
<PageIntro id="pages.system" />

{#if loading}
  <Spinner color="primary" />
{:else if error}
  <p class="text-danger">{error}</p>
{:else if info}
  <Row class="g-3">
    <Col md="6" class="min-w-0">
      <Card>
        <CardBody>
          <Row class="align-items-center mb-2">
            <Col>
              <CardTitle class="mb-0">{$_('pages.system.Instance')}</CardTitle>
            </Col>
            <Col xs="auto">
              <Button
                color="warning"
                size="sm"
                href="#/docs"
                on:click={(e) => hashLinkClick(e, '/docs')}
              >
                {$_('nav.API')}
              </Button>
            </Col>
          </Row>
          <Table borderless class="mb-0 instance-table">
            <tbody>
              <tr
                ><th class="text-muted">{$_('pages.system.Version')}</th><td
                  >{info.version}</td
                ></tr
              >
              <tr
                ><th class="text-muted">{$_('pages.system.Host')}</th><td
                  ><code>{hostLabel(info)}</code></td
                ></tr
              >
              <tr
                ><th class="text-muted">{$_('pages.system.Started')}</th><td
                  >{dateTime(info.started)}</td
                ></tr
              >
              <tr
                ><th class="text-muted">{$_('pages.system.Uptime')}</th><td
                  >{info.uptime}</td
                ></tr
              >
              <tr
                ><th class="text-muted">{$_('pages.system.Listen')}</th><td
                  ><code>{info.listenAddr}</code></td
                ></tr
              >
              <tr>
                <th class="text-muted">{$_('pages.system.ConfigFile')}</th>
                <td class="text-break"
                  ><code class="wrap"
                    >{info.configFile || $_('phrases.Empty')}</code
                  ></td
                >
              </tr>
              <tr>
                <th class="text-muted">{$_('pages.system.Logs')}</th>
                <td class="text-break"
                  ><code class="wrap">{info.logs || $_('phrases.Empty')}</code></td
                >
              </tr>
              <tr
                ><th class="text-muted">{$_('pages.system.Auth')}</th><td
                  ><Badge color="info">{info.auth}</Badge></td
                ></tr
              >
              <tr>
                <th class="text-muted">{$_('pages.system.Metrics')}</th>
                <td>
                  <Badge color={info.metrics ? 'success' : 'secondary'}>
                    {info.metrics
                      ? $_('pages.system.enabled')
                      : $_('pages.system.disabled')}
                  </Badge>
                </td>
              </tr>
            </tbody>
          </Table>
        </CardBody>
      </Card>
    </Col>
    <Col md="6" class="min-w-0">
      <Card>
        <CardBody>
          <CardTitle>{$_('pages.system.You')}</CardTitle>
          <Table borderless class="mb-0 you-table">
            <tbody>
              <tr
                ><th class="text-muted">{$_('pages.system.Username')}</th><td
                  >{profile.info?.username}</td
                ></tr
              >
              <tr
                ><th class="text-muted">{$_('pages.system.Via')}</th><td
                  >{profile.info?.via}</td
                ></tr
              >
              <tr>
                <th class="text-muted align-top"
                  >{$_('pages.system.Permissions')}</th
                >
                <td>
                  {#if profile.info?.permissions.includes('*')}
                    <Badge color="primary">{$_('phrases.AdminAll')}</Badge>
                  {:else}
                    {#each profile.info?.permissions ?? [] as perm (perm)}
                      <Badge color="secondary" class="me-1 mb-1">{perm}</Badge>
                    {/each}
                  {/if}
                </td>
              </tr>
            </tbody>
          </Table>
        </CardBody>
      </Card>
      <Card class="mt-3">
        <CardBody class="py-2">
          <CardTitle class="mb-2">{$_('pages.system.Links')}</CardTitle>
          <div class="link-grid">
            {#each links as link (link.href)}
              <a
                class="link-row"
                href={link.href}
                title={link.hint}
                target="_blank"
                rel="noopener noreferrer"
              >
                <img
                  src={link.icon}
                  alt=""
                  class={['link-ico', link.mono && 'link-ico-mono']}
                />
                <span class="min-w-0">
                  <span class="d-block">{$_(link.label)}</span>
                  <small class="text-muted hint">{link.hint}</small>
                </span>
              </a>
            {/each}
          </div>
        </CardBody>
      </Card>
    </Col>
    <Col md="12">
      <Card>
        <CardBody>
          <Row class="align-items-center mb-2">
            <Col>
              <CardTitle class="mb-0">{$_('buttons.LiveExport')}</CardTitle>
            </Col>
            <Col xs="auto">
              <Button
                color="secondary"
                outline
                disabled={exporting}
                on:click={loadExport}
              >
                {#if exporting}<Spinner size="sm" />{/if}
                <span class="ms-1">{$_('buttons.LiveExport')}</span>
              </Button>
              {#if exportText}
                <Button color="primary" class="ms-1" on:click={copyExport}
                  >{$_('buttons.Copy')}</Button
                >
              {/if}
            </Col>
          </Row>
          {#if exportText}
            <pre class="export-pre mb-0">{exportText}</pre>
          {/if}
        </CardBody>
      </Card>
    </Col>
  </Row>
{/if}

<style>
  :global(.instance-table) {
    table-layout: fixed;
    width: 100%;
  }

  :global(.instance-table th) {
    width: 8.5rem;
  }

  :global(.instance-table th),
  :global(.instance-table td),
  :global(.you-table th),
  :global(.you-table td) {
    padding-block: 0.3rem;
  }

  .link-grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    column-gap: 1rem;
    row-gap: 0.45rem;
  }

  .link-row {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    padding: 0.2rem 0;
    color: inherit;
    text-decoration: none;
    min-width: 0;
    line-height: 1.25;
  }

  .link-row:hover {
    color: var(--bs-link-color);
  }

  .link-row .hint {
    display: block;
    overflow-wrap: anywhere;
  }

  .link-ico {
    width: 1.75rem;
    height: 1.75rem;
    object-fit: contain;
    flex-shrink: 0;
  }

  :global([data-bs-theme='dark']) .link-ico-mono {
    filter: invert(1);
  }

  .export-pre {
    padding: 0.75rem;
    background: var(--bs-tertiary-bg, #f8f9fa);
    border-radius: 0.25rem;
    font-size: 0.8rem;
    white-space: pre-wrap;
    word-break: break-word;
  }
</style>
