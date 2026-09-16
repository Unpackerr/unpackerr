<script lang="ts">
  import { Button, Table } from '@sveltestrap/sveltestrap'
  import type { LogFileInfo } from '../../lib/types'
  import { _ } from '../../lib/i18n/Translate.svelte'
  import { bytes, dateTime } from '../../lib/format'
  import { getUrlbase } from '../../lib/api'
  import { profile } from '../../lib/auth.svelte'
  import Icon from '../../components/Icon.svelte'
  import CloudArrowDown from 'phosphor-svelte/lib/CloudArrowDown'

  let { file }: { file: LogFileInfo } = $props()

  const isWindows = $derived(profile.info?.goos === 'windows')

  const downloadHref = $derived(
    getUrlbase().replace(/\/+$/, '') + '/api/logs/' + file.id + '/download',
  )

  const isLive = $derived(file.id === 'live' || file.path === '(stdout)')
  const isSynthetic = $derived(
    isLive || file.id === 'errors' || file.path === '(errors)',
  )
</script>

<Table striped size="sm" class="mb-1">
  <tbody class="fit">
    <tr><th>{$_('pages.logs.Path')}</th><td><b>{file.path}</b></td></tr>
    {#if !isWindows && file.mode}
      <tr><th>{$_('pages.logs.Mode')}</th><td>{file.mode}</td></tr>
    {/if}
    {#if !isWindows && file.user}
      <tr><th>{$_('pages.logs.Owner')}</th><td>{file.user}</td></tr>
    {/if}
    <tr><th>{$_('pages.logs.Bytes')}</th><td>{bytes(file.size)}</td></tr>
    <tr><th>{$_('pages.logs.Date')}</th><td>{dateTime(file.time)}</td></tr>
    <tr><th>{$_('pages.logs.InUse')}</th><td>{file.used}</td></tr>
  </tbody>
</Table>

<div class="mt-2">
  <Button
    href={downloadHref}
    download="{file.name}.zip"
    color="primary"
    size="sm"
    outline
    disabled={isSynthetic}
  >
    <span class="d-inline-flex align-items-center gap-1">
      <Icon i={CloudArrowDown} />
      {$_('buttons.Download')}
    </span>
  </Button>
</div>

<style>
  .fit th {
    width: 1%;
    white-space: nowrap;
  }
  .fit td {
    word-break: break-all;
  }
</style>
