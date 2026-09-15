<script lang="ts">
  import { onMount } from 'svelte'
  import {
    Button,
    Card,
    CardBody,
    CardHeader,
    Col,
    Row,
    Spinner,
  } from '@sveltestrap/sveltestrap'
  import Input from '../../components/Input.svelte'
  import SaveBar from '../../components/SaveBar.svelte'
  import { _ } from '../../lib/i18n/Translate.svelte'
  import { loadSection, loadSectionLive, saveSection } from '../../lib/config'
  import { has } from '../../lib/auth.svelte'
  import { configPerm } from '../../lib/perms'
  import { deepCopy, deepEqual } from '../../lib/util'
  import { trackDirty } from '../../lib/dirty.svelte'
  import { envHas } from '../../lib/env.svelte'
  import {
    requiredPathError,
    slugDuplicateError,
    slugError,
  } from '../../lib/validate'
  import { failure } from '../../lib/toast'
  import type { FoldersSection, FolderConfig } from '../../lib/types'
  import {
    envField,
    FOLDER_ENV_FIELDS,
    instanceMap,
    mergeEnvOnlyRows,
    newRow,
    omitEnvFields,
    rowsFromMap,
    slugifyPath,
    uniqueSlug,
    type InstanceRow,
  } from '../../lib/slug'

  let interval = $state('5s')
  let buffer = $state(20000)
  let origInterval = $state('5s')
  let origBuffer = $state(20000)
  let rows = $state<InstanceRow<FolderConfig>[]>([])
  let origFolder = $state<Record<string, FolderConfig>>({})
  let excludeText = $state<Record<string, string>>({})
  let origExclude = $state<Record<string, string>>({})
  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')

  const canWrite = has(configPerm('folders', 'write'))
  const defaultFolderBuf = 20000
  const envPrefix = 'FOLDER'
  const invalid = $derived(
    rows.some((row) => {
      if (row.envOnly) return false
      const others = rows
        .filter((r) => r.id !== row.id)
        .map((r) => r.slug)
        .filter(Boolean)
      if (slugError(row.slug) || slugDuplicateError(row.slug, others))
        return true
      return (
        !envHas(envField(envPrefix, row.slug, 'PATH')) &&
        !!requiredPathError(row.value.path)
      )
    }),
  )

  function blank(): FolderConfig {
    return {
      path: '',
      extract_path: '',
      delete_original: false,
      delete_files: false,
      disable_log: false,
      move_back: false,
      delete_after: '',
      extract_isos: false,
      disableRecursion: false,
      maxNested: 0,
      extrasMaxDepth: 0,
      allowSymlinks: false,
      maxBytes: '0',
      maxFiles: 0,
      maxRatio: 0,
      exclude_paths: [],
    }
  }

  function defaultBuffer(n: unknown): number {
    const v = Math.trunc(Number(n))
    return !Number.isFinite(v) || v === 0 ? defaultFolderBuf : v
  }

  function normalizeOne(f: FolderConfig): FolderConfig {
    return {
      ...blank(),
      ...f,
      path: f.path ?? '',
      extract_path: f.extract_path ?? '',
      delete_after: f.delete_after ?? '',
      maxBytes: f.maxBytes || '0',
      // File GET encodes a missing list as null; the textarea always yields [].
      exclude_paths: f.exclude_paths ?? [],
    }
  }

  function taken(row: InstanceRow<FolderConfig>): string[] {
    return rows.filter((r) => r.id !== row.id).map((r) => r.slug).filter(Boolean)
  }

  function setPath(row: InstanceRow<FolderConfig>, path: string) {
    row.value.path = path
    if (row.locked || !row.autoSlug) return
    const s = slugifyPath(path)
    row.slug = s ? uniqueSlug(s, taken(row)) : row.slug
  }

  function setSlug(row: InstanceRow<FolderConfig>, slug: string) {
    row.slug = slug
    row.autoSlug = false
  }

  function currentFolderMap(): Record<string, FolderConfig> {
    const out: Record<string, FolderConfig> = {}
    for (const row of rows) {
      if (row.envOnly) continue
      out[row.slug || row.id] = {
        ...row.value,
        exclude_paths: (excludeText[row.id] ?? '')
          .split('\n')
          .map((s) => s.trim())
          .filter(Boolean),
      }
    }
    return out
  }

  function saveFolderMap(): Record<string, FolderConfig> {
    const out: Record<string, FolderConfig> = {}
    for (const row of rows) {
      if (row.envOnly || !row.slug) continue
      const item: FolderConfig = {
        ...row.value,
        delete_after: row.value.delete_after || null,
        exclude_paths: (excludeText[row.id] ?? '')
          .split('\n')
          .map((s) => s.trim())
          .filter(Boolean),
      }
      out[row.slug] = omitEnvFields(envPrefix, row.slug, item, FOLDER_ENV_FIELDS)
    }
    return out
  }

  onMount(async () => {
    const { data, error: err } = await loadSection<FoldersSection>('folders')
    if (err) error = err
    else {
      const loaded = data ?? { interval: '5s', buffer: 20000, folder: {} }
      interval = loaded.interval
      buffer = defaultBuffer(loaded.buffer)
      origInterval = interval
      origBuffer = buffer
      const map = instanceMap<FolderConfig>(loaded.folder)
      const normalized: Record<string, FolderConfig> = {}
      const ex: Record<string, string> = {}
      for (const [slug, f] of Object.entries(map)) {
        normalized[slug] = normalizeOne(f)
        ex[slug] = (f.exclude_paths ?? []).join('\n')
      }
      let next = rowsFromMap(normalized)
      const live = await loadSectionLive<FoldersSection>('folders')
      if (live.data != null) {
        next = mergeEnvOnlyRows(
          next,
          Object.keys(instanceMap(live.data.folder)),
          blank,
        )
      }
      rows = next
      excludeText = Object.fromEntries(rows.map((r) => [r.id, ex[r.slug] ?? '']))
      origExclude = deepCopy(excludeText)
      origFolder = deepCopy(currentFolderMap())
    }

    loading = false
  })

  trackDirty(
    () =>
      !loading &&
      (interval !== origInterval ||
        buffer !== origBuffer ||
        !deepEqual(currentFolderMap(), origFolder) ||
        JSON.stringify(excludeText) !== JSON.stringify(origExclude)),
  )

  function add() {
    const row = newRow(blank())
    rows = [...rows, row]
    excludeText = { ...excludeText, [row.id]: '' }
  }

  function remove(id: string) {
    rows = rows.filter((r) => r.id !== id)
    const next = { ...excludeText }
    delete next[id]
    excludeText = next
  }

  async function save() {
    if (invalid) {
      failure($_('phrases.FixInvalidFields'))
      return
    }

    saving = true
    const payload: FoldersSection = {
      interval,
      buffer,
      folder: saveFolderMap(),
    }

    const ok = await saveSection('folders', payload)
    if (ok) {
      for (const row of rows) {
        if (row.slug) {
          row.locked = true
          row.autoSlug = false
        }
      }
      origInterval = interval
      origBuffer = buffer
      origFolder = deepCopy(currentFolderMap())
      origExclude = deepCopy(excludeText)
    }

    saving = false
  }
</script>

{#if loading}
  <Spinner color="primary" />
{:else if error}
  <p class="text-danger">{error}</p>
{:else}
  <Card class="mb-3">
    <CardBody>
      <Row class="g-2">
        <Col md="6">
          <Input
            id="config.folders.interval"
            type="folderpoll"
            bind:value={interval}
            original={origInterval}
            disabled={!canWrite}
            envVar="FOLDERS_INTERVAL"
          />
        </Col>
        <Col md="6">
          <Input
            id="config.folders.buffer"
            type="number"
            bind:value={buffer}
            original={origBuffer}
            disabled={!canWrite}
            envVar="FOLDERS_BUFFER"
          />
        </Col>
      </Row>
    </CardBody>
  </Card>

  {#if rows.length === 0}
    <p class="text-muted">{$_('phrases.NoFolders')}</p>
  {/if}

  {#snippet heading(text: string)}
    <Col xs="12">
      <div class="folder-section">{text}</div>
    </Col>
  {/snippet}

  {#each rows as row (row.id)}
    {@const folder = row.value}
    {@const slug = row.slug}
    {@const prev = origFolder[slug]}
    {@const envOnly = row.envOnly}
    <Card class="mb-3">
      <CardHeader>
        <Row class="align-items-center">
          <Col>
            <span class="fw-semibold"
              >{slug || $_('pages.settings.AddFolder')}</span
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
              id={`folder-${row.id}-slug`}
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
              id={`folder-${row.id}-path`}
              helpKey="config.folders.path"
              label={$_('config.folders.path.label')}
              bind:value={() => folder.path, (v) => setPath(row, v)}
              original={prev?.path}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'PATH')}
              browse="dir"
              validate={(_id, v) => requiredPathError(v)}
            />
          </Col>
          <Col md="12">
            <Input
              id={`folder-${row.id}-extract`}
              helpKey="config.folders.extract_path"
              label={$_('config.folders.extract_path.label')}
              bind:value={folder.extract_path}
              original={prev?.extract_path}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'EXTRACT_PATH')}
              browse="dir"
            />
          </Col>
          {@render heading($_('pages.settings.folders.AfterExtract'))}
          <Col md="6">
            <Input
              id={`folder-${row.id}-delete-after`}
              helpKey="config.folders.delete_after"
              type="nullable"
              label={$_('config.folders.delete_after.label')}
              bind:value={folder.delete_after}
              original={prev?.delete_after ?? ''}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'DELETE_AFTER')}
            />
          </Col>
          <Col md="6">
            <Input
              id={`folder-${row.id}-delete-orig`}
              helpKey="config.folders.delete_original"
              type="select"
              label={$_('config.folders.delete_original.label')}
              bind:value={folder.delete_original}
              original={prev?.delete_original}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'DELETE_ORIGINAL')}
            />
          </Col>
          <Col md="6">
            <Input
              id={`folder-${row.id}-delete-files`}
              helpKey="config.folders.delete_files"
              type="select"
              label={$_('config.folders.delete_files.label')}
              bind:value={folder.delete_files}
              original={prev?.delete_files}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'DELETE_FILES')}
            />
          </Col>
          <Col md="6">
            <Input
              id={`folder-${row.id}-move-back`}
              helpKey="config.folders.move_back"
              type="select"
              label={$_('config.folders.move_back.label')}
              bind:value={folder.move_back}
              original={prev?.move_back}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'MOVE_BACK')}
            />
          </Col>
          {@render heading($_('pages.settings.folders.Extraction'))}
          <Col md="6">
            <Input
              id={`folder-${row.id}-isos`}
              helpKey="config.folders.extract_isos"
              type="select"
              label={$_('config.folders.extract_isos.label')}
              bind:value={folder.extract_isos}
              original={prev?.extract_isos}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'EXTRACT_ISOS')}
            />
          </Col>
          <Col md="6">
            <Input
              id={`folder-${row.id}-norecurse`}
              helpKey="config.folders.disableRecursion"
              type="select"
              invert
              label={$_('config.folders.disableRecursion.label')}
              bind:value={folder.disableRecursion}
              original={prev?.disableRecursion}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'DISABLE_RECURSION')}
            />
          </Col>
          <Col md="12">
            <Input
              id={`folder-${row.id}-exclude`}
              helpKey="config.folders.exclude_paths"
              type="textarea"
              rows={2}
              label={$_('config.folders.exclude_paths.label')}
              bind:value={excludeText[row.id]}
              original={(prev?.exclude_paths ?? []).join('\n')}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'EXCLUDE_PATH_')}
            />
          </Col>
          {@render heading($_('pages.settings.folders.Protection'))}
          <Col md="6">
            <Input
              id={`folder-${row.id}-symlinks`}
              helpKey="config.folders.allowSymlinks"
              type="select"
              label={$_('config.folders.allowSymlinks.label')}
              bind:value={folder.allowSymlinks}
              original={prev?.allowSymlinks}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'ALLOW_SYMLINKS')}
            />
          </Col>
          <Col md="6">
            <Input
              id={`folder-${row.id}-nested`}
              helpKey="config.folders.maxNested"
              type="number"
              label={$_('config.folders.maxNested.label')}
              bind:value={folder.maxNested}
              original={prev?.maxNested}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'MAX_NESTED')}
            />
          </Col>
          <Col md="6">
            <Input
              id={`folder-${row.id}-extras`}
              helpKey="config.folders.extrasMaxDepth"
              type="number"
              label={$_('config.folders.extrasMaxDepth.label')}
              bind:value={folder.extrasMaxDepth}
              original={prev?.extrasMaxDepth}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'EXTRAS_MAX_DEPTH')}
            />
          </Col>
          <Col md="6">
            <Input
              id={`folder-${row.id}-bytes`}
              helpKey="config.folders.maxBytes"
              type="maxbytes"
              label={$_('config.folders.maxBytes.label')}
              bind:value={folder.maxBytes}
              original={prev?.maxBytes ?? '0'}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'MAX_BYTES')}
            />
          </Col>
          <Col md="6">
            <Input
              id={`folder-${row.id}-files`}
              helpKey="config.folders.maxFiles"
              type="number"
              label={$_('config.folders.maxFiles.label')}
              bind:value={folder.maxFiles}
              original={prev?.maxFiles}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'MAX_FILES')}
            />
          </Col>
          <Col md="6">
            <Input
              id={`folder-${row.id}-ratio`}
              helpKey="config.folders.maxRatio"
              type="number"
              step="0.1"
              label={$_('config.folders.maxRatio.label')}
              bind:value={folder.maxRatio}
              original={prev?.maxRatio}
              disabled={!canWrite || row.envOnly}
              envVar={envField(envPrefix, slug, 'MAX_RATIO')}
            />
          </Col>
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
      <Button color="secondary" outline on:click={add}
        >{$_('pages.settings.AddFolder')}</Button
      >
    </SaveBar>
  {/if}
{/if}

<style>
  .folder-section {
    color: var(--bs-secondary-color, #6c757d);
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    margin: 0.25rem 0 0;
    padding-top: 0.65rem;
    border-top: 1px solid var(--bs-border-color);
  }
</style>
