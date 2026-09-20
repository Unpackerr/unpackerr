<script lang="ts">
  import { Button, FormGroup, FormText, Label } from '@sveltestrap/sveltestrap'
  import X from 'phosphor-svelte/lib/X'
  import Input from './Input.svelte'
  import Icon from './Icon.svelte'
  import { _ } from '../lib/i18n/Translate.svelte'
  import {
    mapFromPairs,
    pairsFromMap,
    validSlug,
    type MapPair,
  } from '../lib/slug'
  import { envHas } from '../lib/env.svelte'

  let {
    values = $bindable({} as Record<string, string>),
    invalid = $bindable(false),
    disabled = false,
    idPrefix,
    helpKey = '',
    envVar = '',
    label = '',
    description = '',
  }: {
    values: Record<string, string>
    invalid?: boolean
    disabled?: boolean
    idPrefix: string
    helpKey?: string
    envVar?: string
    label?: string
    description?: string
  } = $props()

  let pairs = $state<MapPair[]>(pairsFromMap(values))
  const locked = $derived(disabled || envHas(envVar))

  function flush() {
    values = mapFromPairs(pairs)
  }

  function add() {
    pairs = [...pairs, { key: '', value: '' }]
  }

  function remove(i: number) {
    pairs = pairs.filter((_, idx) => idx !== i)
    flush()
  }

  function setKey(i: number, key: string) {
    pairs[i].key = key
    flush()
  }

  function setValue(i: number, value: string) {
    pairs[i].value = value
    flush()
  }

  function keyError(i: number, raw: unknown): string {
    const key = String(raw ?? '').trim()
    if (!key) return ''

    if (!validSlug(key)) return $_('phrases.SlugInvalid')
    const dup = pairs.some(
      (p, idx) => idx !== i && p.key.trim().toLowerCase() === key.toLowerCase(),
    )
    if (dup) return $_('phrases.SlugDuplicate')

    return ''
  }

  const keysInvalid = $derived(pairs.some((_, i) => keyError(i, pairs[i].key) !== ''))

  $effect(() => {
    invalid = keysInvalid
  })
</script>

<FormGroup>
  {#if label}
    <Label>{label}</Label>
  {/if}
  {#if description}
    <FormText class="d-block mb-2">{description}</FormText>
  {/if}
  {#each pairs as pair, i (idPrefix + '-' + i)}
    <div class="map-pairs-row">
      <div class="map-pairs-key">
        <Input
          compact
          id="{idPrefix}-key-{i}"
          helpKey={i === 0 ? helpKey : ''}
          label={i === 0 ? $_('phrases.MapKey') : ''}
          aria-label={i === 0 ? undefined : $_('phrases.MapKey')}
          bind:value={() => pair.key, (v) => setKey(i, String(v))}
          disabled={locked}
          envVar={i === 0 ? envVar : ''}
          validate={(_id, v) => keyError(i, v)}
        />
      </div>
      <div class="map-pairs-val">
        <Input
          compact
          id="{idPrefix}-val-{i}"
          label={i === 0 ? $_('phrases.MapValue') : ''}
          aria-label={i === 0 ? undefined : $_('phrases.MapValue')}
          bind:value={() => pair.value, (v) => setValue(i, String(v))}
          disabled={locked}
        >
          {#snippet post()}
            {#if !locked}
              <Button
                type="button"
                class="btn-icon"
                outline
                color="danger"
                title={$_('buttons.Remove')}
                aria-label={$_('buttons.Remove')}
                on:click={() => remove(i)}
              >
                <Icon i={X} c1="crimson" d1="#ff6b6b" />
              </Button>
            {/if}
          {/snippet}
        </Input>
      </div>
    </div>
  {/each}
  {#if !locked}
    <Button type="button" size="sm" outline color="success" on:click={add}>
      {$_('buttons.AddPair')}
    </Button>
  {/if}
</FormGroup>

<style>
  .map-pairs-row {
    display: flex;
    flex-wrap: nowrap;
    gap: 0.35rem;
    align-items: flex-start;
    margin-bottom: 0.35rem;
  }

  .map-pairs-key {
    flex: 0 0 9rem;
    width: 9rem;
  }

  .map-pairs-val {
    flex: 1 1 auto;
    min-width: 0;
  }

  @media (min-width: 992px) {
    .map-pairs-key {
      flex: 0 0 13rem;
      width: 13rem;
    }
  }
</style>
