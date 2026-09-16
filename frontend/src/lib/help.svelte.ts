import { api } from './api'

export type FieldHelp = {
  short?: string
  desc?: string
  env?: string
}

export const help = $state({
  fields: {} as Record<string, FieldHelp>,
})

export async function loadHelp() {
  const res = await api.get<Record<string, FieldHelp>>('config/help')
  if (res.ok && res.body) help.fields = res.body
}

export function fieldHelp(key: string | undefined): FieldHelp | undefined {
  if (!key) return undefined
  return help.fields[key]
}
