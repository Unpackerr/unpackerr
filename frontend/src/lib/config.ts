import { get } from 'svelte/store'
import { _ } from 'svelte-i18n'
import { api, type BackendResponse } from './api'
import { markRestart } from './restart.svelte'
import { success, failure } from './toast'
import type { ConfigSection, ConfigWriteReply } from './types'

function t(key: string, fallback: string): string {
  try {
    const text = get(_)(key)
    if (text && text !== key) return text
  } catch {
    // i18n may not be ready
  }

  return fallback
}

/** loadSection GETs the on-disk (file-shaped) config for a section. */
export async function loadSection<T>(
  section: ConfigSection,
): Promise<{ data?: T; error?: string }> {
  const res = await api.get<T>('config/' + section)
  if (!res.ok)
    return {
      error:
        (res.body as { error?: string })?.error ?? 'failed to load ' + section,
    }

  return { data: res.body }
}

/** Running config after UN_* overlays. List forms use this only to list env-only slugs. */
export async function loadSectionLive<T>(
  section: ConfigSection,
): Promise<{ data?: T; error?: string }> {
  const res = await api.get<T>('config/' + section + '/live')
  if (!res.ok)
    return {
      error:
        (res.body as { error?: string })?.error ?? 'failed to load ' + section,
    }

  return { data: res.body }
}

/** saveSection PUTs the file-shaped payload back and surfaces restartRequired. */
export async function saveSection(
  section: ConfigSection,
  body: unknown,
  quiet = false,
): Promise<boolean> {
  const res = await api.put<ConfigWriteReply>('config/' + section, body)
  if (!res.ok) {
    failure((res.body as { error?: string })?.error ?? 'save failed')
    return false
  }

  markRestart(res.body?.restartRequired ?? false)

  if (!quiet) {
    success(
      res.body?.restartRequired
        ? t(
            'phrases.SavedRestart',
            'Saved. A restart is needed to fully apply.',
          )
        : t('phrases.Saved', 'Saved.'),
    )
  }

  return true
}

const testTimeoutMs = 45000

/** Probe a Starr instance or fire one sample hook. Requires config:{section}:write. */
export async function testSection<T>(
  section: ConfigSection,
  body: unknown,
): Promise<BackendResponse<T>> {
  return api.post<T>('config/' + section + '/test', body, testTimeoutMs)
}
