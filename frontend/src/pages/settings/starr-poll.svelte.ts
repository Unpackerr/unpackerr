import { has } from '../../lib/auth.svelte'
import { configPerm } from '../../lib/perms'
import { saveSection } from '../../lib/config'
import { STARR_SECTIONS, type GeneralConfig } from '../../lib/types'
import { deepCopy } from '../../lib/util'

/** Shared Starr poll / queue-log state so the controls can sit above the app
 *  tabs while Save on the instance form still persists them. */
export const starrPoll = $state({
  general: null as GeneralConfig | null,
  orig: null as GeneralConfig | null,
})

export function starrPollDirty(): boolean {
  const { general, orig } = starrPoll
  return (
    general != null &&
    orig != null &&
    (general.interval !== orig.interval ||
      general.logQueues !== orig.logQueues ||
      general.activity !== orig.activity)
  )
}

/** Poll controls sit above the Starr app tabs and stay mounted between them. */
export function starrPollRetains(path: string): boolean {
  return STARR_SECTIONS.some((id) => path === '/settings/' + id)
}

export async function saveStarrPoll(quiet = false): Promise<boolean> {
  if (
    !starrPollDirty() ||
    !starrPoll.general ||
    !has(configPerm('general', 'write'))
  )
    return true
  const ok = await saveSection('general', starrPoll.general, quiet)
  if (ok) starrPoll.orig = deepCopy(starrPoll.general)
  return ok
}

export function clearStarrPoll() {
  starrPoll.general = null
  starrPoll.orig = null
}
