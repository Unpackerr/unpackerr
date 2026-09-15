import { get } from 'svelte/store'
import { api } from '../../lib/api'
import type { BrowseDir } from '../../lib/types'
import { success } from '../../lib/toast'
import { _ } from '../../lib/i18n/Translate.svelte'

function rtrim(s: string, c: string): string {
  while (s.endsWith(c)) s = s.slice(0, -c.length)
  return s
}

function baseName(path: string): string {
  const trimmed = path.replace(/[/\\]+$/, '')
  const i = Math.max(trimmed.lastIndexOf('/'), trimmed.lastIndexOf('\\'))
  return i >= 0 ? trimmed.slice(i + 1) : trimmed
}

function fileNameFor(current: string, fallback: string): string {
  const t = current.trim()
  if (!t || t.endsWith('/') || t.endsWith('\\')) return fallback
  const base = baseName(t)
  if (!base || base === '.' || base === '..') return fallback
  return base
}

function joinFile(dir: string, name: string, sep: string): string {
  const trimmed = rtrim(dir, sep)
  if (!trimmed) return name
  if (baseName(trimmed) === name) return trimmed
  return trimmed + sep + name
}

function errorMessage(body: unknown): string {
  if (
    body &&
    typeof body === 'object' &&
    'error' in body &&
    typeof (body as { error: unknown }).error === 'string'
  ) {
    return (body as { error: string }).error
  }
  return String(body ?? '')
}

export class FileBrowser {
  public wd: BrowseDir
  public respErr: string
  public loading: boolean
  public input: string
  public readonly fileName: string
  private value: string
  private selected: string
  private readonly defaultFile: string
  private readonly close: (value: string) => void

  constructor(value: string, close: (value: string) => void, defaultFile = '') {
    this.value = value || ''
    this.defaultFile = defaultFile
    this.fileName = fileNameFor(value || '', defaultFile)
    this.wd = $state({
      path: this.value,
      files: [],
      dirs: [],
      sep: '/',
      mom: '',
      error: '',
    })
    this.selected = $state(value || '~')
    this.respErr = $state('')
    this.loading = $state(false)
    this.input = $derived(this.wd.path)
    this.close = close
    void this.getFiles()
  }

  public readonly preview = (dirPath: string): string => {
    if (!this.defaultFile) return dirPath
    return joinFile(dirPath, this.fileName, this.wd.sep || '/')
  }

  public readonly cd = async (e: Event, to: string, direct = false) => {
    e.preventDefault()
    // Windows volume root: do not produce \C:
    if (!this.wd.path && this.wd.sep === '\\') direct = true
    this.selected = direct
      ? to
      : rtrim(this.wd.path, this.wd.sep) + this.wd.sep + to
    this.respErr = ''
    await this.getFiles()
  }

  public readonly select = (e: Event, file: string, dir = false) => {
    e.preventDefault()
    const picked =
      (dir ? '' : rtrim(this.wd.path, this.wd.sep) + this.wd.sep) + file
    this.value = this.defaultFile && dir ? this.preview(picked) : picked
    this.close(this.value)
  }

  public readonly mkdir = async (name: string): Promise<boolean> => {
    if (name.includes('/') || name.includes('\\')) {
      this.respErr = get(_)('FileBrowser.InvalidPath')
      return false
    }

    this.loading = true
    const path = rtrim(this.wd.path, this.wd.sep) + this.wd.sep + name
    const resp = await api.post<BrowseDir>('browse', { path })
    this.loading = false
    if (!resp.ok) {
      this.respErr = errorMessage(resp.body)
      return false
    }

    success(get(_)('FileBrowser.Created', { values: { path } }))
    this.wd = resp.body
    this.respErr = this.wd.error || ''
    return true
  }

  private readonly getFiles = async () => {
    this.loading = true
    const resp = await api.get<BrowseDir>(
      'browse?dir=' + encodeURIComponent(this.selected),
    )
    this.loading = false

    if (resp.ok) {
      this.wd = resp.body
      this.respErr = this.wd.error || ''
      return
    }

    this.wd = {
      ...this.wd,
      mom: this.wd.path,
      path: this.selected,
      dirs: [],
      files: [],
    }
    this.respErr = errorMessage(resp.body)
  }
}
