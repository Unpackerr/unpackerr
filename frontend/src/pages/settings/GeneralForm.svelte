<script lang="ts">
  import { onMount } from 'svelte'
  import {
    Button,
    Card,
    CardBody,
    Col,
    Row,
    Spinner,
  } from '@sveltestrap/sveltestrap'
  import Input from '../../components/Input.svelte'
  import SaveBar from '../../components/SaveBar.svelte'
  import { _ } from '../../lib/i18n/Translate.svelte'
  import { loadSection, saveSection } from '../../lib/config'
  import { has, profile } from '../../lib/auth.svelte'
  import { configPerm } from '../../lib/perms'
  import { deepCopy, deepEqual } from '../../lib/util'
  import { trackDirty } from '../../lib/dirty.svelte'
  import type { GeneralConfig } from '../../lib/types'

  let cfg = $state<GeneralConfig | null>(null)
  let orig = $state<GeneralConfig | null>(null)
  let passwordsText = $state('')
  let origPasswords = $state('')
  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')

  const canWrite = has(configPerm('general', 'write'))
  const unixModes = $derived(profile.info?.goos !== 'windows')
  const parallelChoices = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((n) => ({
    value: n,
    name: String(n),
  }))
  const retryChoices = [0, 1, 2, 3, 4, 5, 6].map((n) => ({
    value: n,
    name: String(n),
  }))
  const historyChoices = $derived(
    [0, 5, 10, 20, 50, 100, 200, 300, 500, 750, 1000, 1500, 2000].map((n) => ({
      value: n,
      name: n === 0 ? $_('config.general.keepHistory.options.0') : String(n),
    })),
  )
  const logFileCountChoices = $derived(
    [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 20, 30, 40, 50].map((n) => ({
      value: n,
      name: n === 0 ? $_('words.select-option.NoRotation') : String(n),
    })),
  )
  const logFileMbChoices = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 20].map((n) => ({
    value: n,
    name: String(n),
  }))
  const fileModeChoices = ['0600', '0640', '0644', '0660', '0664', '0666'].map(
    modeOption,
  )
  const dirModeChoices = ['0700', '0750', '0755', '0770', '0775', '0777'].map(
    modeOption,
  )

  function rwx(bits: number): string {
    return (
      (bits & 4 ? 'r' : '-') + (bits & 2 ? 'w' : '-') + (bits & 1 ? 'x' : '-')
    )
  }

  function modeOption(octal: string): { value: string; name: string } {
    const n = Number.parseInt(octal, 8)
    return {
      value: octal,
      name: `${octal} (${rwx((n >> 6) & 7)}${rwx((n >> 3) & 7)}${rwx(n & 7)})`,
    }
  }

  function clampParallel(n: unknown): number {
    const v = Math.trunc(Number(n))
    return !Number.isFinite(v) || v < 1 ? 1 : v
  }

  function defaultLogMb(n: unknown): number {
    const v = Math.trunc(Number(n))
    return !Number.isFinite(v) || v === 0 ? 10 : v
  }

  function defaultMode(v: string | undefined, fallback: string): string {
    const raw = (v ?? '').trim() || fallback
    const n = Number.parseInt(raw, 8)
    if (!Number.isFinite(n) || n < 0) return fallback
    return (n & 0o777).toString(8).padStart(4, '0')
  }

  function defaultProgress(v: string | undefined): string {
    const s = (v ?? '').trim()
    return !s || s === '0s' ? '15s' : s
  }

  trackDirty(
    () =>
      !loading &&
      cfg != null &&
      orig != null &&
      (!deepEqual(cfg, orig) || passwordsText !== origPasswords),
  )

  onMount(async () => {
    const { data, error: err } = await loadSection<GeneralConfig>('general')
    if (err) error = err
    else {
      cfg = data!
      orig = deepCopy(data!)
      cfg.parallel = clampParallel(cfg.parallel)
      orig.parallel = clampParallel(orig.parallel)
      cfg.logFileMb = defaultLogMb(cfg.logFileMb)
      orig.logFileMb = defaultLogMb(orig.logFileMb)
      cfg.fileMode = defaultMode(cfg.fileMode, '0644')
      orig.fileMode = defaultMode(orig.fileMode, '0644')
      cfg.dirMode = defaultMode(cfg.dirMode, '0755')
      orig.dirMode = defaultMode(orig.dirMode, '0755')
      cfg.progress = defaultProgress(cfg.progress)
      orig.progress = defaultProgress(orig.progress)
      passwordsText = (cfg.passwords ?? []).join('\n')
      origPasswords = passwordsText
    }
    loading = false
  })

  async function save() {
    if (!cfg) return
    saving = true
    cfg.passwords = passwordsText
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean)
    cfg.parallel = clampParallel(cfg.parallel)
    const ok = await saveSection('general', cfg)
    if (ok) {
      orig = deepCopy(cfg)
      origPasswords = passwordsText
    }
    saving = false
  }
</script>

{#if loading}
  <Spinner color="primary" />
{:else if error}
  <p class="text-danger">{error}</p>
{:else if cfg && orig}
  {#snippet heading(text: string, first = false)}
    <Col xs="12">
      <div class="settings-section" class:settings-section-first={first}>
        {text}
      </div>
    </Col>
  {/snippet}

  <Card>
    <CardBody>
      <Row class="g-2">
        {@render heading($_('pages.settings.general.Logging'), true)}
        <Col md="12">
          <Input
            id="config.general.logFile"
            bind:value={cfg.logFile}
            original={orig.logFile}
            disabled={!canWrite}
            envVar="LOG_FILE"
            browse="dir"
            browseFile="unpackerr.log"
          />
        </Col>
        <Col md="6">
          <Input
            id="config.general.logFiles"
            type="select"
            bind:value={cfg.logFiles}
            original={orig.logFiles}
            disabled={!canWrite}
            envVar="LOG_FILES"
            options={logFileCountChoices}
          />
        </Col>
        <Col md="6">
          <Input
            id="config.general.logFileMb"
            type="select"
            bind:value={cfg.logFileMb}
            original={orig.logFileMb}
            disabled={!canWrite}
            envVar="LOG_FILE_MB"
            options={logFileMbChoices}
          />
        </Col>
        <Col md="6">
          <Input
            id="config.general.debug"
            type="select"
            bind:value={cfg.debug}
            original={orig.debug}
            disabled={!canWrite}
            envVar="DEBUG"
          />
        </Col>
        <Col md="6">
          <Input
            id="config.general.quiet"
            type="select"
            bind:value={cfg.quiet}
            original={orig.quiet}
            disabled={!canWrite}
            envVar="QUIET"
          />
        </Col>
        <Col md="6">
          <Input
            id="config.general.errorStderr"
            type="select"
            bind:value={cfg.errorStderr}
            original={orig.errorStderr}
            disabled={!canWrite}
            envVar="ERROR_STDERR"
          />
        </Col>
        <Col md="6">
          <Input
            id="config.general.progress"
            type="short"
            bind:value={cfg.progress}
            original={orig.progress}
            disabled={!canWrite}
            envVar="PROGRESS"
          />
        </Col>
        {@render heading($_('pages.settings.general.Extraction'))}
        <Col md="6">
          <Input
            id="config.general.parallel"
            type="select"
            bind:value={cfg.parallel}
            original={orig.parallel}
            disabled={!canWrite}
            envVar="PARALLEL"
            options={parallelChoices}
          />
        </Col>
        <Col md="6">
          <Input
            id="config.general.maxRetries"
            type="select"
            bind:value={cfg.maxRetries}
            original={orig.maxRetries}
            disabled={!canWrite}
            envVar="MAX_RETRIES"
            options={retryChoices}
          />
        </Col>
        <Col md="6">
          <Input
            id="config.general.startDelay"
            type="delay"
            bind:value={cfg.startDelay}
            original={orig.startDelay}
            disabled={!canWrite}
            envVar="START_DELAY"
          />
        </Col>
        <Col md="6">
          <Input
            id="config.general.retryDelay"
            type="delay"
            bind:value={cfg.retryDelay}
            original={orig.retryDelay}
            disabled={!canWrite}
            envVar="RETRY_DELAY"
          />
        </Col>
        <Col md="6">
          <Input
            id="config.general.deleteDelay"
            type="deletedelay"
            bind:value={cfg.deleteDelay}
            original={orig.deleteDelay}
            disabled={!canWrite}
            envVar="DELETE_DELAY"
          />
        </Col>
        <Col md="6">
          <Input
            id="config.general.keepHistory"
            type="select"
            bind:value={cfg.keepHistory}
            original={orig.keepHistory}
            disabled={!canWrite}
            envVar="KEEP_HISTORY"
            options={historyChoices}
          />
        </Col>
        {#if unixModes}
          <Col md="6">
            <Input
              id="config.general.fileMode"
              type="select"
              bind:value={cfg.fileMode}
              original={orig.fileMode}
              disabled={!canWrite}
              envVar="FILE_MODE"
              options={fileModeChoices}
            />
          </Col>
          <Col md="6">
            <Input
              id="config.general.dirMode"
              type="select"
              bind:value={cfg.dirMode}
              original={orig.dirMode}
              disabled={!canWrite}
              envVar="DIR_MODE"
              options={dirModeChoices}
            />
          </Col>
        {/if}
        <Col md="12">
          <Input
            id="config.general.passwords"
            type="textarea"
            rows={4}
            bind:value={passwordsText}
            original={origPasswords}
            disabled={!canWrite}
            envVar="PASSWORD_"
          />
        </Col>
        <Col md="12">
          <Input
            id="config.general.remnantAction"
            type="select"
            bind:value={cfg.remnantAction}
            original={orig.remnantAction}
            disabled={!canWrite}
            envVar="REMNANT_ACTION"
          >
            <option value="rename"
              >{$_('config.general.remnantAction.rename')}</option
            >
            <option value="delete"
              >{$_('config.general.remnantAction.delete')}</option
            >
            <option value="off">{$_('config.general.remnantAction.off')}</option
            >
          </Input>
        </Col>
      </Row>
    </CardBody>
  </Card>
  {#if canWrite}
    <SaveBar>
      <Button color="primary" disabled={saving} on:click={save}>
        {#if saving}<Spinner size="sm" />{/if}
        <span class="ms-1">{$_('buttons.Save')}</span>
      </Button>
    </SaveBar>
  {/if}
{/if}

<style>
  .settings-section {
    color: var(--bs-secondary-color, #6c757d);
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    margin: 0.25rem 0 0;
    padding-top: 0.65rem;
    border-top: 1px solid var(--bs-border-color);
  }

  .settings-section-first {
    border-top: none;
    padding-top: 0;
    margin-top: 0;
  }
</style>
