<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    Badge,
    Button,
    ButtonGroup,
    Card,
    CardBody,
    CardTitle,
    Col,
    Row,
    Spinner,
    Table,
    Tooltip,
    Modal,
    ModalHeader,
    ModalBody,
    ModalFooter,
  } from '@sveltestrap/sveltestrap'
  import { _ } from '../lib/i18n/Translate.svelte'
  import PageIntro from '../components/PageIntro.svelte'
  import { api } from '../lib/api'
  import { has } from '../lib/auth.svelte'
  import { systemPerm } from '../lib/perms'
  import {
    statusColor,
    statusPhrase,
    errorPhrase,
    relTime,
    progressCaption,
    remainCompact,
    isZeroTime,
    bytes,
  } from '../lib/format'
  import { success, failure } from '../lib/toast'
  import { live, type LiveTopic } from '../lib/socket.svelte'
  import type { BufferStat, QueueItem } from '../lib/types'
  import History from './History.svelte'
  import ItemDetail from '../components/ItemDetail.svelte'
  import InfoIcon from 'phosphor-svelte/lib/Info'
  import { theme } from '../lib/theme.svelte'

  let busy = $state<Record<string, boolean>>({})
  let now = $state(Date.now())
  let ageTimer: ReturnType<typeof setInterval> | undefined
  let pendingForget = $state<QueueItem | null>(null)
  let pendingDetail = $state<QueueItem | null>(null)
  let detailFiles = $state<QueueItem | null>(null)
  let detailFilesReady = $state(false)

  function ageLabel(ms: number): string {
    const sec = Math.max(0, Math.floor(ms / 1000))
    if (sec < 60) return `${sec}s`
    const min = Math.floor(sec / 60)
    if (min < 60) return `${min}m`
    const hr = Math.floor(min / 60)
    if (hr < 24) return `${hr}h`
    return `${Math.floor(hr / 24)}d`
  }

  const dataAge = $derived(
    live.fetchedAt === undefined ? '' : ageLabel(now - live.fetchedAt),
  )

  const canQueue = has(systemPerm('queue', 'read'))
  const canWrite = has(systemPerm('queue', 'write'))
  const canStats = has(systemPerm('stats', 'read'))
  const canHistory = has(systemPerm('history', 'read'))

  const TERMINAL = [
    'extractfailed',
    'extractednothing',
    'imported',
    'deleted',
    'deletefailed',
  ]

  function statusHasExtractResult(status: string): boolean {
    switch (status.toLowerCase()) {
      case 'extracted':
      case 'extractednothing':
      case 'extractfailed':
      case 'imported':
      case 'deleting':
      case 'deleted':
      case 'deletefailed':
        return true
      default:
        return false
    }
  }

  const uid = $props.id()
  const stats = $derived(live.stats)
  const queue = $derived(live.queue)
  const loading = $derived(live.fetchedAt === undefined)

  const cards = $derived(
    stats
      ? [
          {
            id: 'waiting',
            label: $_('pages.dashboard.Waiting'),
            hint: $_('pages.dashboard.WaitingHint'),
            value: stats.waiting,
            color: 'secondary',
          },
          {
            id: 'queued',
            label: $_('pages.dashboard.Queued'),
            hint: $_('pages.dashboard.QueuedHint'),
            value: stats.queued,
            color: 'info',
          },
          {
            id: 'extracting',
            label: $_('pages.dashboard.Extracting'),
            hint: $_('pages.dashboard.ExtractingHint'),
            value: stats.extracting,
            color: 'primary',
          },
          {
            id: 'extracted',
            label: $_('pages.dashboard.Extracted'),
            hint: $_('pages.dashboard.ExtractedHint'),
            value: stats.extracted,
            color: 'success',
          },
          {
            id: 'imported',
            label: $_('pages.dashboard.Imported'),
            hint: $_('pages.dashboard.ImportedHint'),
            value: stats.imported,
            color: 'success',
          },
          {
            id: 'failed',
            label: $_('pages.dashboard.Failed'),
            hint: $_('pages.dashboard.FailedHint'),
            value: stats.failed,
            color: 'danger',
          },
          {
            id: 'deleted',
            label: $_('pages.dashboard.Deleted'),
            hint: $_('pages.dashboard.DeletedHint'),
            value: stats.deleted,
            color: 'dark',
          },
          {
            id: 'finished',
            label: $_('pages.dashboard.Finished'),
            hint: $_('pages.dashboard.FinishedHint'),
            value: stats.finished,
            color: 'secondary',
          },
        ]
      : [],
  )

  type SideRow = {
    id: string
    label: string
    hint: string
    value: string
    fail?: number
    full?: boolean
  }

  function stackValue(buf?: BufferStat): string {
    return `${buf?.len ?? 0} / ${buf?.cap ?? 0}`
  }

  function stackFull(buf?: BufferStat): boolean {
    return (buf?.cap ?? 0) > 0 && (buf?.len ?? 0) >= (buf?.cap ?? 0)
  }

  function stackRow(
    id: string,
    label: string,
    hint: string,
    buf?: BufferStat,
  ): SideRow {
    return { id, label, hint, value: stackValue(buf), full: stackFull(buf) }
  }

  const side = $derived.by((): SideRow[] => {
    if (!stats) return []

    const rows: SideRow[] = []
    if (stats.starrs) {
      rows.push({
        id: 'starrs',
        label: $_('pages.dashboard.Starrs'),
        hint: $_('pages.dashboard.StarrsHint'),
        value: String(stats.starrs),
      })
    }
    if (stats.folders) {
      rows.push({
        id: 'folders',
        label: $_('pages.dashboard.Folders'),
        hint: $_('pages.dashboard.FoldersHint'),
        value: String(stats.folders),
      })
    }
    if (stats.webhooks) {
      rows.push({
        id: 'webhooks',
        label: $_('pages.dashboard.Webhooks'),
        hint: $_('pages.dashboard.WebhooksHint'),
        value: String(stats.hookOK ?? 0),
        fail: stats.hookFail ?? 0,
      })
    }
    if (stats.cmdhooks) {
      rows.push({
        id: 'cmdhooks',
        label: $_('pages.dashboard.Cmdhooks'),
        hint: $_('pages.dashboard.CmdhooksHint'),
        value: String(stats.cmdOK ?? 0),
        fail: stats.cmdFail ?? 0,
      })
    }
    if (stats.folders) {
      rows.push(
        stackRow(
          'stack-fs',
          $_('pages.dashboard.StackFS'),
          $_('pages.dashboard.StackFSHint'),
          stats.stackFS,
        ),
        stackRow(
          'stack-folder',
          $_('pages.dashboard.StackFolder'),
          $_('pages.dashboard.StackFolderHint'),
          stats.stackFolder,
        ),
      )
    }
    if (stats.starrs) {
      rows.push(
        stackRow(
          'stack-xtractr',
          $_('pages.dashboard.StackXtractr'),
          $_('pages.dashboard.StackXtractrHint'),
          stats.stackXtractr,
        ),
      )
    }
    if (stats.webhooks || stats.cmdhooks) {
      rows.push(
        stackRow(
          'stack-hook',
          $_('pages.dashboard.StackHook'),
          $_('pages.dashboard.StackHookHint'),
          stats.stackHook,
        ),
      )
    }
    rows.push(
      stackRow(
        'stack-del',
        $_('pages.dashboard.StackDel'),
        $_('pages.dashboard.StackDelHint'),
        stats.stackDel,
      ),
      stackRow(
        'stack-task',
        $_('pages.dashboard.StackTask'),
        $_('pages.dashboard.StackTaskHint'),
        stats.stackTask,
      ),
    )
    return rows
  })

  const starrQueues = $derived(stats?.starrQueues ?? [])

  function showBar(item: QueueItem): boolean {
    return (
      item.status === 'extracting' &&
      !!(
        item.percent ||
        item.total ||
        item.compressed ||
        item.wrote ||
        item.read ||
        item.archive ||
        item.archives
      )
    )
  }

  const dueKeys: Record<string, string> = {
    start: 'pages.dashboard.DueStart',
    retry: 'pages.dashboard.DueRetry',
    cleanup: 'pages.dashboard.DueCleanup',
    history: 'pages.dashboard.DueHistory',
  }

  function dueLabel(item: QueueItem, clock: number): string {
    if (isZeroTime(item.due) || !item.dueKind) return ''
    const key = dueKeys[item.dueKind]
    if (!key) return ''
    const remain = remainCompact(item.due, clock)
    if (!remain) {
      if (item.note) return ''
      return $_('pages.dashboard.DueSoon')
    }
    return $_(key, { values: { remain } })
  }

  function isFolderWait(item: QueueItem): boolean {
    return item.status === 'waiting' && !!item.event
  }

  function updatedAge(item: QueueItem, clock: number): string {
    const ago = relTime(item.updated, clock)
    if (isFolderWait(item)) {
      return `${$_('pages.dashboard.LastWrite')} ${ago}`
    }
    return ago
  }

  function extractingBytes(item: QueueItem): string {
    const max = item.total || item.compressed || 0
    if (max || item.percent) return progressCaption(item)
    return ''
  }

  function extractingSpeeds(item: QueueItem): string {
    const parts: string[] = []
    if (item.avgSpeedBps) {
      parts.push(
        $_('pages.dashboard.SpeedAvg', {
          values: { speed: bytes(item.avgSpeedBps) },
        }),
      )
    }
    if (item.speedBps) {
      parts.push(
        $_('pages.dashboard.SpeedNow', {
          values: { speed: bytes(item.speedBps) },
        }),
      )
    }
    return parts.join(' · ')
  }

  function extractingEta(item: QueueItem, clock: number): string {
    if (isZeroTime(item.eta)) return ''
    const remain = remainCompact(item.eta, clock)
    return remain
      ? $_('pages.dashboard.ETA', { values: { remain } })
      : $_('pages.dashboard.DueSoon')
  }

  function extractingCaption(item: QueueItem, clock: number): string {
    return [
      extractingBytes(item),
      extractingSpeeds(item),
      extractingEta(item, clock),
    ]
      .filter(Boolean)
      .join(' · ')
  }

  function archiveCount(item: QueueItem): string {
    if (!item.archives) return ''
    const n = (item.extracted ?? 0) + 1
    return $_('pages.dashboard.ArchiveOf', {
      values: { n, total: item.archives },
    })
  }

  function queueMeta(item: QueueItem, clock: number): string {
    const parts = [
      `${$_('pages.dashboard.Retries')} ${item.retries}`,
      isFolderWait(item)
        ? `${$_('pages.dashboard.LastWrite')} ${relTime(item.updated, clock)}`
        : `${$_('pages.dashboard.Updated')} ${relTime(item.updated, clock)}`,
    ]
    const due = dueLabel(item, clock)
    if (due) parts.push(due)
    return `${parts.join(' · ')}.`
  }

  async function retry(item: QueueItem) {
    busy[item.id] = true
    const res = await api.post('queue/retry', { id: item.id })
    busy[item.id] = false
    if (res.ok)
      success($_('pages.dashboard.Retrying', { values: { id: item.id } }))
    else failure(res.body?.error ?? 'retry failed')
  }

  function wantsCleanupWarning(item: QueueItem): boolean {
    return item.status.toLowerCase() === 'imported'
  }

  const detailItem = $derived.by(() => {
    if (!pendingDetail) return null
    const liveRow =
      live.queue.find((row) => row.id === pendingDetail?.id) ?? pendingDetail
    return {
      ...liveRow,
      newFiles: detailFiles?.newFiles ?? liveRow.newFiles,
      origFiles: detailFiles?.origFiles ?? liveRow.origFiles,
      output: detailFiles?.output ?? liveRow.output,
      ids: liveRow.ids ?? detailFiles?.ids,
    }
  })

  async function fetchDetail(id: string) {
    const res = await api.get<QueueItem>(
      'queue/item?id=' + encodeURIComponent(id),
    )
    if (res.ok && pendingDetail?.id === id) detailFiles = res.body
  }

  async function openDetail(item: QueueItem) {
    pendingDetail = item
    detailFiles = null
    detailFilesReady = statusHasExtractResult(item.status)
    await fetchDetail(item.id)
  }

  function closeDetail() {
    pendingDetail = null
    detailFiles = null
    detailFilesReady = false
  }

  // One-shot: live progress omits file lists, so refetch when extract finishes.
  $effect(() => {
    const open = pendingDetail
    if (!open || detailFilesReady) return
    const status = live.queue.find((row) => row.id === open.id)?.status
    if (!status || !statusHasExtractResult(status)) return
    detailFilesReady = true
    void fetchDetail(open.id)
  })

  function forget(item: QueueItem) {
    if (wantsCleanupWarning(item)) {
      pendingForget = item
      return
    }
    void runForget(item)
  }

  function cancelForget() {
    pendingForget = null
  }

  function confirmForget() {
    const item = pendingForget
    pendingForget = null
    if (item) void runForget(item)
  }

  async function runForget(item: QueueItem) {
    busy[item.id] = true
    const res = await api.post('queue/forget', { id: item.id })
    busy[item.id] = false
    if (res.ok)
      success($_('pages.dashboard.Forgotten', { values: { id: item.id } }))
    else failure(res.body?.error ?? 'forget failed')
  }

  onMount(() => {
    const topics: LiveTopic[] = []
    if (canQueue) {
      topics.push('queue')
      topics.push('progress')
    }
    if (canHistory) topics.push('history')
    if (topics.length) live.subscribe(topics)
    void live.restSnapshot()
    ageTimer = setInterval(() => {
      now = Date.now()
    }, 1000)
    return () => {
      if (topics.length) live.unsubscribe(topics)
    }
  })
  onDestroy(() => {
    if (ageTimer) clearInterval(ageTimer)
  })
</script>

<h4 class="mb-2">{$_('pages.dashboard.Title')}</h4>
<PageIntro id="pages.dashboard" />

{#if (canStats || canQueue) && loading}
  <Spinner color="primary" />
{:else}
  {#if canStats && stats}
    <Row class="g-2 mb-3 align-items-stretch">
      <Col xs="12" md="8">
        <Row class="g-2">
          {#each cards as c (c.id)}
            <Col xs="6" sm="3">
              <Card id="{uid}-{c.id}" class="stat-card text-center h-100">
                <CardBody class="py-2 px-1">
                  <div class="stat-value text-{c.color}">{c.value}</div>
                  <div class="text-muted small text-uppercase">{c.label}</div>
                </CardBody>
              </Card>
              <Tooltip target="{uid}-{c.id}" placement="top" theme={theme.tooltip}>{c.hint}</Tooltip>
            </Col>
          {/each}
        </Row>
      </Col>
      <Col xs="12" md="4">
        <Card class="stat-side h-100">
          <CardBody class="py-2 px-2">
            <Table size="sm" striped borderless class="stat-side-table mb-0">
              <tbody>
                {#each side as row (row.id)}
                  <tr>
                    <th id="{uid}-{row.id}" scope="row">{row.label}</th>
                    <td class="text-end" class:text-danger={row.full}>
                      {#if row.fail === undefined}
                        {row.value}
                      {:else}
                        {row.value} /
                        <span class={row.fail > 0 ? 'text-danger' : ''}
                          >{row.fail}</span
                        >
                      {/if}
                    </td>
                  </tr>
                  <Tooltip target="{uid}-{row.id}" placement="top" theme={theme.tooltip}
                    >{row.hint}</Tooltip
                  >
                {/each}
              </tbody>
            </Table>
          </CardBody>
        </Card>
      </Col>
    </Row>
  {/if}

  {#if canStats && starrQueues.length}
    <Card class="mb-3">
      <CardBody class="py-2 px-2">
        <CardTitle class="mb-2" id="{uid}-starr-queues"
          >{$_('pages.dashboard.StarrQueues')}</CardTitle
        >
        <Tooltip target="{uid}-starr-queues" placement="top" theme={theme.tooltip}
          >{$_('pages.dashboard.StarrQueuesHint')}</Tooltip
        >
        <Table size="sm" striped borderless class="stat-side-table mb-0">
          <thead>
            <tr>
              <th scope="col">{$_('pages.dashboard.App')}</th>
              <th id="{uid}-starrq-h-total" class="text-end" scope="col"
                >{$_('pages.dashboard.StarrQueueTotal')}</th
              >
              <th id="{uid}-starrq-h-complete" class="text-end" scope="col"
                >{$_('pages.dashboard.StarrQueueComplete')}</th
              >
              <th id="{uid}-starrq-h-match" class="text-end" scope="col"
                >{$_('pages.dashboard.StarrQueueMatch')}</th
              >
              <th id="{uid}-starrq-h-issues" class="text-end" scope="col"
                >{$_('pages.dashboard.StarrQueueIssues')}</th
              >
              <th id="{uid}-starrq-h-dl" class="text-end" scope="col"
                >{$_('pages.dashboard.StarrQueueDownloading')}</th
              >
            </tr>
          </thead>
          <tbody>
            {#each starrQueues as row, i (`${row.app}-${row.url}-${i}`)}
              <tr>
                <th id="{uid}-starrq-{i}" scope="row">{row.name}</th>
                <td class="text-end" class:text-danger={!!row.error}>
                  {#if row.queued !== row.retrieved}
                    {row.queued} / {row.retrieved}
                  {:else}
                    {row.queued}
                  {/if}
                </td>
                <td class="text-end">{row.complete ?? 0}</td>
                <td class="text-end">{row.match ?? 0}</td>
                <td class="text-end" class:text-danger={(row.issues ?? 0) > 0}
                  >{row.issues ?? 0}</td
                >
                <td class="text-end">{row.downloading ?? 0}</td>
              </tr>
              <Tooltip target="{uid}-starrq-{i}" placement="top" theme={theme.tooltip}>
                {row.error || row.url || $_('pages.dashboard.StarrQueuesHint')}
              </Tooltip>
            {/each}
          </tbody>
        </Table>
        <Tooltip target="{uid}-starrq-h-total" placement="top" theme={theme.tooltip}
          >{$_('pages.dashboard.StarrQueueTotalHint')}</Tooltip
        >
        <Tooltip target="{uid}-starrq-h-complete" placement="top" theme={theme.tooltip}
          >{$_('pages.dashboard.StarrQueueCompleteHint')}</Tooltip
        >
        <Tooltip target="{uid}-starrq-h-match" placement="top" theme={theme.tooltip}
          >{$_('pages.dashboard.StarrQueueMatchHint')}</Tooltip
        >
        <Tooltip target="{uid}-starrq-h-issues" placement="top" theme={theme.tooltip}
          >{$_('pages.dashboard.StarrQueueIssuesHint')}</Tooltip
        >
        <Tooltip target="{uid}-starrq-h-dl" placement="top" theme={theme.tooltip}
          >{$_('pages.dashboard.StarrQueueDownloadingHint')}</Tooltip
        >
      </CardBody>
    </Card>
  {/if}

  {#if canQueue}
    <Card>
      <CardBody>
        <Row class="align-items-center mb-2">
          <Col>
            <CardTitle class="mb-0"
              >{$_('pages.dashboard.ActiveQueue')}</CardTitle
            >
          </Col>
          <Col xs="auto" class="d-flex align-items-center gap-2 flex-wrap">
            {#if live.connected}
              <Badge color="success">{$_('pages.dashboard.Live')}</Badge>
            {:else if dataAge}
              <span class="small text-muted text-nowrap">
                {$_('pages.dashboard.UpdatedAgo', {
                  values: { age: dataAge },
                })}
              </span>
            {/if}
          </Col>
        </Row>
        {#if queue.length === 0}
          <p class="text-muted mb-0">{$_('phrases.NothingQueued')}</p>
        {:else}
          <Table hover size="sm" class="align-middle queue-table">
            <thead>
              <tr>
                <th class="queue-app">{$_('pages.dashboard.App')}</th>
                <th class="queue-status">{$_('pages.dashboard.Status')}</th>
                <th class="queue-progress">{$_('pages.dashboard.Progress')}</th>
                <th class="text-end queue-retries">
                  {$_('pages.dashboard.Retries')}
                </th>
                <th class="queue-updated">{$_('pages.dashboard.Updated')}</th>
                <th class="text-end queue-actions">
                  {$_('pages.dashboard.Actions')}
                </th>
              </tr>
            </thead>
            {#each queue as item, i (item.id)}
              {@const due = dueLabel(item, now)}
              <tbody class="stack-item">
                <tr>
                  <td class="queue-app">{item.app}</td>
                  <td class="queue-status"
                    ><Badge color={statusColor(item.status)}
                      >{$_(statusPhrase(item.status))}</Badge
                    ></td
                  >
                  <td class="small queue-progress">
                    <div class="queue-progress-inner">
                      {#if showBar(item)}
                        {@const cap = extractingCaption(item, now)}
                        {@const bytesLine = extractingBytes(item)}
                        {@const speeds = extractingSpeeds(item)}
                        {@const eta = extractingEta(item, now)}
                        {@const archOf = archiveCount(item)}
                        {@const etaArch = [eta, archOf]
                          .filter(Boolean)
                          .join(' · ')}
                        <div class="progress mb-1">
                          <div
                            class="progress-bar"
                            style="width: {Math.min(item.percent ?? 0, 100)}%"
                          ></div>
                        </div>
                        {#if bytesLine}
                          <div class="queue-progress-caption" title={cap}>
                            {bytesLine}
                          </div>
                        {/if}
                        {#if speeds}
                          <div class="queue-progress-caption text-muted">
                            {speeds}
                          </div>
                        {/if}
                        {#if etaArch}
                          <div class="queue-progress-caption text-muted">
                            {etaArch}
                          </div>
                        {/if}
                        {#if item.archive}
                          <div
                            class="queue-progress-archive text-muted"
                            title={item.archive}
                          >
                            {item.archive}
                          </div>
                        {/if}
                      {/if}
                      {#if item.error}
                        {@const errKey = errorPhrase(item.error)}
                        <div class="queue-progress-caption text-danger">
                          {errKey ? $_(errKey) : item.error}
                        </div>
                      {:else if !showBar(item) && isFolderWait(item) && item.note}
                        <div
                          class="queue-progress-caption text-muted"
                          title={item.note}
                        >
                          {$_('pages.dashboard.WaitFile', {
                            values: { file: item.note },
                          })}
                        </div>
                      {:else if !showBar(item) && !isFolderWait(item)}
                        {@const cap = progressCaption(item)}
                        {#if cap}
                          {@const capKey = errorPhrase(cap)}
                          <div
                            class="queue-progress-caption text-muted"
                            title={item.progress || ''}
                          >
                            {capKey ? $_(capKey) : cap}
                          </div>
                        {/if}
                      {/if}
                    </div>
                  </td>
                  <td class="text-end queue-retries">{item.retries}</td>
                  <td class="small queue-updated">
                    <div class="queue-updated-inner">
                      <div class="text-nowrap">{updatedAge(item, now)}</div>
                      {#if due}
                        <div class="text-nowrap text-muted" title={item.due}>
                          {due}
                        </div>
                      {/if}
                    </div>
                  </td>
                  <td class="small queue-meta">{queueMeta(item, now)}</td>
                  <td class="text-end text-nowrap queue-actions">
                    <ButtonGroup size="sm">
                      <Button
                        id="{uid}-info-{i}"
                        color="secondary"
                        outline
                        type="button"
                        aria-label={$_('pages.detail.Info')}
                        aria-haspopup="dialog"
                        onclick={() => openDetail(item)}
                      >
                        <InfoIcon
                          size="1.25em"
                          weight="bold"
                          aria-hidden="true"
                          focusable="false"
                        />
                      </Button>
                      {#if canWrite && (item.status === 'extractfailed' || TERMINAL.includes(item.status))}
                        {#if item.status === 'extractfailed'}
                          <Button
                            color="primary"
                            outline
                            disabled={busy[item.id]}
                            onclick={() => retry(item)}
                            >{$_('buttons.Retry')}</Button
                          >
                        {/if}
                        {#if TERMINAL.includes(item.status)}
                          <Button
                            type="button"
                            color="secondary"
                            outline
                            disabled={busy[item.id]}
                            onclick={() => forget(item)}
                            >{$_('buttons.Forget')}</Button
                          >
                        {/if}
                      {/if}
                    </ButtonGroup>
                    <Tooltip
                      target="{uid}-info-{i}"
                      placement="left"
                      theme={theme.tooltip}
                    >
                      {$_('pages.detail.Info')}
                    </Tooltip>
                  </td>
                </tr>
                <tr class="stack-item-path">
                  <td colspan="7">
                    <code class="wrap small queue-path">
                      <button
                        type="button"
                        class="queue-path-btn"
                        title={$_('pages.detail.OpenItem', {
                          values: { id: item.id },
                        })}
                        aria-label={$_('pages.detail.OpenItem', {
                          values: { id: item.id },
                        })}
                        aria-haspopup="dialog"
                        onclick={() => openDetail(item)}
                      >
                        {item.id}
                      </button>{#if item.event}<Badge
                          id="{uid}-event-{i}"
                          class="queue-event-badge"
                          color={item.event === 'fsnotify'
                            ? 'success'
                            : 'warning'}
                        >
                          {item.event === 'fsnotify'
                            ? $_('pages.dashboard.FSNotify')
                            : $_('pages.dashboard.Polling')}
                        </Badge>
                        <Tooltip target="{uid}-event-{i}" placement="top" theme={theme.tooltip}>
                          {item.event === 'fsnotify'
                            ? $_('pages.dashboard.FSNotifyHint')
                            : $_('pages.dashboard.PollingHint')}
                        </Tooltip>{/if}
                    </code>
                  </td>
                </tr>
              </tbody>
            {/each}
          </Table>
        {/if}
      </CardBody>
    </Card>
  {/if}
{/if}

{#if canHistory}
  <div class="mt-4">
    <History {now} />
  </div>
{/if}

<Modal isOpen={pendingForget !== null} toggle={cancelForget}>
  <ModalHeader toggle={cancelForget}
    >{$_('phrases.ForgetImportedTitle')}</ModalHeader
  >
  <ModalBody>{$_('phrases.ForgetImportedConfirm')}</ModalBody>
  <ModalFooter>
    <Button color="secondary" type="button" onclick={cancelForget}
      >{$_('buttons.Cancel')}</Button
    >
    <Button color="danger" type="button" onclick={confirmForget}
      >{$_('buttons.Forget')}</Button
    >
  </ModalFooter>
</Modal>

<ItemDetail item={detailItem} {now} onclose={closeDetail} />
