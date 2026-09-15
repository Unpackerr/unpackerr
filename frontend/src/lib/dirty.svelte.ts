import { onDestroy } from 'svelte'

export const dirty = $state({
  changed: false,
  pending: null as string | null,
})

type Part = {
  changed: boolean
  /** True when this form stays mounted at next, so its edits are not discarded. */
  retain?: (path: string) => boolean
}

const parts = new Map<symbol, Part>()

function sync() {
  dirty.changed = [...parts.values()].some((p) => p.changed)
}

/** True when some dirty form would unmount if we navigated to next. */
export function wouldLoseChanges(next: string): boolean {
  return [...parts.values()].some((p) => p.changed && !p.retain?.(next))
}

/** Sync a settings form's dirty flag into the router guard. Call during component init.
 *  Several forms can be mounted at once (Starr poll interval + app list); leaving one
 *  must not clear the others. Pass retain when the form stays up on some routes
 *  (poll interval across Starr app tabs) so those clicks do not prompt. */
export function trackDirty(
  getChanged: () => boolean,
  retain?: (path: string) => boolean,
) {
  const id = Symbol('dirty')
  $effect(() => {
    parts.set(id, { changed: getChanged(), retain })
    sync()
  })

  onDestroy(() => {
    parts.delete(id)
    if (dirty.pending == null) sync()
  })
}
