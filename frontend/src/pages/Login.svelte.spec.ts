import { fireEvent, render, screen } from '@testing-library/svelte'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const auth = vi.hoisted(() => ({
  login: vi.fn(async (_username: string, _password: string) => ''),
  profile: { loginDisabled: false },
}))

vi.mock('../lib/auth.svelte', () => ({
  login: auth.login,
  profile: auth.profile,
}))

import Login from './Login.svelte'

describe('Login', () => {
  beforeEach(() => {
    auth.login.mockReset()
    auth.profile.loginDisabled = false
  })

  it('asks for a password and does not call login when it is empty', async () => {
    render(Login)

    await fireEvent.click(screen.getByRole('button', { name: 'Log in' }))

    expect(screen.getByRole('alert').textContent).toContain('Enter your password.')
    expect(auth.login).not.toHaveBeenCalled()
  })

  it('posts the username and password', async () => {
    auth.login.mockResolvedValue('nope')
    render(Login)

    await fireEvent.input(screen.getByLabelText('Username'), {
      target: { value: 'david' },
    })
    await fireEvent.input(screen.getByLabelText('Password'), {
      target: { value: 'secret' },
    })
    await fireEvent.click(screen.getByRole('button', { name: 'Log in' }))

    expect(auth.login).toHaveBeenCalledWith('david', 'secret')
    expect(screen.getByRole('alert').textContent).toContain('nope')
  })

  it('explains when password login is turned off', () => {
    auth.profile.loginDisabled = true
    render(Login)

    expect(screen.getByRole('status').textContent).toContain('Logins Disabled')
    expect(screen.queryByLabelText('Password')).toBeNull()
  })
})
