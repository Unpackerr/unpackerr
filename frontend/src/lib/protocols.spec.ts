import { describe, expect, it } from 'vitest'
import {
  PROTOCOL_BOTH,
  PROTOCOL_TORRENT,
  PROTOCOL_USENET,
  canonicalizeProtocols,
  encodeProtocols,
  parseProtocols,
} from './protocols'

describe('protocols', () => {
  it('collapses stored tokens into the three UI choices', () => {
    expect(parseProtocols(undefined)).toBe('torrent')
    expect(parseProtocols('usenet')).toBe('usenet')
    expect(parseProtocols('TorrentDownloadProtocol,UsenetDownloadProtocol')).toBe(
      'both',
    )
  })

  it('round-trips a choice through the canonical stored string', () => {
    expect(encodeProtocols('torrent')).toBe(PROTOCOL_TORRENT)
    expect(encodeProtocols('usenet')).toBe(PROTOCOL_USENET)
    expect(encodeProtocols('both')).toBe(PROTOCOL_BOTH)
    expect(canonicalizeProtocols('torrent,usenet')).toBe(PROTOCOL_BOTH)
  })
})
