export type AuthMode = 'password' | 'header' | 'noauth'

export const defaultAuthHeader = 'X-Webauth-User'
export const defaultAuthRoleHeader = 'X-Webauth-Role'
export const minUIPassword = 8

/** Select options from GET /api/auth/me headers: `Name (value)`. */
export function headerPickerOptions(
  headers: Record<string, string[] | null> | null | undefined,
): { value: string; name: string }[] {
  return Object.entries(headers ?? {}).map(([key, values]) => {
    const sample = (values ?? []).filter((v) => v !== '').join(', ')
    return { value: key, name: sample ? `${key} (${sample})` : key }
  })
}

export function parseStoredPassword(raw: string): {
  mode: AuthMode
  username: string
  header: string
} {
  const value = (raw || '').trim()
  if (value === 'noauth' || value.startsWith('noauth:')) {
    const header = value.startsWith('noauth:')
      ? value.slice('noauth:'.length)
      : ''
    return {
      mode: 'noauth',
      username: 'admin',
      header: header || defaultAuthHeader,
    }
  }

  if (value === 'webauth' || value.startsWith('webauth:')) {
    const header = value.startsWith('webauth:')
      ? value.slice('webauth:'.length)
      : ''
    return {
      mode: 'header',
      username: 'admin',
      header: header || defaultAuthHeader,
    }
  }

  if (value.startsWith('!!cryptd!!')) {
    const rest = value.slice('!!cryptd!!'.length)
    const user = rest.split(':')[0] || 'admin'
    return {
      mode: 'password',
      username: user.startsWith('$2') ? 'admin' : user,
      header: defaultAuthHeader,
    }
  }

  return { mode: 'password', username: 'admin', header: defaultAuthHeader }
}

export function reservedUsername(name: string): boolean {
  const n = name.trim().toLowerCase()
  return (
    n === 'webauth' || n === 'noauth' || n === 'filepath' || name.includes(':')
  )
}
