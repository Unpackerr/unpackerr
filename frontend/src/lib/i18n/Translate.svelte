<!-- Smoother syntax for HTML translations with interpolation. -->

<script module lang="ts">
  import { derived, get } from 'svelte/store'
  import { _, json, time as t, date as d, isLoading, locale } from 'svelte-i18n'
  export { _, json }
  export const isReady = derived(
    locale,
    ($locale) => typeof $locale === 'string',
  )

  export function datetime(datetime: string | Date | number): string {
    return `${date(datetime)} ${time(datetime)}`
  }

  export function date(date: string | Date | number): string {
    if (typeof date !== 'string') return get(d)(date)
    return get(d)(Date.parse(date))
  }

  export function time(time: string | Date | number): string {
    if (typeof time !== 'string') return get(t)(time)
    return get(t)(Date.parse(time), { format: 'long' })
  }
</script>

<script lang="ts">
  interface Props {
    id: string
    [key: string]: string | number | boolean | undefined
  }

  let { ...props }: Props = $props()
</script>

{#if $isReady === true && $isLoading === false}
  {@html $_(props.id, { values: { ...props } })}
{/if}
