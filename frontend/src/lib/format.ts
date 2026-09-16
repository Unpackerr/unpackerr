export function bytes(n: number | undefined): string {
  if (!n || n < 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']
  let i = 0
  let v = n

  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }

  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

export function relTime(iso: string | undefined, now = Date.now()): string {
  if (!iso) return '—'
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return '—'

  // Fresh server times can beat the 1s UI clock; don't flash "from now".
  const diff = Math.max(0, now - t)
  const sec = Math.round(diff / 1000)
  const min = Math.round(sec / 60)
  const hr = Math.round(min / 60)
  const day = Math.round(hr / 24)

  if (sec < 60) return `${sec}s ago`
  if (min < 60) return `${min}m ago`
  if (hr < 24) return `${hr}h ago`

  return `${day}d ago`
}

export function dateTime(iso: string | undefined): string {
  if (!iso) return '—'
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return '—'
  return new Date(t).toLocaleString()
}

// A zero time from Go marshals to 0001-01-01T00:00:00Z; treat it as empty.
export function isZeroTime(iso: string | undefined): boolean {
  return !iso || iso.startsWith('0001-01-01')
}

const STATUS_COLORS: Record<string, string> = {
  waiting: 'secondary',
  queued: 'info',
  extracting: 'primary',
  extractfailed: 'danger',
  'extract failed': 'danger',
  extracted: 'success',
  imported: 'success',
  deleting: 'warning',
  deletefailed: 'danger',
  'delete failed': 'danger',
  deleted: 'dark',
  extractednothing: 'secondary',
  'extracted nothing': 'secondary',
}

export function statusColor(status: string | undefined): string {
  if (!status) return 'secondary'
  return STATUS_COLORS[status.toLowerCase()] ?? 'secondary'
}

const FINISHED_HISTORY = new Set([
  'extractfailed',
  'extract failed',
  'extractednothing',
  'extracted nothing',
  'imported',
  'deleted',
  'deletefailed',
  'delete failed',
])

export function isFinishedHistory(row: { status?: string } | undefined): boolean {
  return !!row?.status && FINISHED_HISTORY.has(row.status.toLowerCase())
}

export function progressCaption(item: {
  percent?: number
  wrote?: number
  total?: number
  read?: number
  compressed?: number
  progress?: string
}): string {
  const max = item.total || item.compressed || 0
  const have = item.total ? (item.wrote ?? 0) : (item.read ?? 0)
  if (!max && !item.percent) return item.progress || ''
  return `${bytes(have)} / ${bytes(max)} (${Math.round(item.percent || 0)}%)`
}
