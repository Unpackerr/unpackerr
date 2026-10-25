import { describe, expect, it } from 'vitest'
import {
  deepCopy,
  deepEqual,
  explicitDeleteDelay,
  explicitTimeout,
  helpToHtml,
  helpToInline,
  readCookie,
  writeCookie,
} from './util'

describe('deepEqual and deepCopy', () => {
  it('compares nested values and copies without sharing', () => {
    expect(deepEqual({ a: [1, { b: 2 }] }, { a: [1, { b: 2 }] })).toBe(true)
    expect(deepEqual({ a: 1 }, { a: 1, b: 2 })).toBe(false)
    expect(deepEqual(null, {})).toBe(false)

    const src = { a: [1] }
    const copy = deepCopy(src)
    copy.a.push(2)
    expect(src.a).toEqual([1])
  })
})

describe('help html', () => {
  it('escapes text and turns backticks into code', () => {
    expect(helpToInline('use `UN_DEBUG` & friends')).toBe(
      'use <code>UN_DEBUG</code> &amp; friends',
    )
    expect(helpToInline('see <em>this</em>')).toBe('see <em>this</em>')
    expect(helpToHtml('already <em>html</em>')).toBe(
      '<p class="mt-2 mb-0">already <em>html</em></p>',
    )
    expect(helpToHtml('one\n\ntwo')).toBe(
      '<p class="mt-2 mb-0">one</p><p class="mt-2 mb-0">two</p>',
    )
  })
})

describe('cookies and inherited durations', () => {
  it('writes and reads a cookie', () => {
    writeCookie('un-test', 'a b')
    expect(readCookie('un-test')).toBe('a b')
    expect(readCookie('missing')).toBe('')
  })

  it('replaces an inheriting zero duration', () => {
    expect(explicitTimeout('0s')).toBe('10s')
    expect(explicitTimeout('30s')).toBe('30s')
    expect(explicitDeleteDelay('')).toBe('5m')
  })
})
