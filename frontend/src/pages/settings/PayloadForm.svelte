<script lang="ts">
  import { onMount } from 'svelte'
  import {
    Button,
    Col,
    FormGroup,
    FormText,
    Label,
    Row,
    Spinner,
  } from '@sveltestrap/sveltestrap'
  import Input from '../../components/Input.svelte'
  import MapPairs from '../../components/MapPairs.svelte'
  import SaveBar from '../../components/SaveBar.svelte'
  import { _ } from '../../lib/i18n/Translate.svelte'
  import { loadSection, saveSection } from '../../lib/config'
  import { has } from '../../lib/auth.svelte'
  import { configPerm } from '../../lib/perms'
  import { deepCopy, deepEqual } from '../../lib/util'
  import { trackDirty } from '../../lib/dirty.svelte'
  import { failure } from '../../lib/toast'
  import { envHas } from '../../lib/env.svelte'
  import { mapKeysInvalid } from '../../lib/slug'
  import {
    EXTRACT_EVENT_TITLES,
    EXTRACT_STATUSES,
    type HooksConfig,
  } from '../../lib/types'

  const section = 'hooks' as const
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

  let cfg = $state<HooksConfig | null>(null)
  let orig = $state<HooksConfig | null>(null)
  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')

  const canWrite = has(configPerm(section, 'write'))
  const titlesLocked = $derived(envHas('HOOKS_TITLES_*'))
  const invalid = $derived(mapKeysInvalid(cfg?.customIDs))

  function normalize(
    raw: Partial<HooksConfig> | null | undefined,
  ): HooksConfig {
    return {
      customIDs: { ...(raw?.customIDs ?? {}) },
      titles: { ...(raw?.titles ?? {}) },
    }
  }

  function savePayload(): HooksConfig {
    const titles: Record<string, string> = {}
    for (const [key, value] of Object.entries(cfg?.titles ?? {})) {
      const t = value.trim()
      if (t) titles[key] = t
    }

    return { customIDs: { ...(cfg?.customIDs ?? {}) }, titles }
  }

  onMount(async () => {
    try {
      const { data, error: err } = await loadSection<HooksConfig>(section)
      if (err) error = err
      else {
        const next = normalize(data)
        cfg = next
        orig = deepCopy(next)
      }
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
  })

  trackDirty(() => !loading && !!cfg && !!orig && !deepEqual(cfg, orig))

  async function save() {
    if (!cfg || invalid) {
      if (invalid) failure($_('phrases.FixInvalidFields'))
      return
    }

    saving = true
    const payload = savePayload()
    const ok = await saveSection(section, payload)

    if (ok) {
      cfg = normalize(payload)
      orig = deepCopy(cfg)
    } else {
      failure($_('phrases.FixInvalidFields'))
    }

    saving = false
  }
</script>

{#if loading}
  <Spinner color="primary" />
{:else if error}
  <p class="text-danger">{error}</p>
{:else if cfg}
  {@const data = cfg}
  <Row class="g-3">
    <Col md="12">
      <MapPairs
        bind:values={data.customIDs}
        disabled={!canWrite}
        idPrefix="hooks-ids"
        helpKey="config.payload.customIds"
        envVar="HOOKS_CUSTOM_IDS_*"
        label={$_('config.payload.customIDs.label')}
        description={$_('config.payload.customIDs.description')}
      />
    </Col>
    <Col md="12">
      <FormGroup>
        <Label>{$_('config.payload.titles.label')}</Label>
        <FormText class="d-block mb-2"
          >{$_('config.payload.titles.description')}</FormText
        >
        <Row class="g-1">
          {#each EXTRACT_STATUSES as st, si (st.id)}
            <Col md="6">
              <Input
                compact
                id={`hooks-title-${st.id}`}
                helpKey={si === 0 ? 'config.payload.titles' : ''}
                label={$_('status.' + (statusKeys[si] ?? st.label))}
                placeholder={EXTRACT_EVENT_TITLES[st.id] ?? st.label}
                bind:value={
                  () => data.titles[st.id] ?? '',
                  (v) => {
                    data.titles[st.id] = String(v)
                  }
                }
                original={orig?.titles[st.id] ?? ''}
                disabled={!canWrite || titlesLocked}
                envVar={`HOOKS_TITLES_${st.id}`}
              />
            </Col>
          {/each}
        </Row>
      </FormGroup>
    </Col>
  </Row>

  {#if canWrite}
    <SaveBar>
      <Button
        color="primary"
        disabled={saving || (canWrite && invalid)}
        on:click={save}
      >
        {#if saving}<Spinner size="sm" />{/if}
        <span class="ms-1">{$_('buttons.Save')}</span>
      </Button>
    </SaveBar>
  {/if}
{/if}
