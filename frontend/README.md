# Unpackerr Web UI

Svelte 5 + Vite 8 + sveltestrap SPA for the Unpackerr HTTP API. It consumes only the
documented endpoints in `pkg/unpackerr/openapi.json` (`GET /api/openapi.json`).
UI strings live in `src/lib/i18n/locales/` (`en.json` is canonical). Locales:
English, Spanish, Greek, and Dutch.

Production builds land in `dist/` and are `go:embed`ed by `frontend.go`. The Go
process serves that tree at the configured urlbase (SPA fallback to `index.html`).

## Run it locally

1. Start unpackerr with the web server on and a known password (headless on macOS
   because `USEGUI` is not `true`):

   ```sh
   cd ../            # repo root
   UN_WEBSERVER_LISTEN_ADDR=127.0.0.1:5656 \
   UN_WEBSERVER_UI_PASSWORD='admin:supersecret123' \
   go run . -c /tmp/unpackerr.conf
   ```

   (Or run a normal install and grab the temporary password it prints, or set one
   with `unpackerr --reset`.)

2. Start the dev server (proxies `/api`, `/metrics`, and `/ws` to the backend):

   ```sh
   npm install
   npm run dev
   ```

   Point it at a different backend with `UN_BACKEND=http://host:port npm run dev`.

3. Open http://localhost:5173 and log in (`admin` / `supersecret123` above).

To try the embedded UI instead of Vite: `go generate ./frontend` (or
`npm run build` in `frontend/`) then `go run .` and open the listen address
(urlbase cookie tells the SPA where `/api` lives).

## Scripts

- `npm run dev` – dev server with API proxy
- `npm run build` – production bundle into `dist/`
- `go generate ./frontend` / `./generate.sh` – `npm ci` (skipped if `node_modules` exists) + `npm run build`
- `npm run check` – svelte-check / TypeScript
- `npm run preview` – serve the built bundle

## Notes

- Login and password changes run PBKDF2-HMAC-SHA-256 in the browser (Web Crypto)
  and post only the hex digest (`kdf` on login, `user:<kdf>` plus `uiCurrentKdf`
  on config PUT). The plaintext password never leaves the page.
- Settings read the on-disk (`GET /api/config/{section}`) shape and PUT it back.
  They never post `/live`. A paired live GET greys fields that an `UN_*` env var
  currently overlays. Starr and hook instance cards can POST `/api/config/{section}/test`
  (write perm) to probe a queue or fire one sample payload without saving. The UI does not advertise or add `filepath:` values.
- Auth type is local password, proxy header, or no auth. Header and no-auth are
  disabled unless `/api/auth/me` reports `upstreamAllowed` (client IP in
  Upstreams), so you cannot lock yourself out from a non-proxy address.
- `dist/` is ignored except tracked `dist/.gitignore`, so `//go:embed all:dist` and
  `go test` compile without npm. `go generate ./...` (CI tests, GoReleaser, local)
  runs `frontend/generate.sh`.
