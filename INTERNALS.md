# Unpackerr internals

This is the operator-and-future-dev map of the HTTP API, the two in-memory configs, and the main-loop ownership model. It exists so the next person (or LLM) does not “fix” races that are not races, bake `UN_*` secrets into TOML, or expand `filepath:` onto disk.

The machine-readable contract is `GET /api/openapi.json` (unauthenticated). This file is the *why*. Review assumptions that have already been argued to death live in [`.github/copilot-instructions.md`](.github/copilot-instructions.md).

---

## What landed (the stacks)

Two stacked feature series plus a follow-up mux swap. Closed duplicates (`#688`, `#689`) were rebuilt and merged as `#692` / `#693`.

| PR | Title | What it actually did |
| --- | --- | --- |
| [#678](https://github.com/Unpackerr/unpackerr/pull/678) | Listen from `listen_addr` | Web server starts when `listen_addr` is non-empty, not when `metrics` is true. Default installs listen on `:5656`. Empty `listen_addr` disables the server. |
| [#679](https://github.com/Unpackerr/unpackerr/pull/679) | `pkg/configdef` | Example TOML / compose / docs come from `definitions.yml`. Atomic config writes use the same renderer. |
| [#680](https://github.com/Unpackerr/unpackerr/pull/680) | UI password | `ui_password` hashing, `filepath:`, `webauth` / `noauth`, `--reset`, tray Change Password. File snapshot **before** `UN_*` overlay so env is not written back. |
| [#681](https://github.com/Unpackerr/unpackerr/pull/681) | Named API keys + roles | `apiKeys[]`, custom `roles`, permission constants, built-in `admin` / `*`. |
| [#682](https://github.com/Unpackerr/unpackerr/pull/682) | HTTP login | Session cookie, `X-Api-Key` / Bearer, login KDF, proxy auth, generated admin key. |
| [#683](https://github.com/Unpackerr/unpackerr/pull/683) | Stats / system / metrics | `GET /api/stats`, `GET /api/system`, authenticated `/metrics`. |
| [#685](https://github.com/Unpackerr/unpackerr/pull/685) | Queue + history GET | JSONL extract history; `GET /api/queue` and `GET /api/history`. |
| [#686](https://github.com/Unpackerr/unpackerr/pull/686) | Queue + history writes | Retry/forget; history delete/clear. |
| [#687](https://github.com/Unpackerr/unpackerr/pull/687) | Config GET | `GET /api/config/{section}` and `/live`. |
| [#692](https://github.com/Unpackerr/unpackerr/pull/692) | Config PUT | Per-section PUT, `onMainLoop`, idle restart. Replaced the overbuilt `#688`. |
| [#693](https://github.com/Unpackerr/unpackerr/pull/693) | OpenAPI | Embedded `pkg/unpackerr/openapi.json`. |
| [#697](https://github.com/Unpackerr/unpackerr/pull/697) | Stdlib mux | Dropped `julienschmidt/httprouter`. Go `http.ServeMux` with `{section}` and `GET …/{$}` for the index. |

The mux PR is routing only. Behavior below is from the API stacks unless noted.

---

## Process model (who owns what)

Unpackerr is **one process**. One goroutine — `(*Unpackerr).Run()` in `pkg/unpackerr/start.go` — owns:

- live `*Config` (embedded on `Unpackerr`)
- the extract queue map (`History.Map`)
- the folder tracker (`u.folders`)
- tickers (`poller`, `start delay`, `progress`, `log queues`)
- `pendingRestart`

HTTP handlers **must not** mutate those on the HTTP goroutine. They validate the body, then call `onMainLoop`. Queue retry/forget use the same handoff. Config GET of the **file** snapshot does **not** need the main loop (it is under `configMu`). Config GET of **live** general/starr/folders **does**, because live `Config` is main-loop memory.

If a new reader of live `u.Sonarr` / `u.Passwords` / `u.StartDelay` runs off the main loop, do not add a mutex. Route it through `onMainLoop`.

---

## `onMainLoop`

```go
// pkg/unpackerr/mainloop.go
func (u *Unpackerr) onMainLoop(ctx context.Context, fn func() error) error
```

It allocates a `mainTask{fn, result chan error}`, sends it on `u.taskChan` (buffer 100, same as extract callbacks), and waits for `fn` to run **on `Run()`**.

`Run()`’s `select` already had poller / xtractr / cleaner / folder / progress cases. The API work added:

```text
case task := <-u.taskChan:
    task.result <- task.fn()
```

That is **not** a new goroutine. It serializes HTTP mutations onto the loop that already owns the maps.

If `ctx` is done before the send or before the result, the handler gets `504` (`context.Canceled` / `DeadlineExceeded`). The task may still run if it was already dequeued — local admin POSTs are not designed for cancellation-between-enqueue-and-run. Do not add that.

Tests that exercise HTTP without `Run()` call `go unpack.runMainTasks(ctx)`, which only drains `taskChan`.

The `//nolint:contextcheck` on config PUT is because hook-worker startup (`ensureHookWorker`) outlives the request.

---

## Goroutines (what we spawn)

Count **ours**, not `net/http` per-connection goroutines or `xtractr` extract workers.

| Goroutine | When | New with the API stack? |
| --- | --- | --- |
| `Run()` | Always (either this is `Start`’s goroutine, or `go u.Run()` from the tray) | No. Now also drains `taskChan`. |
| `watchDeleteChannel` | Always after start | No |
| `watchCmdAndWebhooks` | First hook at startup **or** first webhook/cmdhook PUT (`sync.Once`) | Once-on-PUT is new |
| `runWebServer` | `listen_addr` set | Yes relative to pre-#678 (metrics-only) |
| Starr poll workers (`workChan`) | `max(1, starrAppCount)`, **grows never shrinks** on Starr PUT | Grow-on-PUT is new |
| `folders.watchFSNotify` | At least one watch folder | No |
| folder poller `Watcher.Start` | Folder interval ≥ minimum | No |
| tray: `watchKillerChannels` / `watchDebugChannels` | GUI builds | No |

**Idle restart does not add a goroutine.** `maybeRestart` runs on the existing cleaner tick (5s). Unix `syscall.Exec` replaces the process (same PID). Windows starts a copy and `os.Exit(0)`.

xtractr’s `Parallel` extract workers existed before. General PUT that changes `parallel` sets `restartRequired` because the queue is built once at start.

---

## Two configs in memory

There are **two** `*Config` values after startup:

### 1. Live — `u.Config` (embedded)

What the daemon *runs*: Starr HTTP clients, expanded secrets, hashed UI password, env overlays, clamped defaults.

**Owner:** `Run()`. Writes only from `onMainLoop` apply closures (and startup before `Run()`).

**HTTP readers that are allowed off-loop:**

- `uiPassMu` covers live webserver **auth** fields HTTP hits every request: `UIPassword`, `APIKeys`, `Roles`, `keyPerms`, `Upstreams`, `allow`. PUT applies those under that lock via `applyLiveWebserverAuth`.
- Stats/queue/history have their own locks (`History.mu`, `histMu`, `configMu` for hook *counts*).

Everything else live is main-loop only.

### 2. File-shaped — `u.fileConfig`

Clone of config **after TOML load, before `UN_*` overlay**. Keeps:

- `filepath:/path` as the string `filepath:/path` (not file contents)
- on-disk `ui_password` (`!!cryptd!!…`, `filepath:…`, or leftover plaintext)
- API keys as stored in the file (not env-only keys)
- Starr lists / folders / hooks as written

**Owner:** `configMu`. Also written by the **tray** (Change Password, generated admin key). HTTP GET of the file snapshot clones under that lock. PUT stages a clone, atomically writes TOML, then swaps `fileConfig`.

If there is no config path (env-only), `fileConfig` still exists as a snapshot of pre-env defaults, but `writeConfigFrom` returns `errNoConfigFile` and that is **not** a PUT error. Live still applies.

### 3. Extra slice: `u.livePasswords`

Archive passwords **after env overlay, before `filepath:` expansion**. `GET /api/config/general/live` returns this so operators see `filepath:/secrets` (or `UN_PASSWORDS`) instead of the rar passwords that were read from disk. Live extract uses `u.Passwords` (expanded).

---

## Startup load order (do not reorder)

`unmarshalConfig` in `cnfgfile.go`:

1. Find or create a TOML file (`configdef` example on first run).
2. `cnfgfile.Unmarshal` into `u.Config`.
3. **`snapshotFileConfig()`** — this is the file-shaped copy. **Must happen before env.**
4. `cnfg.UnmarshalENV(u.Config, u.EnvPrefix)` — default prefix `UN`. `UN_SONARR_0_API_KEY`, `UN_WEBSERVER_UI_PASSWORD`, `UN_WEBSERVER_ROLES_stats_PERMISSIONS_0`, etc.
5. `snapshotLivePasswords()` — copy `Passwords` (post-env, still `filepath:`).
6. Password / UI password / API key setup (hash, generate, `--reset`).
7. `validateAuth`, normalize + **validate URLBase** (`{` / `}` forbidden; ServeMux wildcards).
8. `validateConfig` / `clampConfig`.

Later in `Start()`, `cnfgfile.Parse(..., Prefix: "filepath:")` expands **live** `Config` in place. File snapshot is not parsed.

Then: xtractr queue, hook worker, delete worker, HTTP server, Starr work threads, tray or `Run()`.

---

## `filepath:` vs env vs GET vs PUT

### `filepath:`

Any string field `cnfgfile` walks can be `filepath:/abs/or/~/path`. At startup and on PUT of the **live** copy, the file contents replace the string (passwords: one secret per line).

**Disk and `fileConfig` keep the prefix.** GET file returns that string, TOML writes that string, live client uses the file contents. That was a real bug in the first PUT stack: round-trip kept `filepath:` on disk but published the literal to the Starr client.

**PUT cannot introduce a new `filepath:`.** Every `filepath:` string in the body must already appear in the same file-config section. Changing `filepath:/a` to `filepath:/b` is 400. Replacing `filepath:` with a literal (or omitting it) is allowed. The check runs **before** `expandFilepaths` / `expandPasswords` / `expandCryptPassFile` so a rejected PUT does not read the target file. Operators who need a new secret file edit the TOML (or env), not the API.

Archive passwords: expansion is `expandPasswords` (newline split). Other fields: `expandFilepaths` (`cnfgfile.Parse`).

Empty / missing secret file is an error on PUT (400) when the `filepath:` was already allowed. Startup already failed that way for live parse.

### Env (`UN_*`)

Env overlays **live only**. They are not merged into `fileConfig`. A later PUT that rewrites the whole Starr list from a **file GET** will not include an env-only extra server — that is intentional. Env-only extra keys/roles exist at runtime until restart unless you add them in the PUT body.

`UN_WEBSERVER_UI_PASSWORD`:

- Wins at runtime.
- Is not written back.
- Tray Change Password is hidden when it is set.
- A **present empty** env value can wipe a stored hash (documented in definitions). Do not “helpfully” treat empty as omit.

### GET `/api/config/{section}` (file)

Permission: `read:config:{section}`.

Returns `fileConfig` (cloned). Webserver GET:

- `apiKeys[].key` is **blank** unless the caller has `*` (`PermAll`). Names and roles stay.
- `ui_password` is never plaintext `user:pass`. Crypt, `webauth`, `noauth`, `filepath:` stay. If the file snapshot is still plaintext because env supplied the real password, GET **blanks** it.

Starr API keys in the sonarr/… sections are **visible** to anyone with that section’s read perm. They are not webserver API keys.

### GET `/api/config/{section}/live`

Same read perm. Running shape:

- Env overlays included.
- `filepath:` expanded **except** general passwords (`livePasswords`).
- UI password hashed / webauth as used for login.
- Webserver key redaction same as file GET (`*` to see secrets).

Live GET of general/starr/folders/hooks runs `onMainLoop` so it does not race `Run()`.

### PUT `/api/config/{section}`

Permission: `write:config:{section}`.

Body **replaces** the section (not patch). Workflow the UI is built for: GET file → edit → PUT.

Handler on HTTP goroutine: read ≤1 MiB, reject trailing JSON, reject `null` / empty object `{}` for object sections, reject `[null]` in lists. `DisallowUnknownFields`. Webserver PUT that is only `uiCurrentKdf` is the same empty-section 400 (`uiCurrentKdf` is a sidecar, not a config field).

Then `onMainLoop` → `replaceConfigSection` → `commitConfig`:

1. Clone `fileConfig`, mutate the **unexpanded** section onto the clone.
2. Atomic write TOML (`configdef.AtomicWrite`). Failure → **500**, live unchanged (`errPersistConfig`).
3. Swap `fileConfig` to the clone.
4. `applyLive()`: expand, validate, swap live lists / general fields / webserver auth.

Env-only (no config path): skip write, still apply live.

**Empty `apiKeys[].key`:** treated as “keep existing secret of this **name**”. File fill from **file snapshot only** (so an env-overlay key is not persisted). Live fill from live then file. That is how a redacted GET round-trips without `*`.

**Omitted / blank `ui_password` on PUT:** keep live password. An already-stored `filepath:` on PUT: expand for live, store the `filepath:` string on disk. A new `filepath:` is 400.

**New `ui_password` on PUT:** `!!cryptd!!…`, `webauth`, `noauth`, or `user:<64-char hex>` where the hex is the same PBKDF2 digest as login (`CryptPass.Set`, then bcrypt; mixed-case hex is stored lowercase). Plaintext `user:pass` is **400**. While live auth is local password, changing the hash or switching to header/noauth requires `uiCurrentKdf` (login `Valid()` on the current username). Header/noauth live mode does not. `uiCurrentKdf` is a PUT-only JSON field and is never written to TOML. A body that contains only `uiCurrentKdf` is **400** (empty section), so it cannot wipe `listen_addr` / keys / roles. Omitting `uiPassword` or sending the on-disk value unchanged keeps the live overlay, so `UN_WEBSERVER_UI_PASSWORD` is not replaced by the file hash.

**Starr PUT:** invalid URL/key is **400** (startup *skips* bad apps; PUT does not). `path` merges into `paths` without dupes. Last poll `Queue` carries over when `url` + expanded `apiKey` match. Work thread pool **grows** to `starrAppCount`.

**Folders PUT:** always `restartRequired: true`. Watcher is built once; rebuilding in-process was rejected (leak / dual poller).

**Webhooks / cmdhooks PUT:** validate (including HTTP client) then publish. First-ever hook starts the hook worker.

**General PUT:** applies interval / delays / remnant action / keep_history / passwords in place and **`resetTickers()`**. Interval is **not** `restartRequired`. Logger construction, `parallel` (xtractr), `file_mode` / `dir_mode`, `timeout` / `delete_delay` (copied into apps at validate time) **are** restart.

**Webserver PUT:** keys/roles/password/upstreams apply in place under `uiPassMu`. `listen_addr`, `urlbase`, TLS, metrics, pprof, HTTP log file settings stay on the running `http.Server` and set `restartRequired`. `urlbase` must not contain `{` or `}`.

Reply:

```json
{"status":"ok","restartRequired":true}
```

`pendingRestart` is sticky OR: one folders PUT then a general PUT that does not need restart still restarts when idle.

---

## Idle restart

When `restartRequired`, `u.pendingRestart = true`. Every cleaner tick (5s) `maybeRestart()`:

- Not pending or not `idle()` → return.
- `idle()` is false if `inFlight` (deletes/hooks) > 0, extract/folder callback channels have work, a folder is `QUEUED`/`EXTRACTING`, or a map item is `QUEUED`/`EXTRACTING`/`EXTRACTED`/`IMPORTED`/`DELETING`.
- `WAITING` and `EXTRACTFAILED` do **not** block (Starr will rediscover them).
- Resolve `os.Executable()` **before** shutting the listener. Then `Shutdown` (5s), then `restartProcess`. Unix exec; Windows spawn+exit.

Tray on macOS/Windows after re-exec may need a click to reappear. Do not invent an in-process listener rebuild.

---

## HTTP routes

All paths sit under configured `urlbase` (normalized to `/` or `/foo/`) except `/metrics` which is registered at **both** `/metrics` and `{urlbase}metrics` when urlbase is not `/`.

Index is `GET {urlbase}{$}` so `GET /` is not a ServeMux prefix match (that would 405 `POST /api/auth/login`).

| Method | Path | Auth | Perm | Notes |
| --- | --- | --- | --- | --- |
| GET | `{urlbase}` | none | — | `"Welcome!\n"` |
| GET | `{urlbase}api/openapi.json` | none | — | Spec; `servers[0].url` rewritten to urlbase |
| POST | `{urlbase}api/auth/login` | none | — | JSON `{name?, kdf}`. 3s fail delay. 5s read deadline **outside** apache log wrapper. Missing if cookies failed to init. |
| POST | `{urlbase}api/auth/logout` | none | — | Clears session cookie |
| GET | `{urlbase}api/auth/me` | yes | any auth | Session / key / proxy identity + permissions, `header`, `clientIP`, `upstreamAllowed`. `headers` (this request minus the Trust exclusion list) only with `read:system:headers` |
| GET | `{urlbase}api/stats` | yes | `read:system:stats` | Queue counts + hook counters |
| GET | `{urlbase}api/system` | yes | `read:system:info` | Version, uptime, bind addr, urlbase, auth type, metrics flag |
| GET | `{urlbase}api/queue` | yes | `read:system:queue` | In-flight items |
| POST | `{urlbase}api/queue/retry` | yes | `write:system:queue` | `{id}`; only `extractfailed`; Starr → `WAITING`; folder resets on main loop |
| POST | `{urlbase}api/queue/forget` | yes | `write:system:queue` | Terminal statuses only; in-progress **409**; Starr titles get a tombstone until they leave the upstream queue |
| GET | `{urlbase}api/history` | yes | `read:system:history` | Durable JSONL-backed rows |
| POST | `{urlbase}api/history/clear` | yes | `write:system:history` | |
| POST | `{urlbase}api/history/delete` | yes | `write:system:history` | `{id}` |
| GET | `{urlbase}api/browse` | yes | `read:system:browse` | `?dir=`; empty → home; file path lists parent; unreadable path with readable parent is 200 + `error`; both fail → 406. Windows empty/`/`/`\` lists `C:\`–`Z:\` that exist. |
| POST | `{urlbase}api/browse` | yes | `write:system:browse` | `{path}`; folder `MkdirAll` 0755 (existing folders succeed) |
| GET | `{urlbase}api/config/env` | yes | any auth | UN_* overlays from startup; secret values blank unless `*` |
| GET | `{urlbase}api/config/{section}` | yes | `read:config:{section}` | File snapshot |
| GET | `{urlbase}api/config/{section}/live` | yes | `read:config:{section}` | Running copy |
| PUT | `{urlbase}api/config/{section}` | yes | `write:config:{section}` | Replace section |
| GET | `/metrics` (+ urlbase) | **API key / Bearer only** | `read:system:metrics` | No session cookie, no webauth/noauth |
| GET | `/debug/pprof/…` | none extra | — | Only if `pprof = true`. Treat as a loaded gun. |

`{section}` is one of: `general`, `webserver`, `sonarr`, `radarr`, `lidarr`, `readarr`, `whisparr`, `folders`, `webhooks`, `cmdhooks`. Unknown → 404 from `requireConfigPerm`.

Stdlib mux does **not** redirect trailing slashes the way httprouter did. `/api/stats/` is 404. Documented as acceptable for this API (no external consumers). Do not add a compatibility wrapper unless product asks.

---

## Auth

Order in `authenticate`:

1. `X-Api-Key` (exact key lookup → that key’s permissions)
2. `Authorization: Bearer …` (same)
3. Proxy `webauth` / header auth if `ui_password` is that type **and** `RemoteAddr` is in `upstreams`
4. Session cookie (login). Session identity gets **all** permissions and the generated/admin API key in `authInfo.apiKey`.

`noauth` is a `ui_password` type for the UI, not “skip API auth”.

Login body: PBKDF2-HMAC-SHA-256 of the password, salt `unpackerr:`+username, 210000 iterations, 32-byte hex in `kdf`. Never send plaintext. Default username `admin` if `name` omitted. `webauth` login returns **403**.

Metrics (`requirePermHTTP`) ignore cookies and proxy auth on purpose so a stolen browser session cannot scrape Prometheus.

First start with listen enabled and no password: generate one, print once, hash, try to write the file (non-fatal if read-only). Same idea for a missing admin API key (`admin` role).

---

## Permissions

`verb:area:resource`. Built-in role `admin` is reserved and means `*`.

System: `read:system:stats|info|queue|history|metrics|headers|browse`, `write:system:queue|history|browse`.

Config: `read:config:{section}`, `write:config:{section}` for each section above.

Custom roles are a map of name → permission list. Env for roles is picky: do **not** set `UN_WEBSERVER_ROLES` itself. Use `UN_WEBSERVER_ROLES_<name>_PERMISSIONS_0=…`.

---

## History JSONL

Path: next to the log file if rotating, else next to the config file, else `~/.unpackerr/unpackerr.history.jsonl`.

Append-only one line per durable transition (`extractfailed`, `extractednothing`, `imported`, `deleted`, `deletefailed`). Compact on load and when appends reach `2 × keep_history`. `histMu` covers records + file; HTTP reads, main loop appends.

This file is **ours**. Do not add line-length caps, atomic rename, or `.bak` “hardening”.

`keep_history = 0` disables. Turning it on via general PUT loads the file on the main loop.

---

## Locks (there are four; do not add a fifth)

| Lock | Guards |
| --- | --- |
| (none — main loop) | live `Config` minus webserver auth, `Map`, folders, tickers, `pendingRestart` |
| `configMu` | `fileConfig` + hook slices `/api/stats` counts |
| `uiPassMu` | live webserver auth fields HTTP reads |
| `histMu` | history records + JSONL |
| `History.mu` | extract map for HTTP queue GET |

`syncFileUIPassword` takes `uiPassword()` (uiPassMu) **then** `configMu`. That order is intentional.

The tray reads live `Config` in `readyTray` **before** `go u.Run()`. A PUT cannot apply until the loop exists and drains `taskChan`. That is not a race.

Two admins saving at once is not a design target. Do not add snapshot-merge.

---

## Other tricks worth remembering

- **Persist-first PUT:** disk write happens before live publish. 500 on write → runtime unchanged.
- **`commitConfig`:** one lock, stage, write, swap file, apply live.
- **Hook worker `sync.Once`:** empty hook list at boot does not start the worker; first PUT that adds a hook does, so the channel is not a deadlock.
- **`inFlight`:** incremented when the **main loop sends** to delete/hook channels, decremented when the worker finishes. Idle restart cannot observe a gap between send and receive.
- **Starr identity for queue carry:** `url + "\x00" + apiKey` after expansion.
- **Login read deadline** sits outside `apachelog.Wrap` because that `ResponseWriter` does not `Unwrap`.
- **Forwarded-For** rewrite only if the peer is in `upstreams`.
- **URLBase** `{`/`}` rejected at load, start, and PUT so ServeMux does not treat urlbase as a wildcard (or panic on `/foo/{/`).
- **Index pattern** `…/{$}` is required with stdlib mux.
- **OpenAPI** is committed JSON, not swag-generated, so the binary cannot accidentally embed secrets.
- **configdef** is the source of example conf, compose env names, and the TOML writer. PUT persist goes through it so comments/defaults stay consistent.
- **`--reset`:** new UI password, write file, print, exit. Not an HTTP route.
- **Websockets** were anticipated (`{urlbase}ws` on a mux that skips apache log). No WS API in this stack yet; do not rip that mux split out casually.

---

## Sections vs restart vs live apply (cheat sheet)

| Section | Live apply | Restart |
| --- | --- | --- |
| general | Yes; `resetTickers`; expand passwords | Logger / parallel / file+dir mode / timeout / delete_delay |
| webserver | Auth fields in place | listen, urlbase, TLS, metrics, pprof, HTTP log |
| sonarr…whisparr | Rebuild clients, carry queues, grow workers | No |
| folders | Live slices updated | **Always** (watcher) |
| webhooks / cmdhooks | Replace lists, ensure worker | No |

---

## Files to read first

| File | Why |
| --- | --- |
| `pkg/unpackerr/mainloop.go` | `onMainLoop` |
| `pkg/unpackerr/start.go` | `Run` select, workers, startup |
| `pkg/unpackerr/cnfgfile.go` | snapshot / env / write |
| `pkg/unpackerr/configapi.go` | GET file vs live, redaction |
| `pkg/unpackerr/configput.go` | PUT, `commitConfig`, expand, restart flags |
| `pkg/unpackerr/auth.go` | login + authenticate order |
| `pkg/unpackerr/permissions.go` | perm strings |
| `pkg/unpackerr/webserver.go` | mux, urlbase, listen |
| `pkg/unpackerr/restart.go` | idle re-exec |
| `pkg/unpackerr/queue_actions.go` | retry/forget |
| `pkg/unpackerr/historyfile.go` | JSONL |
| `pkg/unpackerr/openapi.json` | wire contract |
| `.github/copilot-instructions.md` | what not to “fix” |
