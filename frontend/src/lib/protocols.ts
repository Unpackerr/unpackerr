export type ProtocolChoice = 'torrent' | 'usenet' | 'both'

export const PROTOCOL_TORRENT = 'torrent,TorrentDownloadProtocol'
export const PROTOCOL_USENET = 'usenet,UsenetDownloadProtocol'
export const PROTOCOL_BOTH = PROTOCOL_TORRENT + ',' + PROTOCOL_USENET

function tokens(raw: string): Set<string> {
  return new Set(
    (raw || '')
      .split(',')
      .map((s) => s.trim().toLowerCase())
      .filter(Boolean),
  )
}

/** Collapse a stored protocols string into the 3-way UI choice. */
export function parseProtocols(raw: string | undefined): ProtocolChoice {
  const parts = tokens(raw ?? '')
  const torrent = parts.has('torrent') || parts.has('torrentdownloadprotocol')
  const usenet = parts.has('usenet') || parts.has('usenetdownloadprotocol')

  if (torrent && usenet) return 'both'
  if (usenet) return 'usenet'
  return 'torrent'
}

export function encodeProtocols(choice: ProtocolChoice): string {
  if (choice === 'usenet') return PROTOCOL_USENET
  if (choice === 'both') return PROTOCOL_BOTH
  return PROTOCOL_TORRENT
}

/** Canonical stored string for a value that came off disk (or a new app). */
export function canonicalizeProtocols(raw: string | undefined): string {
  return encodeProtocols(parseProtocols(raw))
}
