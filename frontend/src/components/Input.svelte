<!-- Adapted from Notifiarr Input.svelte. Env overlay uses UN_ keys from GET /api/config/env. -->
<script lang="ts">
  import {
    Badge,
    Button,
    Card,
    FormFeedback,
    FormGroup,
    FormText,
    Input,
    InputGroup,
    Label,
    type InputType,
  } from '@sveltestrap/sveltestrap'
  import Eye from 'phosphor-svelte/lib/Eye'
  import EyeSlash from 'phosphor-svelte/lib/EyeSlash'
  import WarningCircle from 'phosphor-svelte/lib/WarningCircle'
  import { type Snippet } from 'svelte'
  import { slide } from 'svelte/transition'
  import Icon from '../components/Icon.svelte'
  import T, { _ } from '../lib/i18n/Translate.svelte'
  import { deepEqual, helpToHtml, helpToInline } from '../lib/util'
  import { fieldHelp } from '../lib/help.svelte'
  import { envHas, envLiveValue } from '../lib/env.svelte'
  import { has } from '../lib/auth.svelte'
  import { systemPerm } from '../lib/perms'
  import BButton from './fileBrowser/BButton.svelte'
  import BModal from './fileBrowser/BModal.svelte'

  type DurationKind =
    | 'interval'
    | 'timeout'
    | 'delay'
    | 'deletedelay'
    | 'short'
    | 'logqueue'
    | 'folderpoll'
    | 'nullable'
    | 'maxbytes'

  interface Props {
    id: string
    label?: string
    placeholder?: string
    description?: string
    type?: InputType | DurationKind
    tooltip?: string
    /** Look up env/short from GET /api/config/help (definitions.yml). */
    helpKey?: string
    value?: any
    original?: any
    showChanged?: boolean
    badge?: string
    options?: Option[] | undefined
    validate?: (id: string, value: any) => string | undefined
    pre?: Snippet
    post?: Snippet
    children?: Snippet
    msg?: Snippet
    noDisable?: boolean
    invert?: boolean
    /** Suffix after UN_, e.g. DEBUG or SONARR_0_URL. */
    envVar?: string
    /** Open a file/folder picker on this host (hidden without system:browse:read). */
    browse?: 'file' | 'dir'
    /** Folder picker that writes folder + this filename (keeps an existing basename). */
    browseFile?: string
    /** Hide create-folder in the picker. File pickers are picker-only anyway. */
    disableMkdir?: boolean
    [key: string]: any
  }

  let {
    id,
    label,
    placeholder = $bindable(),
    description,
    type = 'text',
    tooltip,
    helpKey,
    value = $bindable(undefined),
    original = value,
    showChanged = true,
    options = undefined,
    validate,
    pre,
    children,
    badge = '',
    post,
    msg,
    noDisable = true,
    invert,
    envVar,
    class: restClass,
    disabled = false,
    browse,
    disableMkdir = false,
    browseFile = '',
    ...rest
  }: Props = $props()

  type Option = {
    value: string | number | boolean
    name: string
    disabled?: boolean
  }

  const durationTypes: DurationKind[] = [
    'interval',
    'timeout',
    'delay',
    'deletedelay',
    'short',
    'logqueue',
    'folderpoll',
    'nullable',
    'maxbytes',
  ]

  const i18nKeys = $derived(
    [helpKey, id.startsWith('config.') ? id : ''].filter(
      (k, i, a) => !!k && a.indexOf(k) === i,
    ),
  )

  const translated = (suffix: string, override?: string) => {
    if (override !== undefined) {
      if (i18nKeys.some((k) => override === `${k}.${suffix}`)) return ''
      if (override === `${id}.${suffix}`) return ''
      return override
    }
    for (const key of i18nKeys) {
      const k = `${key}.${suffix}`
      const text = $_(k)
      if (text && text !== k) return text
    }
    return ''
  }

  let showTooltip = $state(false)
  let revealed = $state(false)
  const hasEnv = $derived(envHas(envVar))
  let changed = $derived(
    showChanged && !hasEnv && original !== null && !deepEqual(value, original),
  )
  const currType = $derived(
    durationTypes.includes(type as DurationKind)
      ? 'select'
      : type === 'password' && revealed
        ? 'text'
        : type,
  )
  const passIcon = $derived(revealed ? EyeSlash : Eye)
  const helpEntry = $derived(
    fieldHelp(helpKey ?? (id.startsWith('config.') ? id : undefined)),
  )
  const feedback = $derived(hasEnv ? '' : (validate?.(id, value) ?? ''))
  const i18nDesc = $derived(translated('description', description))
  const i18nTip = $derived(translated('tooltip', tooltip))
  const i18nConfig = $derived(translated('config'))
  const labelText = $derived(label ?? (translated('label') || id))
  const placeholderText = $derived(translated('placeholder', placeholder))
  /** One-liner under the field: i18n description, else YAML short. */
  const descriptionText = $derived(i18nDesc || helpEntry?.short || '')
  /** How-it-works copy in the help card. Does not restate the description. */
  const tooltipText = $derived(i18nTip)
  const typeConfigKey = $derived.by(() => {
    switch (type) {
      case 'interval':
      case 'delay':
      case 'deletedelay':
      case 'short':
        return 'phrases.ConfigDuration'
      case 'timeout':
        return noDisable
          ? 'phrases.ConfigDuration'
          : 'phrases.ConfigDurationDisabled'
      case 'logqueue':
        return 'phrases.ConfigDurationOff'
      case 'folderpoll':
        return 'phrases.ConfigDurationFolderPoll'
      case 'nullable':
        return 'phrases.ConfigDurationUnset'
      case 'maxbytes':
        return 'phrases.ConfigSize'
      default:
        return ''
    }
  })
  const configHintText = $derived.by(() => {
    const parts: string[] = []
    if (typeConfigKey) {
      const text = $_(typeConfigKey)
      if (text && text !== typeConfigKey) parts.push(text)
    }
    if (i18nConfig) parts.push(i18nConfig)
    return parts.join('\n\n')
  })
  const tooltipHtml = $derived(helpToHtml(tooltipText))
  const configHintHtml = $derived(
    configHintText ? helpToHtml(configHintText) : '',
  )
  const descriptionHtml = $derived(helpToInline(descriptionText))
  const showHelp = $derived(!!(tooltipText || configHintText || envVar))
  const inputClass = $derived(
    feedback ? 'is-invalid' : changed ? 'is-valid' : '',
  )
  /** Indexed slices: YAML envvar ends with `_`, or the name already has `#`/`*`. Textareas are lists too. */
  const envIsList = $derived(
    type === 'textarea' ||
      (!!envVar &&
        (envVar.endsWith('_') || envVar.includes('#') || envVar.includes('*'))),
  )
  const env = $derived(envVar ? formatUnEnv(envVar, envIsList) : '')
  const liveText = $derived(envLiveValue(envVar) ?? '')
  /** Env-locked file values are often empty; show the running value in the control. */
  const shownValue = $derived.by(() => {
    if (!hasEnv || liveText === '') return value
    if (typeof value === 'boolean')
      return liveText === 'true' || liveText === '1'
    if (typeof value === 'number') {
      const n = Number(liveText)
      return Number.isFinite(n) ? n : value
    }
    return liveText
  })
  const inverted = $derived(invert ?? id.endsWith('disabled'))
  const isDisabled = $derived(!!disabled || hasEnv)
  let browseOpen = $state(false)
  const canBrowse = $derived(
    !!(browse || browseFile) &&
      !isDisabled &&
      has(systemPerm('browse', 'read')),
  )
  const pickerOnly = $derived(
    disableMkdir || (browse === 'file' && !browseFile),
  )
  const browseDir = $derived(browse === 'dir' || !!browseFile)

  function formatUnEnv(name: string, list: boolean): string {
    // Preserve slug case so the badge shows UN_SONARR_uhd_URL.
    if (name.includes('#') || name.includes('*')) return 'UN_' + name
    if (name.endsWith('_')) return 'UN_' + name + '#'
    if (list) return 'UN_' + name + '_#'
    return 'UN_' + name
  }

  function unit(n: number, singular: string, plural: string): string {
    return n === 1
      ? '1 ' + $_(`words.select-option.${singular}`)
      : `${n} ` + $_(`words.select-option.${plural}`)
  }

  const selectOptions = $derived.by((): Option[] | undefined => {
    if (type === 'interval') {
      return [
        { value: '15s', name: unit(15, 'second', 'seconds') },
        { value: '30s', name: unit(30, 'second', 'seconds') },
        { value: '45s', name: unit(45, 'second', 'seconds') },
        { value: '1m', name: unit(1, 'minute', 'minute') },
        { value: '2m', name: unit(2, 'minute', 'minutes') },
        { value: '3m', name: unit(3, 'minute', 'minutes') },
        { value: '4m', name: unit(4, 'minute', 'minutes') },
        { value: '5m', name: unit(5, 'minute', 'minutes') },
        { value: '10m', name: unit(10, 'minute', 'minutes') },
        { value: '15m', name: unit(15, 'minute', 'minutes') },
        { value: '20m', name: unit(20, 'minute', 'minutes') },
        { value: '30m', name: unit(30, 'minute', 'minutes') },
      ]
    }

    if (type === 'timeout') {
      const opts: Option[] = [
        { value: '5s', name: unit(5, 'second', 'seconds') },
        { value: '10s', name: unit(10, 'second', 'seconds') },
        { value: '15s', name: unit(15, 'second', 'seconds') },
        { value: '20s', name: unit(20, 'second', 'seconds') },
        { value: '30s', name: unit(30, 'second', 'seconds') },
        { value: '45s', name: unit(45, 'second', 'seconds') },
        { value: '1m', name: unit(1, 'minute', 'minute') },
        { value: '90s', name: unit(90, 'second', 'seconds') },
        { value: '2m', name: unit(2, 'minute', 'minutes') },
      ]
      if (!noDisable) {
        opts.unshift({
          value: '-1s',
          name: $_('words.select-option.InstanceDisabled'),
        })
      }
      return opts
    }

    if (type === 'delay') {
      return [
        { value: '1m', name: unit(1, 'minute', 'minute') },
        { value: '2m', name: unit(2, 'minute', 'minutes') },
        { value: '5m', name: unit(5, 'minute', 'minutes') },
        { value: '10m', name: unit(10, 'minute', 'minutes') },
        { value: '15m', name: unit(15, 'minute', 'minutes') },
        { value: '30m', name: unit(30, 'minute', 'minutes') },
      ]
    }

    if (type === 'deletedelay') {
      return [
        { value: '5s', name: unit(5, 'second', 'seconds') },
        { value: '15s', name: unit(15, 'second', 'seconds') },
        { value: '30s', name: unit(30, 'second', 'seconds') },
        { value: '1m', name: unit(1, 'minute', 'minute') },
        { value: '2m', name: unit(2, 'minute', 'minutes') },
        { value: '5m', name: unit(5, 'minute', 'minutes') },
        { value: '10m', name: unit(10, 'minute', 'minutes') },
        { value: '15m', name: unit(15, 'minute', 'minutes') },
        { value: '30m', name: unit(30, 'minute', 'minutes') },
      ]
    }

    if (type === 'short') {
      return [
        { value: '5s', name: unit(5, 'second', 'seconds') },
        { value: '10s', name: unit(10, 'second', 'seconds') },
        { value: '15s', name: unit(15, 'second', 'seconds') },
        { value: '30s', name: unit(30, 'second', 'seconds') },
        { value: '1m', name: unit(1, 'minute', 'minute') },
        { value: '2m', name: unit(2, 'minute', 'minutes') },
        { value: '3m', name: unit(3, 'minute', 'minutes') },
        { value: '5m', name: unit(5, 'minute', 'minutes') },
        { value: '8m', name: unit(8, 'minute', 'minutes') },
        { value: '10m', name: unit(10, 'minute', 'minutes') },
      ]
    }

    if (type === 'logqueue') {
      return [
        { value: '0s', name: $_('words.select-option.Off') },
        { value: '1m', name: unit(1, 'minute', 'minute') },
        { value: '2m', name: unit(2, 'minute', 'minutes') },
        { value: '5m', name: unit(5, 'minute', 'minutes') },
        { value: '10m', name: unit(10, 'minute', 'minutes') },
        { value: '15m', name: unit(15, 'minute', 'minutes') },
        { value: '30m', name: unit(30, 'minute', 'minutes') },
        { value: '1h', name: unit(1, 'hour', 'hour') },
        { value: '2h', name: unit(2, 'hour', 'hours') },
        { value: '6h', name: unit(6, 'hour', 'hours') },
        { value: '12h', name: unit(12, 'hour', 'hours') },
      ]
    }

    if (type === 'folderpoll') {
      return [
        { value: '0s', name: $_('words.select-option.ChecksDisabled') },
        { value: '1s', name: unit(1, 'second', 'seconds') },
        { value: '2s', name: unit(2, 'second', 'seconds') },
        { value: '5s', name: unit(5, 'second', 'seconds') },
        { value: '10s', name: unit(10, 'second', 'seconds') },
        { value: '15s', name: unit(15, 'second', 'seconds') },
        { value: '30s', name: unit(30, 'second', 'seconds') },
        { value: '1m', name: unit(1, 'minute', 'minute') },
        { value: '2m', name: unit(2, 'minute', 'minutes') },
        { value: '5m', name: unit(5, 'minute', 'minutes') },
      ]
    }

    if (type === 'nullable') {
      return [
        { value: '', name: $_('words.select-option.Unset') },
        { value: '1m', name: unit(1, 'minute', 'minute') },
        { value: '2m', name: unit(2, 'minute', 'minutes') },
        { value: '5m', name: unit(5, 'minute', 'minutes') },
        { value: '10m', name: unit(10, 'minute', 'minutes') },
        { value: '15m', name: unit(15, 'minute', 'minutes') },
        { value: '20m', name: unit(20, 'minute', 'minutes') },
        { value: '30m', name: unit(30, 'minute', 'minutes') },
        { value: '1h', name: unit(1, 'hour', 'hour') },
      ]
    }

    if (type === 'maxbytes') {
      return (
        options ?? [
          { value: '0', name: $_('words.select-option.Unlimited') },
          { value: '5GB', name: '5 GB' },
          { value: '8GB', name: '8 GB' },
          { value: '10GB', name: '10 GB' },
          { value: '15GB', name: '15 GB' },
          { value: '20GB', name: '20 GB' },
          { value: '25GB', name: '25 GB' },
          { value: '30GB', name: '30 GB' },
          { value: '35GB', name: '35 GB' },
          { value: '40GB', name: '40 GB' },
          { value: '45GB', name: '45 GB' },
          { value: '50GB', name: '50 GB' },
          { value: '75GB', name: '75 GB' },
          { value: '100GB', name: '100 GB' },
          { value: '150GB', name: '150 GB' },
          { value: '200GB', name: '200 GB' },
          { value: '500GB', name: '500 GB' },
          { value: '1TB', name: '1 TB' },
          { value: '2TB', name: '2 TB' },
          { value: '5TB', name: '5 TB' },
          { value: '10TB', name: '10 TB' },
        ]
      )
    }
    return options
  })

  function toggleTooltip(e: Event | undefined = undefined) {
    e?.preventDefault()
    showTooltip = !showTooltip
  }

  function togglePassword(e: Event | undefined = undefined) {
    e?.preventDefault()
    revealed = !revealed
  }

  /** Native <select> and <input type="number"> stringify; keep the bound type. */
  function bindValue(v: unknown) {
    if (typeof original === 'boolean' || typeof value === 'boolean') {
      if (v === true || v === 'true') {
        value = true
        return
      }
      if (v === false || v === 'false') {
        value = false
        return
      }
    }
    if (
      type === 'number' ||
      typeof original === 'number' ||
      typeof value === 'number'
    ) {
      if (v === '' || v === null || v === undefined) {
        value = 0
        return
      }
      const n = typeof v === 'number' ? v : Number(v)
      if (Number.isFinite(n)) {
        value = n
        return
      }
    }
    value = v
  }
</script>

<div class="input">
  <FormGroup>
    <Label for={id}>
      {@html labelText}
      {#if badge}
        <Badge color="secondary" style="margin-left: 0.5rem;">{badge}</Badge>
      {/if}
    </Label>

    <InputGroup>
      {#if showHelp}
        <Button
          type="button"
          class="btn-icon"
          color="secondary"
          onclick={toggleTooltip}
          outline
          aria-expanded={showTooltip}
          title={$_('phrases.ShowMore')}
        >
          {#if showTooltip}
            <Icon i={WarningCircle} c1="dimgray" d1="gainsboro" />
          {:else if hasEnv}
            <Icon i={WarningCircle} c1="red" d1="mediumvioletred" />
          {:else}
            <Icon help />
          {/if}
        </Button>
      {/if}

      {@render pre?.()}
      <Input
        {id}
        class={[inputClass, changed && 'changed', restClass]}
        type={currType as InputType}
        bind:value={() => shownValue, bindValue}
        autocomplete="off"
        placeholder={placeholderText}
        aria-invalid={!!feedback}
        aria-describedby={feedback ? `${id}-feedback` : undefined}
        disabled={isDisabled}
        {...rest}
      >
        {#if children}
          {@render children()}
        {:else if selectOptions}
          {#if shownValue !== undefined &&
            shownValue !== null &&
            !selectOptions.some((o) => o.value == shownValue)}
            <option value={shownValue}>
              {shownValue} ({$_('words.select-option.custom')})
            </option>
          {/if}
          {#each selectOptions as o (String(o.value))}
            <option value={o.value} disabled={o.disabled}>
              {o.name}
            </option>
          {/each}
        {:else if typeof shownValue === 'boolean' && type === 'select'}
          <option value={inverted}>
            {$_('words.select-option.Disabled')}
          </option>
          <option value={!inverted}>
            {$_('words.select-option.Enabled')}
          </option>
        {/if}
      </Input>

      {#if type === 'password'}
        <Button
          type="button"
          class="btn-icon"
          outline
          onclick={togglePassword}
          aria-pressed={revealed}
          title={$_(revealed ? 'phrases.HidePassword' : 'phrases.ShowPassword')}
        >
          <Icon i={passIcon} c1="royalblue" d1="orange" />
        </Button>
      {/if}
      {#if canBrowse}
        <BButton bind:isOpen={browseOpen} />
      {/if}
      {@render post?.()}
    </InputGroup>
    {#if feedback}
      <FormFeedback class="d-block" id="{id}-feedback">{feedback}</FormFeedback>
    {/if}

    {#if showTooltip}
      <div transition:slide>
        <Card body class="mt-1 help-card" color="warning" outline>
          {#if envVar}
            <ul class="mb-0">
              <li>
                <T
                  id={envIsList
                    ? 'phrases.EnvironmentVariables'
                    : 'phrases.EnvironmentVariable'}
                  variableName={env}
                />
              </li>
            </ul>
            {#if hasEnv}
              <p class="mt-2 mb-0">
                <T
                  id={envIsList
                    ? 'phrases.VariableDescriptionPlural'
                    : 'phrases.VariableDescription'}
                />
              </p>
              {#if liveText}
                <p class="mt-2 mb-0">
                  <T id="phrases.LiveValue" value={liveText} />
                </p>
              {/if}
            {/if}
          {/if}
          {#if tooltipHtml}{@html tooltipHtml}{/if}
          {#if configHintHtml}
            <p class="help-config-label">{$_('phrases.ConfigFile')}</p>
            {@html configHintHtml}
          {/if}
        </Card>
      </div>
    {/if}

    {#if descriptionHtml}<FormText>{@html descriptionHtml}</FormText>{/if}
    {@render msg?.()}
  </FormGroup>

  {#if canBrowse && browseOpen}
    <BModal
      bind:isOpen={browseOpen}
      bind:value
      title={labelText}
      dir={browseDir}
      file={browse === 'file' && !browseFile}
      disableMkdir={pickerOnly}
      {browseFile}
    />
  {/if}
</div>

<style>
  .input :global(textarea) {
    resize: vertical;
  }

  .input :global(label) {
    font-weight: 550;
  }

  .input :global(.form-control.changed),
  .input :global(.form-select.changed),
  .input :global(.btn.changed) {
    background-color: rgba(205, 92, 92, 0.322) !important;
  }

  .input :global(.input-group > .btn-icon) {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 44px;
    padding-left: 0;
    padding-right: 0;
  }

  .input :global(.input-group > .btn:not(.btn-icon)) {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding-left: 0.75rem;
    padding-right: 0.75rem;
    white-space: nowrap;
  }

  .input :global(.help-card) {
    white-space: normal;
  }

  .input :global(.help-card p) {
    overflow-wrap: break-word;
    word-break: normal;
  }

  .input :global(.help-config-label) {
    color: var(--bs-secondary-color, #6c757d);
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    margin: 0.75rem 0 0;
  }
</style>
