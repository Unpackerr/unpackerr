<script lang="ts">
  import { Tooltip } from '@sveltestrap/sveltestrap'
  import type { Snippet } from 'svelte'
  import { theme } from '../lib/theme.svelte'

  // Button (not a tabindex span) so tap/focus opens the tooltip on phones
  // without tripping a11y_no_noninteractive_tabindex.
  let {
    id,
    hint,
    placement = 'top',
    label = '',
    children,
  }: {
    id: string
    hint: string
    placement?: 'top' | 'bottom' | 'left' | 'right'
    label?: string
    children: Snippet
  } = $props()
</script>

<button type="button" {id} class="hint-target" aria-label={label || undefined}>
  {@render children()}
</button>
<Tooltip target={id} {placement} theme={theme.tooltip}>{hint}</Tooltip>
