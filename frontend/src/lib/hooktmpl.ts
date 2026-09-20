export const HOOK_TEMPLATE_NAMES = [
  'notifiarr',
  'discord',
  'gotify',
  'pushover',
  'slack',
  'telegram',
] as const

export type HookTemplateName = (typeof HOOK_TEMPLATE_NAMES)[number]

export function hookTemplateFileName(name: string): string {
  return `unpackerr-webhook-${name}.tmpl`
}

export function defaultHookTemplate(
  name: string | undefined,
): HookTemplateName {
  const n = (name ?? '').trim().toLowerCase()
  if (n === 'default') return 'notifiarr'
  if ((HOOK_TEMPLATE_NAMES as readonly string[]).includes(n)) {
    return n as HookTemplateName
  }
  return 'notifiarr'
}
