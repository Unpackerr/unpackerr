# GitHub Actions

Two workflows. `test-and-lint` (`codetests.yml`) runs tests and golangci-lint on push and `pull_request_target`. `build-and-release` (`release.yml`) is the only publisher.

## Channels

`release.yml` maps the GitHub event to a `CHANNEL` env that `.goreleaser.yaml` reads (`dockers_v2.disable`, packagecloud repo, unstable upload). `--nightly` is a GoReleaser flag: it bumps the version and turns off GitHub Releases / brew / AUR.

| Trigger | CHANNEL | GoReleaser extra | What it publishes |
|---|---|---|---|
| Push tag `v*` | `release` | (none) | GitHub Release, Docker `:latest` + version tags, Homebrew cask, AUR, packagecloud `golift/pkgs` |
| Push branch `unstable` | `unstable` | `--nightly` | Docker `:unstable`, packagecloud `golift/unstable`, [unstable.golift.io](https://unstable.golift.io/?dir=unpackerr) |
| Cron `27 12 * * *` UTC, or `workflow_dispatch` on `main` | `nightly` | `--nightly` | Docker `:nightly` only |

`unstable` is a **manual publish branch**. Recut it by pushing the commit you want:

```bash
git push unpackerr ci/goreleaser-pro:unstable
```

Do not fast-forward `unstable` from `main` in CI. Calendar nightly builds `main` and does not touch the git `unstable` branch.

Nightly skips the Darwin job. Apple notarization is slow and unused for a Docker-only cut.

## Split, then merge

GoReleaser Pro `--split` / `--continue --merge` builds each GOOS in its own job, then one merge job publishes. That exists because:

- Darwin needs **CGO** (`energye/systray` Cocoa) and **native** `codesign` / `notarytool`. Quill on Linux can sign a naked binary; a `.app` inside a DMG is rejected by Gatekeeper unless the bundle is signed on macOS.
- Windows Authenticode talks to house **signerd** (`golift.io/codesign`) and needs `id-token: write` for GitHub OIDC. That is ubuntu, not macOS.
- Linux nFPM (deb/rpm) needs `rpm` + GPG. FreeBSD is just archives.

So:

1. **channel** — compute `CHANNEL` + extra args. Nothing else.
2. **Build: linux / windows / freebsd** (`split` on ubuntu) — `release --clean --split`. Filter with **`GGOOS`**, not `GOOS`. `GOOS` leaks into `go run` before-hooks (man pages, rsrc) and they then target the wrong OS.
3. **Build: darwin** (`split-darwin` on macos-latest, skipped on nightly) — import Developer ID + App Store Connect key, same `--split` with `GGOOS=darwin`, staple the DMG.
4. **release** — download `dist-*` artifacts, `continue --merge`. This is the only job that pushes Docker / GitHub / brew / AUR / packagecloud / unstable.golift.io.

`REVISION` is `git rev-list --count --all`. It must be in the goreleaser-action `env:` map (Actions does not automatically forward `GITHUB_ENV` into a later step’s `env:` block). Nightly/unstable versions are `{{ incpatch .Version }}-{{ .Env.REVISION }}` (example `0.15.3-1045`). nFPM `release` is not templated; uniqueness is that version string. Do not put `CHANNEL` in the package version.

## Darwin signing

`.github/scripts/macos_keychain.sh` is **required** on Build: darwin. Missing `MACOS_SIGN_*` / `MACOS_NOTARY_*` fails the job; there is no unsigned-DMG fallback.

`notarize.macos_native.ids` must be the **app bundle** and **DMG** ids (`unpackerr-app`, `unpackerr-dmg`), not the Darwin build id. Those pipes match Extra.ID on the `.app` / `.dmg`. After GoReleaser, `.github/scripts/macos_staple.sh` checks Developer ID on `Unpackerr.app` and staples the DMG (CloudKit can lag a bit after `notarytool` says Accepted).

## Merge destinations

- **Docker** — always `ghcr.io/unpackerr/unpackerr`. Hub `docker.io/golift/unpackerr` only when `DOCKERHUB_PASSWORD` is set (`DOCKERHUB_PUBLISH=1`). Platforms: `linux/amd64`, `linux/arm64`, `linux/arm/v7`.
- **GitHub Release** — tagged `v*` only (`release.disable: "{{ .IsNightly }}"`).
- **Homebrew** — `homebrew_casks` → `golift/homebrew-mugs` `Casks/`. Skip on `--nightly`.
- **AUR** — `aur_sources` over SSH. Skip on `--nightly`.
- **packagecloud** — `golift/pkgs` vs `golift/unstable`. Skip when `CHANNEL=nightly`.
- **unstable.golift.io** — only `CHANNEL=unstable`. Auto-update URLs are **stable names**; version lives in a sibling `.txt` (plain `0.15.3-1045`, not JSON). Payload is a gzipped/zipped **binary**, not the versioned `tar.gz`. Script: `.github/scripts/unstable_upload.sh`. Upload overwrites by name.

| Stable name | Payload |
|---|---|
| `Unpackerr.dmg` | notarized universal DMG |
| `unpackerr.amd64.exe.zip` | Windows exe zip |
| `unpackerr.{amd64,386,arm,arm64}.linux.gz` | gzipped binary |
| `unpackerr.{amd64,i386,armhf,arm64}.freebsd.gz` | gzipped binary |

Linux nFPM arches are amd64, arm64, i386, armv7 (one `armhf`). Darwin min macOS 13. Windows is `-H=windowsgui`.

## Secrets

Set on the `Unpackerr/unpackerr` repo (or org, granted to this public repo):

| Secret | Used by |
|---|---|
| `GORELEASER_PRO_KEY` | every goreleaser-action |
| `GPG_SIGNING_KEY` | Linux nFPM signatures |
| `MACOS_SIGN_P12`, `MACOS_SIGN_PASSWORD` | Developer ID `.p12` (PEM or long-line base64) |
| `MACOS_NOTARY_KEY`, `MACOS_NOTARY_KEY_ID`, `MACOS_NOTARY_ISSUER_ID` | App Store Connect `.p8` |
| `CODESIGN_URL`, `CODESIGN_CLIENT_CERT`, `CODESIGN_CLIENT_KEY` | Windows Authenticode (OIDC + mTLS) |
| `DOCKERHUB_PASSWORD` | Hub login; absence skips Hub only |
| `HOMEBREW_TAP_GITHUB_TOKEN` | `golift/homebrew-mugs` |
| `PACKAGECLOUD_TOKEN` | `golift/pkgs` / `golift/unstable` |
| `AUR_DEPLOY_KEY` | AUR `unpackerr` |
| `UNSTABLE_UPLOAD_KEY` | unstable.golift.io |

`GITHUB_TOKEN` is the default Actions token (GHCR + GitHub Releases).

## Action pins

`release.yml` pins `owner/repo@<commit-sha> # vX.Y.Z`. Floating major tags (`@v4`) are not used there.
