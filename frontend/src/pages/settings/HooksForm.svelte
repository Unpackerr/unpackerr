<script lang="ts">
  import { onMount } from 'svelte'
  import {
    Button,
    Card,
    CardBody,
    CardHeader,
    Col,
    Collapse,
    FormCheck,
    FormGroup,
    FormText,
    Label,
    Modal,
    ModalBody,
    ModalFooter,
    ModalHeader,
    Row,
    Spinner,
    Badge,
  } from '@sveltestrap/sveltestrap'
  import Input from '../../components/Input.svelte'
  import SaveBar from '../../components/SaveBar.svelte'
  import CreateTemplate from '../../components/fileBrowser/CreateTemplate.svelte'
  import { _ } from '../../lib/i18n/Translate.svelte'
  import {
    loadSection,
    loadSectionLive,
    saveSection,
    testSection,
  } from '../../lib/config'
  import { has } from '../../lib/auth.svelte'
  import { configPerm, systemPerm } from '../../lib/perms'
  import {
    EXTRACT_STATUSES,
    type ConfigSection,
    type HookTestResult,
    type StarrConfig,
    type WebhookConfig,
  } from '../../lib/types'
  import { deepCopy, deepEqual, explicitTimeout } from '../../lib/util'
  import { trackDirty } from '../../lib/dirty.svelte'
  import {
    httpURLError,
    requiredCommandError,
    slugDuplicateError,
    slugError,
  } from '../../lib/validate'
  import { failure } from '../../lib/toast'
  import { envHas } from '../../lib/env.svelte'
  import {
    HOOK_TEMPLATE_NAMES,
    hookFormProfile,
    hookShowsField,
    type DetectedHookTemplate,
    type HookFormProfile,
  } from '../../lib/hooktmpl'
  import {
    envField,
    HOOK_ENV_FIELDS,
    instanceMap,
    mergeEnvOnlyRows,
    newRow,
    omitEnvFields,
    rowsFromMap,
    slugify,
    uniqueSlug,
    type InstanceRow,
  } from '../../lib/slug'

  let { section }: { section: ConfigSection } = $props()

  const isCmd = $derived(section === 'cmdhooks')
  let rows = $state<InstanceRow<WebhookConfig>[]>([])
  let orig = $state<Record<string, WebhookConfig>>({})
  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')
  let testRow = $state<InstanceRow<WebhookConfig> | null>(null)
  let testEvent = $state('extracted')
  let testApp = $state('sonarr')
  let testBusy = $state(false)
  let testError = $state('')
  let testElapsed = $state('')
  let testResult = $state<HookTestResult | null>(null)
  let createRow = $state<InstanceRow<WebhookConfig> | null>(null)
  let guideOpen = $state(false)
  let guidePick = $state<DetectedHookTemplate>('notifiarr')
  let advancedOpen = $state<Record<string, boolean>>({})

  const canWrite = $derived(has(configPerm(section, 'write')))
  const canCreateTemplate = $derived(
    canWrite && has(systemPerm('browse', 'write')),
  )
  const envPrefix = $derived(isCmd ? 'CMDHOOK' : 'WEBHOOK')
  const idField = $derived(isCmd ? 'COMMAND' : 'URL')
  const invalid = $derived(
    rows.some((row) => {
      if (row.envOnly) return false
      const others = rows
        .filter((r) => r.id !== row.id)
        .map((r) => r.slug)
        .filter(Boolean)
      if (slugError(row.slug) || slugDuplicateError(row.slug, others))
        return true
      if (envHas(envField(envPrefix, row.slug, idField))) return false
      return isCmd
        ? !!requiredCommandError(row.value.command)
        : !!httpURLError(row.value.url)
    }),
  )

  const statusKeys = [
    'Waiting',
    'Queued',
    'Extracting',
    'ExtractFailed',
    'Extracted',
    'Imported',
    'Deleting',
    'DeleteFailed',
    'Deleted',
    'ExtractedNothing',
  ]

  const hookApps = [
    { value: 'sonarr', name: 'Sonarr' },
    { value: 'radarr', name: 'Radarr' },
    { value: 'lidarr', name: 'Lidarr' },
    { value: 'readarr', name: 'Readarr' },
    { value: 'folder', name: 'Folder' },
  ]

  type NamedByApp = {
    sonarr: string[]
    radarr: string[]
    lidarr: string[]
    readarr: string[]
  }

  function emptyNamed(): NamedByApp {
    return {
      sonarr: [],
      radarr: [],
      lidarr: [],
      readarr: [],
    }
  }

  let namedByApp = $state<NamedByApp>(emptyNamed())

  function namesFor(dialect: string): string[] {
    return namedByApp[dialect as keyof NamedByApp] ?? []
  }

  const hookAppChoices = $derived.by(() => {
    const extra: { value: string; name: string }[] = []
    const taken = hookApps.map((a) => a.value.toLowerCase())
    for (const dialect of ['sonarr', 'radarr', 'lidarr', 'readarr'] as const) {
      for (const name of namedByApp[dialect]) {
        const key = name.toLowerCase()
        if (
          taken.includes(key) ||
          extra.some((item) => item.value.toLowerCase() === key)
        ) {
          continue
        }
        extra.push({ value: name, name: `${name} (${dialect})` })
      }
    }
    return [...hookApps, ...extra]
  })

  const templateChoices = $derived([
    { value: '', name: $_('config.hooks.template.auto') },
    ...HOOK_TEMPLATE_NAMES.map((name) => ({
      value: name,
      name: i18nOr('config.hooks.profile.' + name, name),
    })),
  ])

  const GUIDE_TEMPLATES: readonly DetectedHookTemplate[] = [
    ...HOOK_TEMPLATE_NAMES,
    'custom',
  ]

  const guideChoices = $derived(
    GUIDE_TEMPLATES.map((name) => ({
      value: name,
      name: i18nOr('config.hooks.profile.' + name, name),
    })),
  )

  function blank(): WebhookConfig {
    return {
      name: '',
      url: '',
      command: '',
      contentType: 'application/json',
      templatePath: '',
      template: '',
      timeout: '10s',
      shell: false,
      ignoreSsl: false,
      silent: false,
      events: [],
      exclude: [],
      nickname: '',
      token: '',
      channel: '',
      update: true,
    }
  }

  function normalizeHook(
    h: Partial<WebhookConfig> | null | undefined,
  ): WebhookConfig {
    return {
      ...blank(),
      ...h,
      timeout: explicitTimeout(h?.timeout),
      events: Array.isArray(h?.events) ? [...h.events] : [],
      exclude: Array.isArray(h?.exclude) ? [...h.exclude] : [],
      update: h?.update ?? true,
    }
  }

  function taken(row: InstanceRow<WebhookConfig>): string[] {
    return rows
      .filter((r) => r.id !== row.id)
      .map((r) => r.slug)
      .filter(Boolean)
  }

  function setName(row: InstanceRow<WebhookConfig>, name: string) {
    row.value.name = name
    if (row.locked || !row.autoSlug) return
    const s = slugify(name)
    row.slug = s ? uniqueSlug(s, taken(row)) : row.slug
  }

  function setSlug(row: InstanceRow<WebhookConfig>, slug: string) {
    row.slug = slug
    row.autoSlug = false
  }

  function currentMap(): Record<string, WebhookConfig> {
    const out: Record<string, WebhookConfig> = {}
    for (const row of rows) {
      if (row.envOnly) continue
      out[row.slug || row.id] = row.value
    }
    return out
  }

  function savePayload(): Record<string, WebhookConfig> {
    const out: Record<string, WebhookConfig> = {}
    for (const row of rows) {
      if (row.envOnly || !row.slug) continue
      out[row.slug] = omitHiddenContentType(
        omitEnvFields(envPrefix, row.slug, row.value, HOOK_ENV_FIELDS),
      )
    }
    return out
  }

  // Built-ins set Content-Type themselves. Sending the form's json default
  // would skip the Pushover form-urlencoded default on save and test.
  function omitHiddenContentType(hook: WebhookConfig): WebhookConfig {
    if (isCmd || hookShowsField(profileOf(hook), 'contentType')) return hook
    return { ...hook, contentType: '' }
  }

  function eventOn(
    hook: WebhookConfig,
    st: (typeof EXTRACT_STATUSES)[number],
  ): boolean {
    const ev = hook.events ?? []
    return ev.some(
      (e) =>
        e === st.value ||
        e === st.id ||
        String(e).toLowerCase() === st.id ||
        String(e) === String(st.value),
    )
  }

  async function loadNamedInstances() {
    const next = emptyNamed()
    await Promise.all(
      (Object.keys(next) as (keyof NamedByApp)[]).map(async (sec) => {
        if (!has(configPerm(sec, 'read'))) return
        const { data } = await loadSectionLive<unknown>(sec)
        const names = Object.values(instanceMap<StarrConfig>(data))
          .map((a) => (a.name ?? '').trim())
          .filter((n) => n !== '')
        next[sec] = [...new Set(names)]
      }),
    )
    namedByApp = next
  }

  onMount(async () => {
    const named = loadNamedInstances()
    try {
      const { data, error: err } = await loadSection<unknown>(section)
      if (err) error = err
      else {
        const map = instanceMap<WebhookConfig>(data)
        const normalized: Record<string, WebhookConfig> = {}
        for (const [slug, h] of Object.entries(map)) {
          normalized[slug] = normalizeHook(h)
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
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      await named.catch(() => undefined)
      loading = false
    }
  })

  trackDirty(() => !loading && !deepEqual(currentMap(), orig))

  function add() {
    rows = [...rows, newRow(blank())]
  }
  function remove(id: string) {
    rows = rows.filter((r) => r.id !== id)
  }
  function toggleEvent(
    hook: WebhookConfig,
    st: (typeof EXTRACT_STATUSES)[number],
    on: boolean,
  ) {
    const ev = hook.events ?? []
    const without = ev.filter(
      (e) =>
        e !== st.value &&
        e !== st.id &&
        String(e).toLowerCase() !== st.id &&
        String(e) !== String(st.value),
    )
    hook.events = on ? [...without, st.id] : without
  }
  function excludeHas(hook: WebhookConfig, token: string): boolean {
    return (hook.exclude ?? []).some(
      (e) => e.toLowerCase() === token.toLowerCase(),
    )
  }
  function dialectOn(hook: WebhookConfig, dialect: string): boolean {
    return excludeHas(hook, dialect)
  }
  function nameOn(hook: WebhookConfig, dialect: string, name: string): boolean {
    return dialectOn(hook, dialect) || excludeHas(hook, name)
  }
  function setExclude(hook: WebhookConfig, remove: string[], add: string[]) {
    const rm = new Set(remove.map((s) => s.toLowerCase()))
    const without = (hook.exclude ?? []).filter((e) => !rm.has(e.toLowerCase()))
    hook.exclude = [...without, ...add]
  }
  function toggleDialect(
    hook: WebhookConfig,
    dialect: string,
    names: string[],
    on: boolean,
  ) {
    if (on) {
      setExclude(hook, [dialect, ...names], [dialect])
      return
    }
    setExclude(hook, [dialect], [])
  }
  function toggleName(
    hook: WebhookConfig,
    dialect: string,
    names: string[],
    name: string,
    on: boolean,
  ) {
    if (dialectOn(hook, dialect) && !on) {
      const keep = names.filter((n) => n.toLowerCase() !== name.toLowerCase())
      setExclude(hook, [dialect, ...names], keep)
      return
    }
    if (on) {
      setExclude(hook, [name], [name])
      return
    }
    setExclude(hook, [name], [])
  }
  async function save() {
    if (invalid) {
      failure($_('phrases.FixInvalidFields'))
      return
    }
    saving = true
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
    saving = false
  }

  function hookLabel(row: InstanceRow<WebhookConfig>): string {
    return (
      (row.value.name ?? '').trim() ||
      row.slug ||
      (isCmd
        ? $_('pages.settings.AddCmdhook')
        : $_('pages.settings.AddWebhook'))
    )
  }

  const testOpen = $derived(testRow !== null)
  const testName = $derived(testRow ? hookLabel(testRow) : '')

  function testBlocked(row: InstanceRow<WebhookConfig>): boolean {
    if (row.envOnly) return true
    return envHas(envField(envPrefix, row.slug, idField))
  }

  function closeHookTest() {
    testRow = null
    testBusy = false
    testError = ''
    testElapsed = ''
    testResult = null
  }

  function openHookTest(row: InstanceRow<WebhookConfig>) {
    if (testBlocked(row)) return
    testRow = row
    testEvent = 'extracted'
    testApp = 'sonarr'
    testBusy = false
    testError = ''
    testElapsed = ''
    testResult = null
  }

  async function runHookTest() {
    if (!testRow || testBlocked(testRow)) return
    const row = testRow
    const hook = row.value
    testBusy = true
    testError = ''
    testElapsed = ''
    testResult = null
    const res = await testSection<HookTestResult>(section, {
      slug: row.slug,
      url: hook.url,
      command: hook.command,
      token: hook.token,
      contentType: omitHiddenContentType(hook).contentType,
      template: hook.template,
      templatePath: hook.templatePath,
      timeout: hook.timeout,
      shell: hook.shell,
      ignoreSsl: hook.ignoreSsl,
      nickname: hook.nickname,
      channel: hook.channel,
      name: hook.name,
      event: testEvent,
      app: testApp,
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

  function closeCreate() {
    createRow = null
  }

  function createdTemplate(path: string) {
    if (createRow) {
      createRow.value.templatePath = path
      createRow.value.template = ''
    }
    createRow = null
  }

  function profileOf(hook: WebhookConfig): HookFormProfile {
    return hookFormProfile(hook.template, hook.url, hook.templatePath)
  }

  function i18nOr(key: string, fallback: string): string {
    const text = $_(key)
    return text && text !== key ? text : fallback
  }

  function hookFieldText(
    template: DetectedHookTemplate,
    field: string,
    part: 'label' | 'description' | 'tooltip',
  ): string {
    const specificKey = 'config.hooks.' + field + '.' + template + '.' + part
    const specific = $_(specificKey)
    if (specific && specific !== specificKey) return specific
    return i18nOr('config.hooks.' + field + '.' + part, '')
  }

  function guideTitle(template: DetectedHookTemplate): string {
    const name = i18nOr('config.hooks.profile.' + template, template)
    return $_('config.hooks.guide.title', { values: { name } })
  }

  function guideParagraphs(template: DetectedHookTemplate): string[] {
    const key = 'config.hooks.guide.' + template
    const text = $_(key)
    if (!text || text === key) return []
    return text
      .trim()
      .split(/\n\s*\n/)
      .map((p) => p.replace(/[ \t]*\n[ \t]*/g, ' ').trim())
      .filter(Boolean)
  }

  function inlineParts(text: string): { code: boolean; text: string }[] {
    return text.split('`').map((bit, i) => ({
      code: i % 2 === 1,
      text: bit,
    }))
  }

  function profileChip(profile: HookFormProfile): string {
    const name = i18nOr(
      'config.hooks.profile.' + profile.template,
      profile.template,
    )
    if (profile.source === 'url') {
      return $_('config.hooks.chip.fromURL', { values: { name } })
    }
    if (profile.source === 'named') {
      return $_('config.hooks.chip.named', { values: { name } })
    }
    if (profile.source === 'file') {
      return $_('config.hooks.chip.custom')
    }
    return $_('config.hooks.chip.default')
  }

  function advancedDefault(hook: WebhookConfig): boolean {
    return !!(hook.template ?? '').trim() || !!(hook.templatePath ?? '').trim()
  }

  function isAdvancedOpen(row: InstanceRow<WebhookConfig>): boolean {
    return advancedOpen[row.id] ?? advancedDefault(row.value)
  }

  function toggleAdvanced(row: InstanceRow<WebhookConfig>) {
    advancedOpen[row.id] = !isAdvancedOpen(row)
  }

  function closeGuide() {
    guideOpen = false
  }

  function openGuide(row: InstanceRow<WebhookConfig>) {
    guidePick = profileOf(row.value).template
    guideOpen = true
  }
</script>

{#if loading}
  <Spinner color="primary" />
{:else if error}
  <p class="text-danger">{error}</p>
{:else}
  {#if rows.length === 0}
    <p class="text-muted">
      {isCmd ? $_('phrases.NoCmdhooks') : $_('phrases.NoWebhooks')}
    </p>
  {/if}

  {#each rows as row (row.id)}
    {@const hook = row.value}
    {@const slug = row.slug}
    {@const prev = orig[slug]}
    {@const envOnly = row.envOnly}
    {@const profile = profileOf(hook)}
    <Card class="mb-3">
      <CardHeader>
        <Row class="align-items-center">
          <Col>
            <span class="fw-semibold">
              {(hook.name ?? '').trim() ||
                slug ||
                (isCmd
                  ? $_('pages.settings.AddCmdhook')
                  : $_('pages.settings.AddWebhook'))}
            </span>
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
          {#if isCmd}
            <Col md="8">
              <Input
                id={`${section}-${row.id}-name`}
                helpKey="config.hooks.name"
                label={$_('config.hooks.name.label')}
                bind:value={() => hook.name, (v) => setName(row, v)}
                original={prev?.name}
                disabled={!canWrite || row.envOnly}
                envVar={envField(envPrefix, slug, 'NAME')}
              />
            </Col>
            <Col md="12">
              <Input
                id={`${section}-${row.id}-command`}
                helpKey="config.hooks.command"
                label={$_('config.hooks.command.label')}
                bind:value={hook.command}
                original={prev?.command}
                disabled={!canWrite || row.envOnly}
                envVar={envField(envPrefix, slug, 'COMMAND')}
                browse="file"
                disableMkdir
                validate={(_id, v) => requiredCommandError(v)}
              >
                {#snippet post()}
                  {#if canWrite}
                    <Button
                      type="button"
                      color="success"
                      outline
                      disabled={testBlocked(row)}
                      title={testBlocked(row)
                        ? $_('phrases.TestHookEnvOwned')
                        : $_('buttons.Test')}
                      on:click={() => openHookTest(row)}
                      >{$_('buttons.Test')}</Button
                    >
                  {/if}
                {/snippet}
              </Input>
            </Col>
            <Col md="4">
              <Input
                id={`${section}-${row.id}-timeout`}
                helpKey="config.hooks.timeout"
                type="timeout"
                label={$_('config.hooks.timeout.label')}
                bind:value={hook.timeout}
                original={prev?.timeout}
                disabled={!canWrite || row.envOnly}
                envVar={envField(envPrefix, slug, 'TIMEOUT')}
              />
            </Col>
            <Col md="4">
              <Input
                id={`${section}-${row.id}-shell`}
                helpKey="config.hooks.shell"
                type="select"
                label={$_('config.hooks.shell.label')}
                bind:value={hook.shell}
                original={prev?.shell}
                disabled={!canWrite || row.envOnly}
                envVar={envField(envPrefix, slug, 'SHELL')}
              />
            </Col>
            <Col md="4">
              <Input
                id={`${section}-${row.id}-silent`}
                helpKey="config.hooks.silent"
                type="select"
                label={$_('config.hooks.silent.label')}
                bind:value={hook.silent}
                original={prev?.silent}
                disabled={!canWrite || row.envOnly}
                envVar={envField(envPrefix, slug, 'SILENT')}
              />
            </Col>
          {:else}
            <Col md="8">
              <Input
                id={`${section}-${row.id}-name`}
                helpKey="config.hooks.name"
                label={$_('config.hooks.name.label')}
                bind:value={() => hook.name, (v) => setName(row, v)}
                original={prev?.name}
                disabled={!canWrite || row.envOnly}
                envVar={envField(envPrefix, slug, 'NAME')}
              />
            </Col>
            <Col md="8">
              <Input
                id={`${section}-${row.id}-url`}
                helpKey="config.hooks.url"
                label={$_('config.hooks.url.label')}
                bind:value={hook.url}
                original={prev?.url}
                disabled={!canWrite || row.envOnly}
                envVar={envField(envPrefix, slug, 'URL')}
                validate={(_id, v) => httpURLError(v)}
              >
                {#snippet post()}
                  {#if canWrite}
                    <Button
                      type="button"
                      color="success"
                      outline
                      disabled={testBlocked(row)}
                      title={testBlocked(row)
                        ? $_('phrases.TestHookEnvOwned')
                        : $_('buttons.Test')}
                      on:click={() => openHookTest(row)}
                      >{$_('buttons.Test')}</Button
                    >
                  {/if}
                {/snippet}
              </Input>
            </Col>
            <Col md="4">
              <Input
                id={`${section}-${row.id}-timeout`}
                helpKey="config.hooks.timeout"
                type="timeout"
                label={$_('config.hooks.timeout.label')}
                bind:value={hook.timeout}
                original={prev?.timeout}
                disabled={!canWrite || row.envOnly}
                envVar={envField(envPrefix, slug, 'TIMEOUT')}
              />
            </Col>
            <Col md="12">
              <div class="d-flex flex-wrap align-items-center gap-2 mb-2">
                <Badge color="info">{profileChip(profile)}</Badge>
                <Button
                  type="button"
                  size="sm"
                  color="primary"
                  outline
                  on:click={() => openGuide(row)}
                  >{$_('config.hooks.guide.button')}</Button
                >
              </div>
            </Col>
            <Col md="6">
              <Input
                id={`${section}-${row.id}-ssl`}
                helpKey="config.hooks.ignoreSsl"
                type="select"
                label={$_('config.hooks.ignoreSsl.label')}
                bind:value={hook.ignoreSsl}
                original={prev?.ignoreSsl}
                disabled={!canWrite || row.envOnly}
                envVar={envField(envPrefix, slug, 'IGNORE_SSL')}
              />
            </Col>
            <Col md="6">
              <Input
                id={`${section}-${row.id}-silent`}
                helpKey="config.hooks.silent"
                type="select"
                label={$_('config.hooks.silent.label')}
                bind:value={hook.silent}
                original={prev?.silent}
                disabled={!canWrite || row.envOnly}
                envVar={envField(envPrefix, slug, 'SILENT')}
              />
            </Col>
            {#if hookShowsField(profile, 'update')}
              <Col md="6">
                <Input
                  id={`${section}-${row.id}-update`}
                  helpKey="config.hooks.update"
                  type="select"
                  label={$_('config.hooks.update.label')}
                  bind:value={hook.update}
                  original={prev?.update}
                  disabled={!canWrite || row.envOnly}
                  envVar={envField(envPrefix, slug, 'UPDATE')}
                />
              </Col>
            {/if}
            {#if hookShowsField(profile, 'nickname')}
              <Col md="6">
                <Input
                  id={`${section}-${row.id}-nick`}
                  helpKey="config.hooks.nickname"
                  label={hookFieldText(profile.template, 'nickname', 'label')}
                  description={hookFieldText(
                    profile.template,
                    'nickname',
                    'description',
                  )}
                  tooltip={hookFieldText(profile.template, 'nickname', 'tooltip')}
                  bind:value={hook.nickname}
                  original={prev?.nickname}
                  disabled={!canWrite || row.envOnly}
                  envVar={envField(envPrefix, slug, 'NICKNAME')}
                />
              </Col>
            {/if}
            {#if hookShowsField(profile, 'channel')}
              <Col md="6">
                <Input
                  id={`${section}-${row.id}-channel`}
                  helpKey="config.hooks.channel"
                  label={hookFieldText(profile.template, 'channel', 'label')}
                  description={hookFieldText(
                    profile.template,
                    'channel',
                    'description',
                  )}
                  tooltip={hookFieldText(profile.template, 'channel', 'tooltip')}
                  bind:value={hook.channel}
                  original={prev?.channel}
                  disabled={!canWrite || row.envOnly}
                  envVar={envField(envPrefix, slug, 'CHANNEL')}
                />
              </Col>
            {/if}
            {#if hookShowsField(profile, 'token')}
              <Col md="6">
                <Input
                  id={`${section}-${row.id}-token`}
                  helpKey="config.hooks.token"
                  type="password"
                  label={hookFieldText(profile.template, 'token', 'label')}
                  description={hookFieldText(
                    profile.template,
                    'token',
                    'description',
                  )}
                  tooltip={hookFieldText(profile.template, 'token', 'tooltip')}
                  bind:value={hook.token}
                  original={prev?.token}
                  disabled={!canWrite || row.envOnly}
                  envVar={envField(envPrefix, slug, 'TOKEN')}
                />
              </Col>
            {/if}
            {#if hookShowsField(profile, 'contentType')}
              <Col md="6">
                <Input
                  id={`${section}-${row.id}-ctype`}
                  helpKey="config.hooks.contentType"
                  label={$_('config.hooks.contentType.label')}
                  bind:value={hook.contentType}
                  original={prev?.contentType}
                  disabled={!canWrite || row.envOnly}
                  envVar={envField(envPrefix, slug, 'CONTENT_TYPE')}
                />
              </Col>
            {/if}
          {/if}
          <Col md="12">
            <FormGroup>
              <Label>{$_('config.hooks.events.label')}</Label>
              <FormText class="d-block mb-2"
                >{$_('config.hooks.events.description')}</FormText
              >
              {#each EXTRACT_STATUSES as st, si (st.value)}
                <FormCheck
                  inline
                  id={`${section}-${row.id}-ev-${st.value}`}
                  label={$_('status.' + (statusKeys[si] ?? st.label))}
                  checked={eventOn(hook, st)}
                  disabled={!canWrite ||
                    row.envOnly ||
                    envHas(envField(envPrefix, slug, 'EVENTS'))}
                  on:change={(e) =>
                    toggleEvent(
                      hook,
                      st,
                      (e.currentTarget as HTMLInputElement).checked,
                    )}
                />
              {/each}
            </FormGroup>
          </Col>
          <Col md="12">
            <FormGroup>
              <Label>{$_('config.hooks.exclude.label')}</Label>
              <FormText class="d-block mb-2"
                >{$_('config.hooks.exclude.description')}</FormText
              >
              {#each hookApps as app (app.value)}
                {@const names = namesFor(app.value)}
                <FormCheck
                  inline
                  id={`${section}-${row.id}-ex-${app.value}`}
                  label={app.name}
                  checked={dialectOn(hook, app.value)}
                  disabled={!canWrite ||
                    row.envOnly ||
                    envHas(envField(envPrefix, slug, 'EXCLUDE'))}
                  on:change={(e) =>
                    toggleDialect(
                      hook,
                      app.value,
                      names,
                      (e.currentTarget as HTMLInputElement).checked,
                    )}
                />
                {#each names as inst (inst)}
                  <FormCheck
                    inline
                    id={`${section}-${row.id}-ex-${app.value}-${inst}`}
                    label={inst}
                    checked={nameOn(hook, app.value, inst)}
                    disabled={!canWrite ||
                      row.envOnly ||
                      envHas(envField(envPrefix, slug, 'EXCLUDE'))}
                    on:change={(e) =>
                      toggleName(
                        hook,
                        app.value,
                        names,
                        inst,
                        (e.currentTarget as HTMLInputElement).checked,
                      )}
                  />
                {/each}
              {/each}
            </FormGroup>
          </Col>
          {#if !isCmd}
            <Col md="12">
              <Button
                type="button"
                size="sm"
                color="link"
                class="px-0"
                aria-expanded={isAdvancedOpen(row)}
                on:click={() => toggleAdvanced(row)}
                >{$_('config.hooks.advanced')}</Button
              >
              <Collapse isOpen={isAdvancedOpen(row)}>
                <Row class="g-2 mt-1">
                  <Col md="6">
                    <Input
                      id={`${section}-${row.id}-template`}
                      helpKey="config.hooks.template"
                      type="select"
                      label={$_('config.hooks.template.label')}
                      bind:value={hook.template}
                      original={prev?.template}
                      disabled={!canWrite || row.envOnly}
                      envVar={envField(envPrefix, slug, 'TEMPLATE')}
                      options={templateChoices}
                    />
                  </Col>
                  <Col md="6">
                    <Input
                      id={`${section}-${row.id}-tmplpath`}
                      helpKey="config.hooks.templatePath"
                      label={$_('config.hooks.templatePath.label')}
                      bind:value={hook.templatePath}
                      original={prev?.templatePath}
                      disabled={!canWrite || row.envOnly}
                      envVar={envField(envPrefix, slug, 'TEMPLATE_PATH')}
                      browse="file"
                      disableMkdir
                    >
                      {#snippet post()}
                        {#if canCreateTemplate && !row.envOnly && !envHas(envField(envPrefix, slug, 'TEMPLATE_PATH'))}
                          <Button
                            type="button"
                            color="primary"
                            outline
                            on:click={() => (createRow = row)}
                          >
                            {$_('buttons.Create')}
                          </Button>
                        {/if}
                      {/snippet}
                    </Input>
                  </Col>
                </Row>
              </Collapse>
            </Col>
          {/if}
        </Row>
      </CardBody>
    </Card>
  {/each}

  {#if canWrite}
    <SaveBar>
      <Button color="primary" disabled={saving || invalid} on:click={save}>
        {#if saving}<Spinner size="sm" />{/if}
        <span class="ms-1">{$_('buttons.Save')}</span>
      </Button>
      <Button color="secondary" outline on:click={add}>
        {isCmd
          ? $_('pages.settings.AddCmdhook')
          : $_('pages.settings.AddWebhook')}
      </Button>
    </SaveBar>
  {/if}
{/if}

<Modal isOpen={testOpen} toggle={closeHookTest}>
  <ModalHeader toggle={closeHookTest}
    >{$_('phrases.TestTitle', { values: { name: testName } })}</ModalHeader
  >
  <ModalBody>
    <FormGroup>
      <Label for="hook-test-event">{$_('phrases.TestEvent')}</Label>
      <select id="hook-test-event" class="form-select" bind:value={testEvent}>
        {#each EXTRACT_STATUSES as st, si (st.value)}
          <option value={st.id}
            >{$_('status.' + (statusKeys[si] ?? st.label))}</option
          >
        {/each}
      </select>
    </FormGroup>
    <FormGroup>
      <Label for="hook-test-app">{$_('phrases.TestApp')}</Label>
      <select id="hook-test-app" class="form-select" bind:value={testApp}>
        {#each hookAppChoices as choice (choice.value)}
          <option value={choice.value}>{choice.name}</option>
        {/each}
      </select>
    </FormGroup>
    {#if testBusy}
      <Spinner color="primary" size="sm" />
    {:else if testError}
      <p class="text-danger mb-0">{testError}</p>
      {#if testElapsed}
        <p class="text-muted mb-0 mt-2">
          {$_('phrases.TestDuration')}: {testElapsed}
        </p>
      {/if}
    {:else if testResult}
      {#if testElapsed}
        <p class="mb-1">{$_('phrases.TestDuration')}: {testElapsed}</p>
      {/if}
      {#if testResult.reply}
        <p class="mb-1">{$_('phrases.TestReply')}</p>
        <pre class="mb-0 small text-break">{testResult.reply}</pre>
      {:else}
        <p class="text-success mb-0">{testResult.status}</p>
      {/if}
    {/if}
  </ModalBody>
  <ModalFooter>
    <Button color="warning" on:click={closeHookTest}
      >{$_('buttons.Close')}</Button
    >
    <Button color="success" disabled={testBusy} on:click={runHookTest}
      >{$_('buttons.Test')}</Button
    >
  </ModalFooter>
</Modal>

<Modal isOpen={guideOpen} toggle={closeGuide}>
  <ModalHeader toggle={closeGuide}>{guideTitle(guidePick)}</ModalHeader>
  <ModalBody>
    <FormGroup>
      <Label for="hook-guide-pick">{$_('config.hooks.guide.pick')}</Label>
      <select id="hook-guide-pick" class="form-select" bind:value={guidePick}>
        {#each guideChoices as choice (choice.value)}
          <option value={choice.value}>{choice.name}</option>
        {/each}
      </select>
    </FormGroup>
    {#each guideParagraphs(guidePick) as para, i (i)}
      <p class="mt-2 mb-0">
        {#each inlineParts(para) as part, j (`${i}-${j}`)}
          {#if part.code}<code>{part.text}</code>{:else}{part.text}{/if}
        {/each}
      </p>
    {/each}
    <p class="mt-3 mb-0">{$_('config.hooks.guide.test')}</p>
  </ModalBody>
  <ModalFooter>
    <Button color="warning" on:click={closeGuide}
      >{$_('buttons.Close')}</Button
    >
  </ModalFooter>
</Modal>

{#if createRow}
  <CreateTemplate
    startPath={createRow.value.templatePath}
    startTemplate={createRow.value.template}
    oncreated={createdTemplate}
    onclose={closeCreate}
  />
{/if}
