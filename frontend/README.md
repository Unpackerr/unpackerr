# Unpackerr Web UI

Svelte 5 + Vite 8 SPA, `go:embed`ed by `frontend.go`. This change lands the
toolchain so `go generate ./frontend` (CI, Docker, `make generate`) produces a
real `dist/`. Later stacked PRs add login, the live dashboard, and settings.

Production builds land in `dist/` and are served at the configured urlbase
(SPA fallback to `index.html`). Vite `base` is `./` so assets work under a
non-root urlbase.

## Scripts

- `npm run dev` – Vite, proxies `/api`, `/metrics`, `/ws` to `UN_BACKEND` or `http://127.0.0.1:5656`
- `npm run build` – production bundle into `dist/`
- `go generate ./frontend` / `./generate.sh` – `npm ci` (skipped if `node_modules` exists) + `npm run build`
- `npm run check` – svelte-check / TypeScript

`dist/` is ignored except tracked `dist/.gitignore`, so `//go:embed all:dist`
and `go test` compile without npm.
