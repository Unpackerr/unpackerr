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
  } from '@sveltestrap/sveltestrap'
  import { _ } from '../lib/i18n/Translate.svelte'
  import { api } from '../lib/api'
  import { has } from '../lib/auth.svelte'
  import { systemPerm } from '../lib/perms'
  import {
    statusColor,
    bytes,
    dateTime,
    relTime,
    isZeroTime,
    isFinishedHistory,
  } from '../lib/format'
  import { success, failure } from '../lib/toast'
  import type { HistoryRecord } from '../lib/types'
  import { live } from '../lib/socket.svelte'

  let { now = Date.now() }: { now?: number } = $props()

  let filter = $state('')
  let busy = $state<Record<string, boolean>>({})
  let loaded = $state(false)
  let pendingClear = $state(false)

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
    const res = await api.get<HistoryRecord[]>('history')
    if (res.ok) live.history = (res.body ?? []).filter(isFinishedHistory)
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
          type="text"
          bsSize="sm"
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
            <th>{$_('pages.history.App')}</th>
            <th>{$_('pages.history.Status')}</th>
            <th class="text-end">{$_('pages.history.Files')}</th>
            <th class="text-end">{$_('pages.history.Size')}</th>
            <th class="text-end">{$_('pages.history.Retries')}</th>
            <th>{$_('pages.history.Finished')}</th>
            {#if canWrite}<th></th>{/if}
          </tr>
        </thead>
        {#each shown as row (row.id)}
          <tbody class="stack-item">
            <tr>
              <td>{row.app}</td>
              <td
                ><Badge color={statusColor(row.status)}>{row.status}</Badge></td
              >
              <td class="text-end">{row.files || 0}</td>
              <td class="text-end text-nowrap">{bytes(row.bytes)}</td>
              <td class="text-end">{row.retries}</td>
              <td class="small text-nowrap" title={dateTime(row.finished)}>
                {isZeroTime(row.finished)
                  ? $_('phrases.Empty')
                  : relTime(row.finished, now)}
              </td>
              {#if canWrite}
                <td class="text-end">
                  <Button
                    size="sm"
                    color="secondary"
                    outline
                    disabled={busy[row.id]}
                    onclick={() => remove(row)}>{$_('buttons.Delete')}</Button
                  >
                </td>
              {/if}
            </tr>
            <tr class="stack-item-path">
              <td colspan={canWrite ? 7 : 6}>
                <code class="wrap small">{row.id}</code>
                {#if row.error}<div class="text-danger small">
                    {row.error}
                  </div>{/if}
              </td>
            </tr>
          </tbody>
        {/each}
      </Table>
    {/if}
  </CardBody>
</Card>

<Modal isOpen={pendingClear} toggle={cancelClear}>
  <ModalHeader toggle={cancelClear}>{$_('phrases.ClearHistoryTitle')}</ModalHeader>
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
