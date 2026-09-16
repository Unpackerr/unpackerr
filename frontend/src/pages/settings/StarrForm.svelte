<script lang="ts">
  import { onMount } from 'svelte'
  import {
    Button,
    Card,
    CardBody,
    CardHeader,
    Col,
    Input as Box,
    Modal,
    ModalBody,
    ModalFooter,
    ModalHeader,
    Row,
    Spinner,
  } from '@sveltestrap/sveltestrap'
  import Input from '../../components/Input.svelte'
  import SaveBar from '../../components/SaveBar.svelte'
  import { _ } from '../../lib/i18n/Translate.svelte'
  import {
    loadSection,
    loadSectionLive,
    saveSection,
    testSection,
  } from '../../lib/config'
  import { has } from '../../lib/auth.svelte'
  import { configPerm } from '../../lib/perms'
  import { canonicalizeProtocols, PROTOCOL_TORRENT } from '../../lib/protocols'
  import {
    deepCopy,
    deepEqual,
    explicitDeleteDelay,
    explicitTimeout,
  } from '../../lib/util'
  import { trackDirty } from '../../lib/dirty.svelte'
  import {
    httpURLError,
    slugDuplicateError,
    slugError,
    starrAPIKeyError,
  } from '../../lib/validate'
  import { failure } from '../../lib/toast'
  import type { ConfigSection, StarrConfig, StarrTestResult } from '../../lib/types'
  import { envHas } from '../../lib/env.svelte'
  import { saveStarrPoll, starrPollDirty } from './starr-poll.svelte'
  import {
    envField,
    instanceMap,
    mergeEnvOnlyRows,
    newRow,
    omitEnvFields,
    rowsFromMap,
    slugify,
    STARR_ENV_FIELDS,
    uniqueSlug,
    type InstanceRow,
  } from '../../lib/slug'

  let { section }: { section: ConfigSection } = $props()

  let rows = $state<InstanceRow<StarrConfig>[]>([])
  let orig = $state<Record<string, StarrConfig>>({})
  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')
  let testRow = $state<InstanceRow<StarrConfig> | null>(null)
  let testBusy = $state(false)
  let testError = $state('')
  let testElapsed = $state('')
  let testResult = $state<StarrTestResult | null>(null)

  const canWrite = $derived(has(configPerm(section, 'write')))
  const canWriteGeneral = $derived(has(configPerm('general', 'write')))
  const showSave = $derived(canWrite || (canWriteGeneral && starrPollDirty()))
  const envPrefix = $derived(section.toUpperCase())
  const invalid = $derived(
    rows.some((row) => {
      if (row.envOnly) return false
      const others = rows
        .filter((r) => r.id !== row.id)
        .map((r) => r.slug)
        .filter(Boolean)
      if (slugError(row.slug) || slugDuplicateError(row.slug, others))
        return true
      const urlBad =
        !envHas(envField(envPrefix, row.slug, 'URL')) &&
        !!httpURLError(row.value.url)
      const keyBad =
        !envHas(envField(envPrefix, row.slug, 'API_KEY')) &&
        !!starrAPIKeyError(row.value.apiKey, orig[row.slug]?.apiKey)
      return urlBad || keyBad
    }),
  )
  const appsDirty = $derived(!deepEqual(currentMap(), orig))
  const maxBytesDefault = $derived(defaultMaxBytes(section))
  const maxBytesChoices = $derived(
    maxBytesOptions(section, $_('words.select-option.Unlimited')),
  )

  function defaultMaxBytes(app: ConfigSection): string {
    switch (app) {
      case 'radarr':
        return '75GB'
      case 'lidarr':
        return '4GB'
      case 'readarr':
        return '1GB'
      default:
        return '20GB'
    }
  }

  const gbSteps = [5, 8, 10, 15, 20, 25, 30, 35, 40, 45, 50, 75, 100, 150, 200]

  function sizeLabel(size: string): string {
    return size.replace(
      /(\d+)(MB|GB|TB)/i,
      (_, n, unit) => `${n} ${unit.toUpperCase()}`,
    )
  }

  function maxBytesSizes(app: ConfigSection): string[] {
    const extra: Record<string, string[]> = {
      lidarr: ['1GB', '2GB', '4GB'],
      readarr: ['500MB', '1GB', '2GB', '4GB'],
    }
    const cap: Record<string, number> = {
      sonarr: 100,
      radarr: 200,
      lidarr: 75,
      readarr: 30,
    }
    const max = cap[app] ?? 100

    return [
      ...(extra[app] ?? []),
      ...gbSteps.filter((n) => n <= max).map((n) => `${n}GB`),
    ]
  }

  function maxBytesOptions(
    app: ConfigSection,
    unlimited: string,
  ): { value: string; name: string }[] {
    return [
      { value: '0', name: unlimited },
      ...maxBytesSizes(app).map((value) => ({ value, name: sizeLabel(value) })),
    ]
  }

  function instanceLabel(row: InstanceRow<StarrConfig>): string {
    const name = (row.value.name ?? '').trim()
    if (name) return name
    if (row.slug) return row.slug
    return $_('pages.settings.AddSection', { values: { section } })
  }

  function blank(): StarrConfig {
    const row: StarrConfig = {
      name: '',
      url: '',
      apiKey: '',
      path: '',
      paths: [],
      protocols: PROTOCOL_TORRENT,
      delete_orig: false,
      delete_delay: '5m',
      syncthing: false,
      valid_ssl: false,
      timeout: '10s',
      maxBytes: defaultMaxBytes(section),
    }
    if (section === 'lidarr') row.split_flac = false
    return row
  }

  function normalizeOne(a: StarrConfig): StarrConfig {
    const maxBytes = (a.maxBytes ?? '').trim() || defaultMaxBytes(section)
    const row = {
      ...a,
      name: a.name ?? '',
      paths: a.paths ?? [],
      protocols: canonicalizeProtocols(a.protocols),
      maxBytes,
      timeout: explicitTimeout(a.timeout),
      delete_delay: explicitDeleteDelay(a.delete_delay),
    }
    if (section !== 'lidarr') delete row.split_flac
    return row
  }

  function currentMap(): Record<string, StarrConfig> {
    const out: Record<string, StarrConfig> = {}
    for (const row of rows) {
      if (row.envOnly) continue
      out[row.slug || row.id] = row.value
    }
    return out
  }

  function taken(row: InstanceRow<StarrConfig>): string[] {
    return rows.filter((r) => r.id !== row.id).map((r) => r.slug).filter(Boolean)
  }

  function setName(row: InstanceRow<StarrConfig>, name: string) {
    row.value.name = name
    if (row.locked || !row.autoSlug) return
    const s = slugify(name)
    row.slug = s ? uniqueSlug(s, taken(row)) : row.slug
  }

  function setSlug(row: InstanceRow<StarrConfig>, slug: string) {
    row.slug = slug
    row.autoSlug = false
  }

  function savePayload(): Record<string, StarrConfig> {
    const out: Record<string, StarrConfig> = {}
    for (const row of rows) {
      if (row.envOnly || !row.slug) continue
      const { split_flac, ...rest } = row.value
      const item: StarrConfig = {
        ...rest,
        delete_delay: explicitDeleteDelay(row.value.delete_delay),
      }
      if (section === 'lidarr') item.split_flac = split_flac ?? false
      out[row.slug] = omitEnvFields(
        envPrefix,
        row.slug,
        item,
        STARR_ENV_FIELDS,
      )
    }
    return out
  }

  trackDirty(() => !loading && appsDirty)

  onMount(load)

  async function load() {
    loading = true

    const { data, error: err } = await loadSection<unknown>(section)
    if (err) error = err
    else {
      const map = instanceMap<StarrConfig>(data)
      const normalized: Record<string, StarrConfig> = {}
      for (const [slug, app] of Object.entries(map)) {
        normalized[slug] = normalizeOne(app)
      }
      orig = deepCopy(normalized)
      let next = rowsFromMap(normalized)
      const live = await loadSectionLive<unknown>(section)
      if (live.data != null) {
        next = mergeEnvOnlyRows(
          next,
          Object.keys(instanceMap(live.data)),
          blank,
        )
      }
      rows = next
    }

    loading = false
  }

  function add() {
    rows = [...rows, newRow(blank())]
  }

  function remove(id: string) {
    rows = rows.filter((r) => r.id !== id)
  }

  async function save() {
    if (canWrite && invalid) {
      failure($_('phrases.FixInvalidFields'))
      return
    }

    saving = true
    if (!(await saveStarrPoll(canWrite))) {
      saving = false
      return
    }

    if (canWrite) {
      const ok = await saveSection(section, savePayload())
      if (ok) {
        for (const row of rows) {
          if (row.slug) {
            row.locked = true
            row.autoSlug = false
          }
        }
        orig = deepCopy(currentMap())
      }
    }

    saving = false
  }

  const testOpen = $derived(testRow !== null)
  const testName = $derived(testRow ? instanceLabel(testRow) : '')

  function testBlocked(row: InstanceRow<StarrConfig>): boolean {
    if (row.envOnly) return true
    return (
      envHas(envField(envPrefix, row.slug, 'URL')) ||
      envHas(envField(envPrefix, row.slug, 'API_KEY'))
    )
  }

  function closeTest() {
    testRow = null
    testBusy = false
    testError = ''
    testElapsed = ''
    testResult = null
  }

  async function runStarrTest(row: InstanceRow<StarrConfig>) {
    if (testBlocked(row)) return
    testBusy = true
    testError = ''
    testElapsed = ''
    testResult = null
    const app = row.value
    const res = await testSection<StarrTestResult>(section, {
      slug: row.slug,
      url: app.url,
      apiKey: app.apiKey,
      valid_ssl: app.valid_ssl,
      timeout: app.timeout,
    })
    if (testRow !== row) return
    testBusy = false
    testElapsed = (res.body as { elapsed?: string })?.elapsed ?? ''
    if (!res.ok) {
      testError = (res.body as { error?: string })?.error ?? 'test failed'
      return
    }
    testResult = res.body
  }

  function openStarrTest(row: InstanceRow<StarrConfig>) {
    if (testBlocked(row)) return
    testRow = row
    void runStarrTest(row)
  }
</script>

{#if loading}
  <Spinner color="primary" />
{:else if error}
  <p class="text-danger">{error}</p>
{:else}
  {#if rows.length === 0}
    <p class="text-muted">
      {$_('phrases.NoInstances', { values: { section } })}
    </p>
  {/if}

  {#each rows as row (row.id)}
    {@const app = row.value}
    {@const slug = row.slug}
    {@const prev = orig[slug]}
    {@const envOnly = row.envOnly}
    <Card class="mb-3">
      <CardHeader>
        <Row class="align-items-center">
          <Col>
            <span
              class={(app.name ?? '').trim()
                ? 'fw-semibold'
                : 'text-capitalize fw-semibold'}>{instanceLabel(row)}</span
            >
          </Col>
          {#if canWrite && !envOnly}
            <Col xs="auto">
              <Button
                size="sm"
                color="danger"
                outline
                on:click={() => remove(row.id)}>{$_('buttons.Remove')}</Button
              >
            </Col>
          {/if}
        </Row>
      </CardHeader>
      <CardBody>
        <Row class="g-2">
          <Col md="4">
            <Input
              id={`${section}-${row.id}-slug`}
              label={$_('pages.settings.Key')}
              description={$_('pages.settings.KeyLocked')}
              tooltip={$_('pages.settings.KeyTooltip')}
              bind:value={() => row.slug, (v) => setSlug(row, v)}
              original={row.locked ? slug : ''}
              disabled={!canWrite || row.locked}
              validate={(_id, v) =>
                slugError(v) || slugDuplicateError(String(v), taken(row))}
            />
          </Col>
          <Col md="8">
            <Input
              id={`${section}-${row.id}-name`}
              helpKey="config.starr.name"
              label={$_('config.starr.name.label')}
              bind:value={() => app.name, (v) => setName(row, v)}
              original={prev?.name}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'NAME')}
            />
          </Col>
          <Col md="12">
            <Input
              id={`${section}-${row.id}-url`}
              helpKey="config.starr.url"
              label={$_('config.starr.url.label')}
              placeholder={$_('config.starr.url.placeholder')}
              bind:value={app.url}
              original={prev?.url}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'URL')}
              validate={(_id, v) => httpURLError(v)}
            >
              {#snippet post()}
                {#if app.url.startsWith('https://')}
                  <Button
                    type="button"
                    outline
                    title={$_('phrases.ValidSSL')}
                    aria-pressed={!!app.valid_ssl}
                    class={app.valid_ssl !== prev?.valid_ssl
                      ? 'ssl-addon changed'
                      : 'ssl-addon'}
                    style="width:44px;"
                    disabled={!canWrite ||
                      row.envOnly ||
                      envHas(envField(envPrefix, slug, 'VALID_SSL'))}
                    on:click={() => (app.valid_ssl = !app.valid_ssl)}
                  >
                    <Box
                      type="checkbox"
                      checked={!!app.valid_ssl}
                      tabindex={-1}
                      class="ssl-check"
                    />
                  </Button>
                {/if}
                {#if canWrite}
                  <Button
                    type="button"
                    color="success"
                    outline
                    disabled={testBlocked(row)}
                    title={testBlocked(row)
                      ? $_('phrases.TestStarrEnvOwned')
                      : $_('buttons.Test')}
                    on:click={() => openStarrTest(row)}
                    >{$_('buttons.Test')}</Button
                  >
                {/if}
              {/snippet}
            </Input>
          </Col>
          <Col md="12">
            <Input
              id={`${section}-${row.id}-apikey`}
              helpKey="config.starr.apiKey"
              type="password"
              label={$_('config.starr.apiKey.label')}
              bind:value={app.apiKey}
              original={prev?.apiKey}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'API_KEY')}
              validate={(_id, v) => starrAPIKeyError(v, prev?.apiKey)}
            />
          </Col>
          <Col md="12">
            <Input
              id={`${section}-${row.id}-path`}
              helpKey="config.starr.path"
              label={$_('config.starr.path.label')}
              bind:value={app.path}
              original={prev?.path}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'PATH')}
              browse="dir"
            />
          </Col>
          <Col md="6">
            <Input
              id={`${section}-${row.id}-protocols`}
              helpKey="config.starr.protocols"
              type="select"
              label={$_('config.starr.protocols.label')}
              bind:value={app.protocols}
              original={prev?.protocols}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'PROTOCOLS')}
              options={[
                {
                  value: 'torrent,TorrentDownloadProtocol',
                  name: $_('config.starr.protocols.torrent'),
                },
                {
                  value: 'usenet,UsenetDownloadProtocol',
                  name: $_('config.starr.protocols.usenet'),
                },
                {
                  value:
                    'torrent,TorrentDownloadProtocol,usenet,UsenetDownloadProtocol',
                  name: $_('config.starr.protocols.both'),
                },
              ]}
            />
          </Col>
          <Col md="6">
            <Input
              id={`${section}-${row.id}-timeout`}
              helpKey="config.starr.timeout"
              type="timeout"
              label={$_('config.starr.timeout.label')}
              bind:value={app.timeout}
              original={prev?.timeout}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'TIMEOUT')}
            />
          </Col>
          <Col md="6">
            <Input
              id={`${section}-${row.id}-delete-delay`}
              helpKey="config.starr.delete_delay"
              type="deletedelay"
              label={$_('config.starr.delete_delay.label')}
              bind:value={app.delete_delay}
              original={prev?.delete_delay}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'DELETE_DELAY')}
            />
          </Col>
          <Col md="6">
            <Input
              id={`${section}-${row.id}-maxbytes`}
              helpKey="config.starr.maxBytes"
              type="maxbytes"
              label={$_('config.starr.maxBytes.label')}
              bind:value={app.maxBytes}
              original={prev?.maxBytes ?? maxBytesDefault}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'MAX_BYTES')}
              options={maxBytesChoices}
            />
          </Col>
          <Col md="6">
            <Input
              id={`${section}-${row.id}-delete-orig`}
              helpKey="config.starr.delete_orig"
              type="select"
              label={$_('config.starr.delete_orig.label')}
              bind:value={app.delete_orig}
              original={prev?.delete_orig}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'DELETE_ORIG')}
            />
          </Col>
          <Col md="6">
            <Input
              id={`${section}-${row.id}-syncthing`}
              helpKey="config.starr.syncthing"
              type="select"
              label={$_('config.starr.syncthing.label')}
              bind:value={app.syncthing}
              original={prev?.syncthing}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'SYNCTHING')}
            />
          </Col>
          {#if section === 'lidarr'}
            <Col md="6">
              <Input
                id={`${section}-${row.id}-flac`}
                helpKey="config.starr.split_flac"
                type="select"
                label={$_('config.starr.split_flac.label')}
                bind:value={app.split_flac}
                original={prev?.split_flac}
                disabled={!canWrite || row.envOnly}
                envVar={envField(envPrefix, slug, 'SPLIT_FLAC')}
              />
            </Col>
          {/if}
        </Row>
      </CardBody>
    </Card>
  {/each}

  {#if showSave}
    <SaveBar>
      <Button
        color="primary"
        disabled={saving || (canWrite && invalid)}
        on:click={save}
      >
        {#if saving}<Spinner size="sm" />{/if}
        <span class="ms-1">{$_('buttons.Save')}</span>
      </Button>
      {#if canWrite}
        <Button color="secondary" outline on:click={add}
          >{$_('pages.settings.AddSection', { values: { section } })}</Button
        >
      {/if}
    </SaveBar>
  {/if}
{/if}

<Modal isOpen={testOpen} toggle={closeTest}>
  <ModalHeader toggle={closeTest}
    >{$_('phrases.TestTitle', { values: { name: testName } })}</ModalHeader
  >
  <ModalBody>
    {#if testBusy}
      <Spinner color="primary" size="sm" />
    {:else if testError}
      <p class="text-danger mb-0">{testError}</p>
      {#if testElapsed}
        <p class="text-muted mb-0 mt-2"
          >{$_('phrases.TestDuration')}: {testElapsed}</p
        >
      {/if}
    {:else if testResult}
      <dl class="row mb-0">
        <dt class="col-6">{$_('phrases.TestQueued')}</dt>
        <dd class="col-6">{testResult.queued}</dd>
        <dt class="col-6">{$_('phrases.TestRetrieved')}</dt>
        <dd class="col-6">{testResult.retrieved}</dd>
        <dt class="col-6">{$_('phrases.TestTorrents')}</dt>
        <dd class="col-6">{testResult.torrents}</dd>
        <dt class="col-6">{$_('phrases.TestNzbs')}</dt>
        <dd class="col-6">{testResult.nzbs}</dd>
        {#if testResult.other}
          <dt class="col-6">{$_('phrases.TestOther')}</dt>
          <dd class="col-6">{testResult.other}</dd>
        {/if}
        {#if testElapsed}
          <dt class="col-6">{$_('phrases.TestDuration')}</dt>
          <dd class="col-6">{testElapsed}</dd>
        {/if}
      </dl>
    {/if}
  </ModalBody>
  <ModalFooter>
    <Button color="warning" on:click={closeTest}>{$_('buttons.Close')}</Button>
    {#if testRow && !testBusy}
      <Button
        color="success"
        on:click={() => {
          if (testRow) void runStarrTest(testRow)
        }}>{$_('buttons.Retry')}</Button
      >
    {/if}
  </ModalFooter>
</Modal>

<style>
  :global(.ssl-addon) {
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  :global(.ssl-addon .form-check) {
    margin: 0;
    min-height: 0;
    padding-left: 0;
  }
  :global(.ssl-addon .form-check-input),
  :global(.ssl-addon .ssl-check) {
    pointer-events: none;
    margin: 0;
    float: none;
  }
</style>
