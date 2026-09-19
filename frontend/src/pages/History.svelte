<script lang="ts">
  import { onMount } from 'svelte'
  import {
    Badge,
    Button,
    ButtonGroup,
    Card,
    CardBody,
    CardTitle,
    Col,
    Input,
    Row,
    Modal,
    ModalBody,
    ModalFooter,
    ModalHeader,
    Spinner,
    Table,
    Tooltip,
  } from '@sveltestrap/sveltestrap'
  import { _ } from '../lib/i18n/Translate.svelte'
  import { api } from '../lib/api'
  import { has } from '../lib/auth.svelte'
  import { systemPerm } from '../lib/perms'
  import {
    statusColor,
    statusPhrase,
    errorPhrase,
    bytes,
    dateTime,
    relTime,
    isZeroTime,
    isFinishedHistory,
  } from '../lib/format'
  import { success, failure } from '../lib/toast'
  import type { HistoryRecord } from '../lib/types'
  import { live } from '../lib/socket.svelte'
  import ItemDetail from '../components/ItemDetail.svelte'
  import InfoIcon from 'phosphor-svelte/lib/Info'
  import { theme } from '../lib/theme.svelte'

  let { now = Date.now() }: { now?: number } = $props()

  const uid = $props.id()

  let filter = $state('')
  let busy = $state<Record<string, boolean>>({})
  let loaded = $state(false)
  let pendingClear = $state(false)
  let pendingDetail = $state<HistoryRecord | null>(null)

  const detailItem = $derived(
    pendingDetail
      ? (live.history.find((row) => row.id === pendingDetail?.id) ??
          pendingDetail)
      : null,
  )

  const canWrite = has(systemPerm('history', 'write'))
  const rows = $derived(live.history.filter(isFinishedHistory))
  const loading = $derived(!loaded && rows.length === 0)

  const shown = $derived(
    filter.trim()
      ? rows.filter((r) =>
          (r.id + r.app + r.status)
            .toLowerCase()
            .includes(filter.trim().toLowerCase()),
        )
      : rows,
  )

  async function refresh() {
    const before = live.history
    const res = await api.get<HistoryRecord[]>('history')
    if (res.ok && live.history === before) {
      live.history = (res.body ?? []).filter(isFinishedHistory)
    }
    loaded = true
  }

  async function remove(row: HistoryRecord) {
    busy[row.id] = true
    const res = await api.post('history/delete', { id: row.id })
    busy[row.id] = false
    if (res.ok) {
      success($_('pages.history.DeletedRow'))
    } else failure(res.body?.error ?? 'delete failed')
  }

  function askClearAll() {
    pendingClear = true
  }

  function cancelClear() {
    pendingClear = false
  }

  async function confirmClear() {
    pendingClear = false
    const res = await api.post('history/clear', {})
    if (res.ok) {
      success($_('pages.history.Cleared'))
    } else failure(res.body?.error ?? 'clear failed')
  }

  onMount(refresh)
</script>

<Card>
  <CardBody>
    <Row class="align-items-center mb-2 g-2">
      <Col>
        <CardTitle class="mb-0">{$_('pages.history.Title')}</CardTitle>
      </Col>
      <Col xs="auto">
        <Input
          id="history-filter"
          name="history-filter"
          type="text"
          bsSize="sm"
          aria-label={$_('phrases.FilterPlaceholder')}
          placeholder={$_('phrases.FilterPlaceholder')}
          bind:value={filter}
          style="width: 12rem"
        />
      </Col>
      <Col xs="auto">
        <ButtonGroup size="sm">
          <Button color="secondary" outline onclick={refresh}
            >{$_('buttons.Refresh')}</Button
          >
          {#if canWrite}
            <Button
              color="danger"
              outline
              type="button"
              onclick={askClearAll}
              disabled={rows.length === 0}
            >
              {$_('buttons.ClearAll')}
            </Button>
          {/if}
        </ButtonGroup>
      </Col>
    </Row>
    {#if loading}
      <Spinner color="primary" />
    {:else if shown.length === 0}
      <p class="text-muted mb-0">{$_('phrases.NoHistory')}</p>
    {:else}
      <Table responsive hover size="sm" class="align-middle">
        <thead>
          <tr>
            <th id="hist-app" scope="col">{$_('pages.history.App')}</th>
            <th id="hist-status" scope="col">{$_('pages.history.Status')}</th>
            <th id="hist-files" class="text-end" scope="col"
              >{$_('pages.history.Files')}</th
            >
            <th id="hist-size" class="text-end" scope="col"
              >{$_('pages.history.Size')}</th
            >
            <th id="hist-retries" class="text-end" scope="col"
              >{$_('pages.history.Retries')}</th
            >
            <th id="hist-finished" scope="col">
              {$_('pages.history.Finished')}
            </th>
            <th id="hist-actions" class="text-end" scope="col">
              {$_('pages.history.Actions')}
            </th>
          </tr>
        </thead>
        {#each shown as row, i (row.id)}
          <tbody class="stack-item">
            <tr>
              <td headers="hist-app">{row.app}</td>
              <td headers="hist-status"
                ><Badge color={statusColor(row.status)}
                  >{$_(statusPhrase(row.status))}</Badge
                ></td
              >
              <td class="text-end" headers="hist-files">{row.files || 0}</td>
              <td class="text-end text-nowrap" headers="hist-size"
                >{bytes(row.bytes)}</td
              >
              <td class="text-end" headers="hist-retries">{row.retries}</td>
              <td
                class="small text-nowrap"
                headers="hist-finished"
                title={dateTime(row.finished)}
              >
                {isZeroTime(row.finished)
                  ? $_('phrases.Empty')
                  : relTime(row.finished, now)}
              </td>
              <td class="text-end" headers="hist-actions">
                <ButtonGroup size="sm">
                  <Button
                    id="{uid}-info-{i}"
                    size="sm"
                    color="secondary"
                    outline
                    type="button"
                    aria-label={$_('pages.detail.Info')}
                    aria-haspopup="dialog"
                    onclick={() => (pendingDetail = row)}
                  >
                    <InfoIcon
                      size="1.25em"
                      weight="bold"
                      aria-hidden="true"
                      focusable="false"
                    />
                  </Button>
                  {#if canWrite}
                    <Button
                      size="sm"
                      color="secondary"
                      outline
                      disabled={busy[row.id]}
                      onclick={() => remove(row)}>{$_('buttons.Delete')}</Button
                    >
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
              <td colspan="7" headers="hist-app">
                <code class="wrap small">
                  <button
                    type="button"
                    class="queue-path-btn"
                    title={$_('pages.detail.OpenItem', {
                      values: { id: row.id },
                    })}
                    aria-label={$_('pages.detail.OpenItem', {
                      values: { id: row.id },
                    })}
                    aria-haspopup="dialog"
                    onclick={() => (pendingDetail = row)}
                  >
                    {row.id}
                  </button>
                </code>
                {#if row.error}
                  {@const errKey = errorPhrase(row.error)}
                  <div class="text-danger small">
                    {errKey ? $_(errKey) : row.error}
                  </div>
                {/if}
              </td>
            </tr>
          </tbody>
        {/each}
      </Table>
    {/if}
  </CardBody>
</Card>

<Modal isOpen={pendingClear} toggle={cancelClear}>
  <ModalHeader toggle={cancelClear}>
    {$_('phrases.ClearHistoryTitle')}
  </ModalHeader>
  <ModalBody>{$_('phrases.ClearHistoryConfirm')}</ModalBody>
  <ModalFooter>
    <Button color="secondary" type="button" onclick={cancelClear}
      >{$_('buttons.Cancel')}</Button
    >
    <Button color="danger" type="button" onclick={confirmClear}
      >{$_('buttons.ClearAll')}</Button
    >
  </ModalFooter>
</Modal>

<ItemDetail item={detailItem} {now} onclose={() => (pendingDetail = null)} />
