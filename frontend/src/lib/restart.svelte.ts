// Shared flag set when a config PUT returns restartRequired. The daemon re-execs
// itself once the queue is idle; the banner just tells the operator to expect it.
// A restart invalidates the session cookie, so the next poll 401s back to login.
export const restart = $state({ pending: false })

export function markRestart(required: boolean) {
  if (required) restart.pending = true
}

export function clearRestart() {
  restart.pending = false
}
