<script lang="ts">
  import type { Component } from 'svelte'
  import Question from 'phosphor-svelte/lib/Question'
  import { theme } from '../lib/theme.svelte'

  let {
    i,
    help = false,
    c1 = 'dimgray',
    d1 = 'gainsboro',
  }: {
    i?: Component<any>
    help?: boolean
    c1?: string
    d1?: string
  } = $props()

  const Comp = $derived(help ? Question : i)
  const color = $derived(theme.resolved === 'dark' ? d1 : c1)
</script>

{#if Comp}
  <Comp size="1.25em" {color} weight="bold" />
{/if}
