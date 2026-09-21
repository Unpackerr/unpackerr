<script lang="ts">
  import {
    Badge,
    Button,
    Modal,
    ModalBody,
    ModalFooter,
    ModalHeader,
    Tooltip,
  } from '@sveltestrap/sveltestrap'
  import { _ } from '../lib/i18n/Translate.svelte'
  import {
    bytes,
    dateTime,
    errorPhrase,
    isZeroTime,
    remainCompact,
    statusColor,
    statusPhrase,
  } from '../lib/format'
  import type { ItemMeta } from '../lib/types'
  import { slugifyPath } from '../lib/slug'
  import DownloadIcon from 'phosphor-svelte/lib/Download'
  import { theme } from '../lib/theme.svelte'

  let {
    item = null,
    now = Date.now(),
    onclose,
  }: {
    item: ItemMeta | null
    now?: number
    onclose: () => void
  } = $props()

  const uid = $props.id()

  const dueKeys: Record<string, string> = {
    start: 'pages.dashboard.DueStart',
    retry: 'pages.dashboard.DueRetry',
    cleanup: 'pages.dashboard.DueCleanup',
    history: 'pages.dashboard.DueHistory',
  }

  const ID_LABEL: Record<string, string> = {
    title: 'pages.detail.Title',
    downloadId: 'pages.detail.DownloadId',
    seriesId: 'pages.detail.SeriesId',
    episodeId: 'pages.detail.EpisodeId',
    movieId: 'pages.detail.MovieId',
    artistId: 'pages.detail.ArtistId',
    albumId: 'pages.detail.AlbumId',
    authorId: 'pages.detail.AuthorId',
    bookId: 'pages.detail.BookId',
    reason: 'pages.detail.Reason',
  }

  const idRows = $derived.by(() => {
    if (!item?.ids) return []

    const path = item.path || item.id
    const folder = item.app === 'Folder'

    const rows = Object.entries(item.ids).filter(([key, value]) => {
      if (value === undefined || value === null || value === '') return false
      // Folder items copy the path into ids.title for webhooks; skip the duplicate.
      if (key === 'title' && (folder || String(value) === path)) return false
      return true
    })

    const title = rows.find(([key]) => key === 'title')
    if (!title) return rows

    return [title, ...rows.filter(([key]) => key !== 'title')]
  })

  function idLabel(key: string): string {
    const loc = ID_LABEL[key]
    return loc ? $_(loc) : key
  }

  const due = $derived(item ? dueText(item, now) : '')
  const extraFiles = $derived(
    item && 'extraFiles' in item ? (item.extraFiles ?? []) : [],
  )
  const origFiles = $derived([...(item?.origFiles ?? []), ...extraFiles])
  const newFiles = $derived(item?.newFiles ?? [])
  const hasOrig = $derived(origFiles.length > 0)
  const hasNew = $derived(newFiles.length > 0)

  function dueText(row: ItemMeta, clock: number): string {
    if (!('dueKind' in row) || !row.dueKind || isZeroTime(row.due)) return ''

    const key = dueKeys[row.dueKind]
    if (!key) return ''

    const remain = remainCompact(row.due, clock)
    if (!remain) return $_('pages.dashboard.DueSoon')

    return $_(key, { values: { remain } })
  }

  function eventLabel(event: string): string {
    if (event === 'fsnotify') return $_('pages.dashboard.FSNotify')
    if (event === 'polling') return $_('pages.dashboard.Polling')

    return event
  }

  function eventHint(event: string): string {
    if (event === 'fsnotify') return $_('pages.dashboard.FSNotifyHint')
    if (event === 'polling') return $_('pages.dashboard.PollingHint')

    return event
  }

  function downloadFiles(
    files: string[] | undefined,
    kind: 'archives' | 'extracted',
  ) {
    if (!files?.length) return
    const base = slugifyPath(item?.path || item?.id || '') || 'item'
    const blob = new Blob([files.join('\n') + '\n'], {
      type: 'text/plain;charset=utf-8',
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${base}-${kind}.txt`
    a.rel = 'noopener'

    document.body.append(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  }
</script>

<Modal
  isOpen={item !== null}
  toggle={onclose}
  size="xl"
  class="item-detail-modal"
  labelledBy="item-detail-title"
>
  {#if item}
    <ModalHeader toggle={onclose} id="item-detail-title">
      <span class="item-detail-title">
        <span class="d-inline-flex align-items-center gap-2">
          <span>{item.app}</span>
          <Badge color={statusColor(item.status)}>
            {$_(statusPhrase(item.status))}
          </Badge>
          <span class="visually-hidden">{item.path || item.id}</span>
        </span>
        {#if item.event}
          <Badge
            id="{uid}-event"
            class="item-detail-event"
            color={item.event === 'fsnotify' ? 'success' : 'warning'}
          >
            {eventLabel(item.event)}
          </Badge>
          <Tooltip target="{uid}-event" placement="left" theme={theme.tooltip}>
            {eventHint(item.event)}
          </Tooltip>
        {/if}
      </span>
    </ModalHeader>

    <ModalBody>
      <dl class="item-detail">
        {#if item.path || item.id}
          <dt>{$_('pages.detail.Path')}</dt>
          <dd><code class="wrap">{item.path || item.id}</code></dd>
        {/if}
        {#if item.outputPath && item.outputPath !== item.path}
          <dt>{$_('pages.detail.OutputPath')}</dt>
          <dd><code class="wrap">{item.outputPath}</code></dd>
        {/if}
        {#if item.output && item.output !== item.outputPath && item.output !== item.path}
          <dt>{$_('pages.detail.Dest')}</dt>
          <dd><code class="wrap">{item.output}</code></dd>
        {/if}
        {#if item.url}
          <dt>{$_('pages.detail.URL')}</dt>
          <dd><code class="wrap">{item.url}</code></dd>
        {/if}
        {#if item.kind && item.kind !== item.app}
          <dt>{$_('pages.detail.Kind')}</dt>
          <dd>{item.kind}</dd>
        {/if}
        {#if item.started && !isZeroTime(item.started)}
          <dt>{$_('pages.detail.Started')}</dt>
          <dd>{dateTime(item.started)}</dd>
        {/if}
        {#if item.elapsed}
          <dt>{$_('pages.detail.Elapsed')}</dt>
          <dd>{item.elapsed}</dd>
        {/if}
        {#if 'finished' in item && item.finished && !isZeroTime(item.finished)}
          <dt>{$_('pages.detail.Finished')}</dt>
          <dd>{dateTime(item.finished)}</dd>
        {/if}
        {#if item.updated && !isZeroTime(item.updated)}
          <dt>{$_('pages.detail.Updated')}</dt>
          <dd>{dateTime(item.updated)}</dd>
        {/if}
        {#if due}
          <dt>{$_('pages.detail.Due')}</dt>
          <dd>{due}</dd>
        {/if}
        {#if 'deleteDelay' in item && item.deleteDelay}
          <dt>{$_('pages.detail.DeleteDelay')}</dt>
          <dd>{item.deleteDelay}</dd>
        {/if}
        {#if item.bytes}
          <dt>{$_('pages.detail.Size')}</dt>
          <dd>{bytes(item.bytes)}</dd>
        {/if}
        {#if item.ratio}
          <dt>{$_('pages.detail.Ratio')}</dt>
          <dd>{item.ratio.toFixed(2)}</dd>
        {/if}
        {#if item.archives || ('extracted' in item && item.extracted)}
          <dt>{$_('pages.detail.Archives')}</dt>
          <dd>
            {#if 'extracted' in item && item.extracted && item.archives}
              {$_('pages.dashboard.ArchiveOf', {
                values: { n: item.extracted, total: item.archives },
              })}
            {:else}
              {item.archives || 0}
            {/if}
          </dd>
        {/if}
        {#if 'archive' in item && item.archive}
          <dt>{$_('pages.detail.Archive')}</dt>
          <dd><code class="wrap">{item.archive}</code></dd>
        {/if}
        {#if item.files}
          <dt>{$_('pages.detail.Files')}</dt>
          <dd>{item.files}</dd>
        {/if}
        {#if item.queue}
          <dt>{$_('pages.detail.Queue')}</dt>
          <dd>{item.queue}</dd>
        {/if}
        {#if item.retries}
          <dt>{$_('pages.detail.Retries')}</dt>
          <dd>{item.retries}</dd>
        {/if}
        {#if item.hookFail}
          <dt>{$_('pages.detail.HookFail')}</dt>
          <dd>{item.hookFail}</dd>
        {/if}
        {#if 'note' in item && item.note}
          <dt>{$_('pages.detail.Note')}</dt>
          <dd>{item.note}</dd>
        {/if}
        {#if item.error}
          {@const errKey = errorPhrase(item.error)}
          <dt>{$_('pages.detail.Error')}</dt>
          <dd class="text-danger">{errKey ? $_(errKey) : item.error}</dd>
        {/if}
        {#each idRows as [key, value] (key)}
          <dt>{idLabel(key)}</dt>
          <dd><code class="wrap">{String(value)}</code></dd>
        {/each}
      </dl>
      {#if hasOrig || hasNew}
        <div
          class="item-file-panes"
          class:item-file-panes-single={!hasOrig || !hasNew}
        >
          {#if hasOrig}
            <section class="item-file-pane">
              <div class="item-file-pane-head">
                <h6 id="item-files-orig">
                  {$_('pages.detail.ArchiveFiles')} ({origFiles.length})
                </h6>
                <Button
                  id="{uid}-dl-orig"
                  size="sm"
                  color="secondary"
                  outline
                  type="button"
                  class="item-file-download"
                  aria-label={$_('pages.detail.DownloadList', {
                    values: { name: $_('pages.detail.ArchiveFiles') },
                  })}
                  onclick={() => downloadFiles(origFiles, 'archives')}
                >
                  <DownloadIcon
                    size="1.25em"
                    weight="bold"
                    aria-hidden="true"
                    focusable="false"
                  />
                </Button>
                <Tooltip
                  target="{uid}-dl-orig"
                  placement="top"
                  theme={theme.tooltip}
                >
                  {$_('pages.detail.DownloadList', {
                    values: { name: $_('pages.detail.ArchiveFiles') },
                  })}
                </Tooltip>
              </div>
              <ul class="item-file-list" aria-labelledby="item-files-orig">
                {#each origFiles as file (file)}
                  <li><code>{file}</code></li>
                {/each}
              </ul>
            </section>
          {/if}
          {#if hasNew}
            <section class="item-file-pane">
              <div class="item-file-pane-head">
                <h6 id="item-files-new">
                  {$_('pages.detail.ExtractedFiles')} ({newFiles.length})
                </h6>
                <Button
                  id="{uid}-dl-new"
                  size="sm"
                  color="secondary"
                  outline
                  type="button"
                  class="item-file-download"
                  aria-label={$_('pages.detail.DownloadList', {
                    values: { name: $_('pages.detail.ExtractedFiles') },
                  })}
                  onclick={() => downloadFiles(newFiles, 'extracted')}
                >
                  <DownloadIcon
                    size="1.25em"
                    weight="bold"
                    aria-hidden="true"
                    focusable="false"
                  />
                </Button>
                <Tooltip
                  target="{uid}-dl-new"
                  placement="top"
                  theme={theme.tooltip}
                >
                  {$_('pages.detail.DownloadList', {
                    values: { name: $_('pages.detail.ExtractedFiles') },
                  })}
                </Tooltip>
              </div>
              <ul class="item-file-list" aria-labelledby="item-files-new">
                {#each newFiles as file (file)}
                  <li><code>{file}</code></li>
                {/each}
              </ul>
            </section>
          {/if}
        </div>
      {/if}
    </ModalBody>

    <ModalFooter>
      <Button color="secondary" type="button" onclick={onclose}>
        {$_('buttons.Close')}
      </Button>
    </ModalFooter>
  {/if}
</Modal>
