# Review guidance for Unpackerr

Longer runtime/API context: [`INTERNALS.md`](../INTERNALS.md).

Unpackerr is a single-process daemon. One goroutine in `Run()` (`pkg/unpackerr/start.go`)
owns the live `Config`, the queue map, and the folder tracker. HTTP handlers validate
input and hand mutations to that goroutine through `onMainLoop`. Review with that model
in mind; the items below have been raised and rejected before.

## Concurrency

- Live `Config` fields are read and written only on the main loop. Do not ask for a
  mutex around `u.StartDelay`, `u.Passwords`, `u.Sonarr`, and similar. If a new reader
  runs on another goroutine, route it through `onMainLoop` instead.
- `retrieveAppQueues` does not need to snapshot the app lists. A config PUT applies on
  the same goroutine, which is parked in `wait.Wait()` until every poll returns.
- `syncFileUIPassword` is not a lock-order inversion. `uiPassword()` releases
  `uiPassMu` before `configMu` is taken.
- Two administrators saving config at the same instant is not a design target.
  Do not propose snapshot-and-merge logic whose only purpose is concurrent PUTs.
- `History.mu`, `histMu`, `configMu`, and `uiPassMu` each guard one thing for HTTP
  readers. Do not suggest adding a fifth lock; suggest moving the work to the main loop.

## Validation and input

- Starr URLs are checked for an `http://` or `https://` prefix, matching startup.
  Do not request `url.Parse` or a non-empty host.
- The history JSONL is written by this process, capped at `keep_history`, and read
  with `bufio.Reader.ReadBytes`. It is not untrusted input. Do not request line
  caps, bounded readers, atomic rename, rollback copies, or `.bak` handling for it.
- A local admin POST does not need context-cancellation checks between enqueue and
  execution on the main loop.
- `New()` allocates `Config`, `Webserver`, `History`, and `folders`. Nil checks on
  those fields in HTTP handlers are dead code.
- The tray builds its menus in `readyTray` before `go u.Run()`, and a config PUT
  cannot apply until the loop drains `taskChan`, so those reads of live `Config`
  are ordered before any possible write. They are not a race and do not need a
  lock. When the web UI replaces the tray history menu it reads `/api/history`,
  which is already guarded by `histMu`.
- `filepath:` values are kept as written in `fileConfig` and expanded on the live
  copy only (`expandFilepaths`). PUT may keep an existing `filepath:` string in the
  same section. A new or changed `filepath:` is 400; the API must not read a file
  the operator did not already put in that section of the config.

## Tests

- Do not force a write failure with a read-only directory. The container image
  runs as root, which ignores the permission bits, and Windows ignores them
  outright. Use `blockedPath`, which puts the target under a regular file so
  the write fails with ENOTDIR for every user and platform.

## Config PUT

- Sections that the loop cannot re-apply in place return `restartRequired: true`
  and set `pendingRestart`; the loop re-execs itself once the queue is idle
  (`maybeRestart`). Folders and listener/TLS/logger changes fall in this group.
  Do not request an in-process watcher or listener rebuild.
- General PUT resets the loop tickers directly; interval changes do not need a restart.
