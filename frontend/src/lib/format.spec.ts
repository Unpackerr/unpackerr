import { describe, expect, it } from 'vitest'
import {
  bytes,
  errorPhrase,
  isFinishedHistory,
  isZeroTime,
  progressCaption,
  relTime,
  remainCompact,
  statusColor,
  statusPhrase,
} from './format'

const now = Date.parse('2026-10-25T12:00:00Z')

describe('bytes', () => {
  it('formats binary units', () => {
    expect(bytes(0)).toBe('0 B')
    expect(bytes(512)).toBe('512 B')
    expect(bytes(1536)).toBe('1.5 KiB')
  })
})

describe('times', () => {
  it('treats Go zero times as empty', () => {
    expect(isZeroTime(undefined)).toBe(true)
    expect(isZeroTime('0001-01-01T00:00:00Z')).toBe(true)
    expect(isZeroTime('2026-10-25T12:00:00Z')).toBe(false)
  })

  it('renders relative time from a fixed clock', () => {
    expect(relTime(undefined)).toBe('—')
    expect(relTime('not-a-time')).toBe('—')
    expect(relTime('2026-10-25T11:59:30Z', now)).toBe('30s ago')
    expect(relTime('2026-10-25T11:00:00Z', now)).toBe('1h ago')
    expect(relTime('2026-10-25T12:00:05Z', now)).toBe('0s ago')
  })

  it('compacts a future remaining duration', () => {
    expect(remainCompact('0001-01-01T00:00:00Z', now)).toBe('')
    expect(remainCompact('2026-10-25T11:00:00Z', now)).toBe('')
    expect(remainCompact('2026-10-25T12:01:30Z', now)).toBe('1m 30s')
    expect(remainCompact('2026-10-26T12:00:00Z', now)).toBe('1d')
  })
})

describe('status and errors', () => {
  it('maps pipeline statuses onto bootstrap colors and locale keys', () => {
    expect(statusColor('Extract Failed')).toBe('danger')
    expect(statusColor('nope')).toBe('secondary')
    expect(statusPhrase('extractednothing')).toBe('status.ExtractedNothing')
    expect(statusPhrase('custom')).toBe('custom')
  })

  it('finds a wrapped xtractr sentinel inside a longer error', () => {
    expect(errorPhrase('')).toBe('')
    expect(
      errorPhrase('file.rar: archived file checksum mismatch (got 1, want 2)'),
    ).toBe('errors.Checksum')
    expect(errorPhrase('something else')).toBe('')
  })

  it('knows which history rows are finished', () => {
    expect(isFinishedHistory({ status: 'Imported' })).toBe(true)
    expect(isFinishedHistory({ status: 'extracting' })).toBe(false)
    expect(isFinishedHistory(undefined)).toBe(false)
  })

  it('captions extract progress from bytes and percent', () => {
    expect(progressCaption({ progress: 'waiting' })).toBe('waiting')
    expect(
      progressCaption({ wrote: 512, total: 1024, percent: 50 }),
    ).toBe('512 B / 1.0 KiB (50%)')
  })
})
