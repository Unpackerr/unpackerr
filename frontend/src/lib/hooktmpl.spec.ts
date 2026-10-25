import { describe, expect, it } from 'vitest'
import {
  applyNotifiarrURL,
  detectHookTemplate,
  hookFormProfile,
  hookShowsHeaders,
  hookTemplateFileName,
} from './hooktmpl'

const apiKey = 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee'

describe('hook templates', () => {
  it('sniffs a webhook URL when no template name is set', () => {
    expect(detectHookTemplate('', 'https://discord.com/api/webhooks/1', '')).toBe(
      'discord',
    )
    expect(detectHookTemplate('default', '', '')).toBe('notifiarr')
    expect(detectHookTemplate('', '', '/opt/custom.tmpl')).toBe('custom')
    expect(hookTemplateFileName('slack')).toBe('unpackerr-webhook-slack.tmpl')
  })

  it('shows the fields for the detected template', () => {
    const profile = hookFormProfile(
      '',
      'https://hooks.slack.com/services/T/B/x',
      '',
    )
    expect(profile).toMatchObject({
      template: 'slack',
      source: 'url',
      canUpdate: false,
    })
    expect(profile.fields).toEqual(['nickname', 'channel'])
    expect(hookShowsHeaders(profile, {}, profile.template)).toBe(false)
  })

  it('lifts a Notifiarr path API key into the header', () => {
    const raw = `https://notifiarr.com/api/v1/notification/unpackerr/${apiKey}`
    const canonical = 'https://notifiarr.com/api/v1/notification/unpackerr'
    const applied = applyNotifiarrURL(raw, { 'X-Api-Key': 'keep-me' })
    expect(applied.url).toBe(canonical)
    expect(applied.headers['X-Api-Key']).toBe('keep-me')

    const moved = applyNotifiarrURL(raw)
    expect(moved.url).toBe(canonical)
    expect(moved.headers['X-Api-Key']).toBe(apiKey)
  })
})
