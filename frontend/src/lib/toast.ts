import { get } from 'svelte/store'
import { _ } from 'svelte-i18n'
import { toast } from 'svelte-sonner'
import { navigate } from './router.svelte'

export function success(msg: string) {
  toast.success(msg, { duration: 4000 })
}

export function failure(msg: string) {
  toast.error(msg, { duration: 8000 })
}

export function appError(msg: string) {
  toast.error(msg, {
    id: 'app-error:' + msg,
    duration: 8000,
    action: {
      label: get(_)('pages.logs.ViewRecent'),
      onClick: () => navigate('/logs/errors'),
    },
  })
}

export function info(msg: string) {
  toast.info(msg, { duration: 4000 })
}
