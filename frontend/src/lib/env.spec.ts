import { afterEach, describe, expect, it } from 'vitest'
import { env, envHas, envLiveValue } from './env.svelte'

describe('env overlays', () => {
  afterEach(() => {
    env.pairs = {}
  })

  it('hides the login secret and returns other live values', () => {
    env.pairs = {
      WEBSERVER_UI_PASSWORD: 'secret',
      SONARR_main_URL: 'http://sonarr',
    }

    expect(envLiveValue('WEBSERVER_UI_PASSWORD')).toBeUndefined()
    expect(envLiveValue('SONARR_main_URL')).toBe('http://sonarr')
    expect(envHas('SONARR_main_URL')).toBe(true)
    expect(envHas('RADARR_main_URL')).toBe(false)
  })

  it('matches indexed children of a trailing-underscore pattern', () => {
    env.pairs = { FOLDER_tv_EXCLUDE_PATH_0: '/skip' }
    expect(envHas('FOLDER_tv_EXCLUDE_PATH_')).toBe(true)
    expect(envLiveValue('FOLDER_tv_EXCLUDE_PATH_')).toBe('/skip')
  })

  it('matches Starr paths indexes and not a singular PATH name', () => {
    env.pairs = { SONARR_0_PATHS_0: '/downloads', SONARR_0_PATHS_1: '/mnt/tv' }
    expect(envHas('SONARR_0_PATHS')).toBe(true)
    expect(envLiveValue('SONARR_0_PATHS')).toBe('/downloads\n/mnt/tv')
    expect(envHas('SONARR_0_PATH')).toBe(false)
  })
})
