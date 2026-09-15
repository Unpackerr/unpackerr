<script lang="ts">
  import { onMount } from 'svelte'
  import {
    Card,
    CardBody,
    CardTitle,
    Col,
    Row,
    Table,
  } from '@sveltestrap/sveltestrap'
  import T, { _ } from '../../lib/i18n/Translate.svelte'
  import PageIntro from '../../components/PageIntro.svelte'
  import { api } from '../../lib/api'
  import { bytes, relTime } from '../../lib/format'
  import Icon from '../../components/Icon.svelte'
  import ListBullets from 'phosphor-svelte/lib/ListBullets'
  import ArrowsClockwise from 'phosphor-svelte/lib/ArrowsClockwise'
  import FileInfo from './FileInfo.svelte'
  import Content from './Content.svelte'
  import type { LogFileInfo, LogFileInfos } from '../../lib/types'
  import { has } from '../../lib/auth.svelte'
  import { systemPerm } from '../../lib/perms'
  import { navigate, segments } from '../../lib/router.svelte'

  const canRead = has(systemPerm('logs', 'read'))

  let infos = $state<LogFileInfos | null>(null)
  let activeTail = $state<LogFileInfo | null>(null)

  const hashID = $derived(segments()[1] ?? '')
  const hashedFile = $derived(
    (infos?.list ?? []).find((f) => f.id === hashID) ?? null,
  )
  const shownTail = $derived(
    activeTail && (!hashID || activeTail.id === hashID) ? activeTail : null,
  )

  function fileName(file: LogFileInfo) {
    if (file.id === 'errors') return $_('pages.logs.RecentErrors')
    if (file.id === 'live') return $_('pages.logs.LiveStdout')
    return file.name
  }

  async function refresh() {
    const res = await api.get<LogFileInfos>('logs')
    if (!res.ok) return
    infos = res.body
    const list = infos.list ?? []
    if (activeTail && !list.find((f) => f.id === activeTail?.id)) {
      activeTail = null
    }
  }

  function viewFile(e: MouseEvent, file: LogFileInfo) {
    e.preventDefault()
    activeTail = null
    navigate('/logs/' + encodeURIComponent(file.id))
  }

  function tailFile(e: MouseEvent, file: LogFileInfo) {
    e.stopPropagation()
    if (!file.used) return
    activeTail = file
    navigate('/logs/' + encodeURIComponent(file.id))
  }

  onMount(() => {
    if (canRead) void refresh()
  })
</script>

<h4 class="mb-2">{$_('pages.logs.Title')}</h4>
<PageIntro id="pages.logs" />

{#if !canRead}
  <p class="text-muted">{$_('phrases.YouDoNotHavePermission')}</p>
{:else}
  <Row class="g-3">
    <Col md={7}>
      <Card>
        <CardBody>
          <CardTitle class="mb-2">{$_('pages.logs.FileList')}</CardTitle>
          <div class="file-list">
            <Table size="sm" striped borderless hover>
              <thead>
                <tr>
                  <th title={$_('pages.logs.FollowTooltip')}>
                    <button
                      type="button"
                      class="btn btn-link p-0"
                      title={$_('buttons.Refresh')}
                      onclick={(e) => {
                        e.preventDefault()
                        void refresh()
                      }}
                    >
                      <Icon i={ArrowsClockwise} />
                    </button>
                  </th>
                  <th>
                    {$_('pages.logs.Name')}
                    <small class="text-muted">
                      <T
                        id="pages.logs.FilesInDirs"
                        fileCount={infos?.list?.length ?? 0}
                        dirCount={infos?.dirs?.length ?? 0}
                      />
                    </small>
                  </th>
                  <th class="text-nowrap">{bytes(infos?.size ?? 0)}</th>
                  <th>{$_('pages.logs.Age')}</th>
                </tr>
              </thead>
              <tbody>
                {#each infos?.list ?? [] as file (file.id)}
                  {@const isActive =
                    hashedFile?.id === file.id || shownTail?.id === file.id}
                  <tr
                    class={['cursor-pointer', isActive && 'isActive']}
                    onclick={(e) => viewFile(e, file)}
                  >
                    <th class="fit">
                      {#if file.used}
                        <button
                          type="button"
                          class="btn btn-link p-0"
                          title={$_('pages.logs.TailFile', {
                            values: { fileName: fileName(file) },
                          })}
                          onclick={(e) => tailFile(e, file)}
                        >
                          <Icon i={ListBullets} />
                        </button>
                      {:else}&nbsp;{/if}
                    </th>
                    <td>{fileName(file)}</td>
                    <td class="fit">{bytes(file.size)}</td>
                    <td class="fit">{relTime(file.time)}</td>
                  </tr>
                {/each}
              </tbody>
            </Table>
          </div>
        </CardBody>
      </Card>
    </Col>
    <Col md={5}>
      <Card>
        <CardBody>
          <CardTitle class="mb-2">{$_('pages.logs.Details')}</CardTitle>
          {#if hashedFile || shownTail}
            <FileInfo file={(shownTail || hashedFile)!} />
          {:else}
            <p class="text-muted mb-0">{$_('pages.logs.NoFileSelected')}</p>
          {/if}
        </CardBody>
      </Card>
    </Col>
    <Col xs={12}>
      <Card>
        <CardBody>
          <CardTitle class="mb-2">
            {#if shownTail}
              {$_('pages.logs.FollowingFile')}
            {:else}
              {$_('pages.logs.FileContent')}
            {/if}
          </CardTitle>
          {#if shownTail}
            {#key shownTail.id}
              <Content file={shownTail} tail />
            {/key}
          {:else if hashedFile}
            {#key hashedFile.id}
              <Content file={hashedFile} />
            {/key}
          {:else}
            <p class="text-muted mb-0">{$_('pages.logs.SelectFile')}</p>
          {/if}
        </CardBody>
      </Card>
    </Col>
  </Row>
{/if}

<style>
  .file-list {
    max-height: 305px;
    overflow-y: auto;
  }
  .fit {
    width: 1%;
    white-space: nowrap;
  }
  .cursor-pointer {
    cursor: pointer;
  }
  .isActive {
    background-color: var(--bs-secondary-bg);
  }
</style>
