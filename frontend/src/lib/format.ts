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

const STATUS_PHRASE: Record<string, string> = {
  waiting: 'status.Waiting',
  queued: 'status.Queued',
  extracting: 'status.Extracting',
  extractfailed: 'status.ExtractFailed',
  'extract failed': 'status.ExtractFailed',
  extracted: 'status.Extracted',
  imported: 'status.Imported',
  deleting: 'status.Deleting',
  deletefailed: 'status.DeleteFailed',
  'delete failed': 'status.DeleteFailed',
  deleted: 'status.Deleted',
  extractednothing: 'status.ExtractedNothing',
  'extracted nothing': 'status.ExtractedNothing',
}

// Map API status ids such as extractednothing to status.* locale keys.
export function statusPhrase(status: string | undefined): string {
  if (!status) return 'phrases.Empty'
  return STATUS_PHRASE[status.toLowerCase()] ?? status
}

// Exact English Error()/note strings from the Go pipeline and xtractr.
// Add a matching errors.* locale key when a new sentinel shows up in the UI.
const ERROR_PHRASE: Record<string, string> = {
  'no compressed files found': 'errors.NoCompressedFiles',
  'no extractable files': 'errors.NoExtractableFiles',
  'waiting for syncthing': 'errors.WaitingSyncthing',
  'interrupted by restart': 'errors.InterruptedRestart',
  'unknown archive file type': 'errors.UnknownArchiveType',
  'archived file checksum mismatch': 'errors.Checksum',
  'extracted size exceeds maximum bytes': 'errors.MaxBytes',
  'extracted file count exceeds maximum': 'errors.MaxFiles',
  'extracted size exceeds maximum compression ratio': 'errors.MaxRatio',
  'nested archive count exceeds maximum': 'errors.MaxNested',
  'refusing to extract a symbolic link as an archive': 'errors.ArchiveSymlink',
  'archived file contains invalid path': 'errors.InvalidPath',
  'archived file contains invalid header file': 'errors.InvalidHead',
}

const ERROR_PHRASE_LIST = Object.entries(ERROR_PHRASE).sort(
  (a, b) => b[0].length - a[0].length,
)

// Map a backend error or note to an errors.* locale key. Empty means show the raw string.
export function errorPhrase(msg: string | undefined): string {
  if (!msg) return ''

  const lower = msg.trim().toLowerCase()
  const exact = ERROR_PHRASE[lower]
  if (exact) return exact

  for (const [en, key] of ERROR_PHRASE_LIST) {
    // xtractr wraps sentinels: "file.rar: archived file checksum mismatch (got ..., want ...)"
    if (lower.includes(en)) return key
  }

  return ''
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

export function isFinishedHistory(
  row: { status?: string } | undefined,
): boolean {
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

// Compact remaining time until iso; empty when due/eta is missing or already past.
export function remainCompact(
  iso: string | undefined,
  now = Date.now(),
): string {
  if (!iso || isZeroTime(iso)) return ''
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return ''

  const sec = Math.ceil((t - now) / 1000)
  if (sec <= 0) return ''
  if (sec < 60) return `${sec}s`

  const min = Math.floor(sec / 60)
  if (min < 60) {
    const rem = sec % 60
    return rem ? `${min}m ${rem}s` : `${min}m`
  }

  const hr = Math.floor(min / 60)
  if (hr < 24) {
    const rem = min % 60
    return rem ? `${hr}h ${rem}m` : `${hr}h`
  }

  return `${Math.floor(hr / 24)}d`
}
