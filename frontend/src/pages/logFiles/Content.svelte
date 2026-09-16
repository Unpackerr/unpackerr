<script lang="ts">
  import type { LogFileInfo } from '../../lib/types'
  import { api } from '../../lib/api'
  import { _ } from '../../lib/i18n/Translate.svelte'
  import {
    Button,
    Card,
    CardBody,
    Col,
    Input,
    InputGroup,
    ListGroup,
    ListGroupItem,
    Row,
  } from '@sveltestrap/sveltestrap'
  import Icon from '../../components/Icon.svelte'
  import CircleNotch from 'phosphor-svelte/lib/CircleNotch'
  import Palette from 'phosphor-svelte/lib/Palette'
  import SortDescending from 'phosphor-svelte/lib/SortDescending'
  import SortAscending from 'phosphor-svelte/lib/SortAscending'
  import ArrowsLeftRight from 'phosphor-svelte/lib/ArrowsLeftRight'
  import ArrowUDownLeft from 'phosphor-svelte/lib/ArrowUDownLeft'
  import { failure } from '../../lib/toast'
  import { live } from '../../lib/socket.svelte'
  import { onMount } from 'svelte'

  let { file, tail = false }: { file: LogFileInfo; tail?: boolean } = $props()

  const isErrors = $derived(file.id === 'errors' || file.path === '(errors)')

  let lineCount = $state(500)
  let offset = $state(500)
  let desc = $state(true)
  let highlight = $state('')
  let colors = $state(true)
  let wrap = $state(true)
  let body = $state('')
  let ok = $state(false)
  let adding = $state(false)
  let list = $state<string[]>([])
  let gone = false

  function clampedLines(): number {
    const n = Math.floor(Number(lineCount))
    if (!Number.isFinite(n) || n <= 0) return 500
    return Math.min(10000, n)
  }

  function colorLine(line: string) {
    if (highlight && line.includes(highlight)) return 'bg-success text-white'
    if (!colors) return ''
    if (isErrors) return 'bg-danger-subtle'
    const lower = line.toLowerCase()
    if (lower.includes('error') || lower.includes('fail')) {
      return 'bg-danger-subtle'
    }
    if (line.includes('DEBUG') || line.includes('[DEBUG]')) {
      return 'bg-primary-subtle'
    }
    if (line.includes('[Unpackerr]')) return 'bg-info-subtle'
    if (lower.includes('extract')) return 'bg-primary-subtle'
    return ''
  }

  function stopFollow() {
    live.setLogHandler(undefined)
    live.setErrorHandler(undefined)
    if (tail && !isErrors) live.unsubscribe(['logs'])
  }

  function appendLine(line: string) {
    const cap = clampedLines()
    list = [...list, line]
    if (list.length > cap) {
      list = list.slice(list.length - cap)
    }
  }

  async function load() {
    adding = true
    stopFollow()
    list = []
    body = ''
    const n = clampedLines()
    if (!tail) {
      const res = await api.get<{ text?: string; error?: string }>(
        `logs/${file.id}?lines=${n}&skip=0`,
      )
      if (gone) return
      ok = res.ok
      body = res.ok ? (res.body?.text ?? '') : String(res.body?.error ?? '')
      offset = n
    } else if (isErrors) {
      const res = await api.get<{ text?: string; error?: string }>(
        `logs/${file.id}?lines=${n}&skip=0`,
      )
      if (gone) return
      ok = res.ok
      if (res.ok) {
        const text = res.body?.text ?? ''
        list = text.trimEnd() ? text.trimEnd().split('\n') : []
      } else {
        body = String(res.body?.error ?? '')
      }
      live.setErrorHandler(appendLine)
    } else {
      live.setLogHandler((fid, line) => {
        if (fid && fid !== file.id) return
        appendLine(line)
      })
      live.subscribe(['logs'], file.id)
      ok = true
    }
    adding = false
  }

  async function add() {
    adding = true
    const n = clampedLines()
    const res = await api.get<{ text?: string; error?: string }>(
      `logs/${file.id}?lines=${n}&skip=${offset}`,
    )
    if (res.ok) {
      const text = (res.body?.text ?? '').trimEnd()
      if (text) {
        offset += text.split('\n').length
        body = body ? text + '\n' + body.trimEnd() : text
      }
    } else {
      failure(res.body?.error ?? $_('pages.logs.Error'))
    }
    adding = false
  }

  onMount(() => {
    void load()
    return () => {
      gone = true
      stopFollow()
    }
  })

  const lines = $derived.by(() => {
    const raw = tail ? list.join('\n') : body
    const parts = raw.trimEnd() ? raw.trimEnd().split('\n') : []
    return desc ? parts : parts.slice().reverse()
  })
  const lineNumberWidth = $derived(
    Math.floor(Math.log10(Math.max(lines.length, 1))) + 1,
  )
</script>

{#if adding && !ok && !body && list.length === 0}
  <h5 class="text-success">
    <span class="spin d-inline-block"><Icon i={CircleNotch} /></span>
    {$_('phrases.LoadingApi')}
  </h5>
{:else if tail || ok}
  <Row>
    <Col sm={12} md="auto" class="mb-2">
      <InputGroup>
        <Button
          outline
          onclick={() => (desc = !desc)}
          active={!desc}
          title={$_('pages.logs.ToggleLinesOrder')}
        >
          <Icon i={desc ? SortAscending : SortDescending} />
        </Button>
        <Button
          outline
          onclick={() => (colors = !colors)}
          active={!colors}
          title={$_('pages.logs.ToggleColors')}
        >
          <span class={adding || (tail && ok) ? 'spin d-inline-block' : ''}
            ><Icon i={Palette} /></span
          >
        </Button>
        <Input
          title={$_('pages.logs.LineCount')}
          style="width: 7rem"
          type="number"
          min={10}
          max={10000}
          bind:value={lineCount}
        />
        <Button outline onclick={add} disabled={file.used || adding || tail}>
          {#if tail}
            {$_('pages.logs.Tailing')}
          {:else}
            {$_('pages.logs.AddMore')}
          {/if}
        </Button>
        <Button outline onclick={() => void load()} disabled={adding}>
          {$_('buttons.Refresh')}
        </Button>
      </InputGroup>
    </Col>
    <Col class="mb-2">
      <InputGroup>
        <Input bind:value={highlight} placeholder={$_('pages.logs.Highlight')} />
        <Button
          outline
          onclick={() => (wrap = !wrap)}
          active={wrap}
          title={$_('pages.logs.ToggleLineWrap')}
        >
          <Icon i={wrap ? ArrowUDownLeft : ArrowsLeftRight} />
        </Button>
      </InputGroup>
    </Col>
  </Row>

  <div
    class={['log-file-content', !wrap && 'no-wrap']}
    style:--line-number-width="{lineNumberWidth}ch"
  >
    <ListGroup flush numbered class="ps-0">
      {#each lines as line, i (i + line.slice(0, 24))}
        <ListGroupItem class="border-0 lh-1">
          <span class={['log-line', colorLine(line)]}>
            <pre class={['m-0', 'pre', !wrap && 'no-wrap']}>{line}</pre>
          </span>
        </ListGroupItem>
      {/each}
    </ListGroup>
  </div>
{:else}
  <Card color="danger" outline>
    <CardBody>{$_('pages.logs.Error')}: {body}</CardBody>
  </Card>
{/if}

<style>
  .log-file-content {
    overflow-x: hidden;
  }
  .log-file-content.no-wrap {
    overflow-x: auto;
  }
  .log-file-content :global(.list-group) {
    counter-reset: liCounter;
  }
  .log-file-content :global(.list-group-item) {
    display: block;
    position: relative;
    overflow: visible;
    min-width: 0;
    padding: 0 0 0 calc(var(--line-number-width) + 1ch);
  }
  .log-file-content :global(.list-group-item)::before {
    color: var(--bs-secondary-color);
    font-family: monospace;
    counter-increment: liCounter;
    content: counter(liCounter);
    display: inline-block;
    min-width: var(--line-number-width);
    text-align: right;
    position: absolute;
    left: 0;
    top: 0;
  }
  .log-line {
    display: block;
    min-width: 0;
  }
  pre.pre {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    word-break: break-word;
    max-width: 100%;
    overflow: hidden;
  }
  pre.no-wrap {
    white-space: pre;
    overflow: visible;
    overflow-wrap: normal;
    word-break: normal;
    max-width: none;
  }
  .spin {
    animation: spin 1s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
