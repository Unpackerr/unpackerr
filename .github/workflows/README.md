# GitHub Actions

Two workflows. `test-and-lint` (`codetests.yml`) runs tests and golangci-lint on push and `pull_request_target`. `build-and-release` (`release.yml`) is the only publisher.

## Channels

`release.yml` maps the GitHub event to a `CHANNEL` env that `.goreleaser.yaml` reads (`dockers_v2.disable`, packagecloud repo, unstable upload). `--nightly` is a GoReleaser flag: it bumps the version and turns off GitHub Releases / AUR.

| Trigger | CHANNEL | GoReleaser extra | What it publishes |
|---|---|---|---|
| Push tag `v*` | `release` | (none) | GitHub Release, Docker `:latest` + version tags, AUR, packagecloud `golift/pkgs` |
| Push branch `unstable` | `unstable` | `--nightly` | Docker `:unstable`, packagecloud `golift/unstable`, [unstable.golift.io](https://unstable.golift.io/?dir=unpackerr) |
| Cron `27 12 * * *` UTC, or `workflow_dispatch` on `main` | `nightly` | `--nightly` | Docker `:nightly` only |

`workflow_dispatch` on any other ref is refused. Tagged and unstable publishes are **push** only.

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
- Linux nFPM (deb/rpm) needs `rpm` + GPG. FreeBSD pkgng `.txz` is built after `--split` by `fpm -t freebsd` (nFPM has no freebsd target). Wrapper: `.github/scripts/freebsd_txz.sh`.

So:

1. **channel** — compute `CHANNEL`, extra args, and `REVISION` (`git rev-list --count --all`). This is the only place the count is taken; later jobs pass `needs.channel.outputs.revision`.
2. **require secrets** — fail closed if any signing/upload/push secret needed for that channel is empty. Missing secrets used to skip Docker Hub, Windows Authenticode, or unstable.golift.io and still go green.
3. **Build: linux / freebsd** (`split` on ubuntu) and **Build: windows** (own job: OIDC + signerd certs stay off the other legs) — `release --clean --split`. Filter with **`GGOOS`**, not `GOOS`. `GOOS` leaks into `go run` before-hooks (man pages, goversioninfo) and they then target the wrong OS. FreeBSD then runs `freebsd_txz.sh` (`fpm -t freebsd`).
4. **Build: darwin** (`split-darwin` on macos-latest, skipped on nightly) — import Developer ID + App Store Connect key, same `--split` with `GGOOS=darwin`, staple the DMG.
5. **release N** — download `dist-*` artifacts, import GPG (checksum signatures are created at merge, not split), `continue --merge`. Display name is `release` plus that `REVISION`. This is the only job that pushes Docker / GitHub / AUR / packagecloud / unstable.golift.io.

`REVISION` must be in the goreleaser-action `env:` map (Actions does not automatically forward `GITHUB_ENV` into a later step’s `env:` block). On `--nightly`, `nightly.version_template` already bakes it into `{{ .Version }}` (example `0.15.3-1056`), so man-page hooks use `{{ .Version }}` only — do not append `REVISION` again. The env var is still required: that template *creates* `.Version`, and tagged builds keep `.Version` as the semver while ldflags `Revision` / Darwin `CFBundleVersion` / Windows FileVersion still need the count. Darwin `CFBundleShortVersionString` and Windows FileVersion `x.y.z` use the prefix of `{{ .Version }}`, not `.RawVersion` (that stays on the last tag during `--nightly`).

nFPM `release` is not templated (GoReleaser copies it verbatim). `${PKG_RELEASE}` stayed literal and Packagecloud rejected `Version: 0.15.3~1081-${PKG_RELEASE}`. The tagged linux split inserts `release: REVISION` after the `nfpm-release:` marker; `--nightly` leaves it unset. Package files use `{{ replace .ConventionalFileName "~" "-" }}` (GoReleaser `replace` is STRING/OLD/NEW, not sprig). That yields `unpackerr_0.15.2-960_i386.deb`, not `_linux_386`. Do not put `CHANNEL` in the package version. Do not set nFPM `version_metadata`. Tagged FreeBSD packages are `unpackerr-0.15.3_REVISION.amd64.txz`.

## Darwin signing

`.github/scripts/macos_keychain.sh` is **required** on Build: darwin. Missing `MACOS_SIGN_*` / `MACOS_NOTARY_*` fails the job; there is no unsigned-DMG fallback.

`notarize.macos_native.ids` must be the **app bundle** and **DMG** ids (`unpackerr-app`, `unpackerr-dmg`), not the Darwin build id. Those pipes match Extra.ID on the `.app` / `.dmg`. After GoReleaser, `.github/scripts/macos_staple.sh` checks Developer ID on `Unpackerr.app` and staples the DMG (CloudKit can lag a bit after `notarytool` says Accepted). The Darwin `dist/` is packed into one tar before `upload-artifact`; uploading the `.app` tree on macos-latest hangs.

## Merge destinations

- **Docker** — always `ghcr.io/unpackerr/unpackerr` and Hub `docker.io/golift/unpackerr` (`DOCKERHUB_PUBLISH=1`). Empty `DOCKERHUB_PASSWORD` fails the merge job. Platforms: `linux/amd64`, `linux/arm64`, `linux/arm/v7`. `upload-artifact` zip stores files as `0644`; the merge job `chmod 0755`s `dist/linux/**/unpackerr` and the Dockerfile `COPY --chmod=755` so the image entrypoint is executable.
- **GitHub Release** — tagged `v*` only (`release.disable: "{{ .IsNightly }}"`). macOS is the notarized `Unpackerr.dmg`. Windows assets are `unpackerr.amd64.exe.zip`. FreeBSD assets are pkgng `unpackerr-<version>.{amd64,i386,armhf,arm64}.txz`. Homebrew is unsupported.
- **AUR** — tagged `CHANNEL=release` only, after packagecloud. `.github/scripts/aur_publish.sh` uses `dist/unpackerr-VERSION.tar.gz` from `continue --merge` (CI fails if that file is missing). GoReleaser `aur_sources` cannot split/merge: `--split` fatals `no linux archives found` (source tarball is merge-only), and merge Publish finds no PKGBUILD artifacts. Skip nightly/unstable. nFPM `.pkg.tar.zst` on the GitHub Release is a separate binary package.
- **packagecloud** — `golift/pkgs` vs `golift/unstable`. Skip when `CHANNEL=nightly`.
- **unstable.golift.io** — only `CHANNEL=unstable`. Auto-update URLs are **stable names**; version lives in a sibling `.txt` (plain `0.15.3-1056`, not JSON). Payload is a gzipped/zipped **binary**, not the versioned `tar.gz`. Script: `.github/scripts/unstable_upload.sh` (reads `dist/$GOOS/artifacts.json` after split/merge). Upload overwrites by name. Empty `UNSTABLE_UPLOAD_KEY` fails in GitHub Actions.

| Stable name | Payload |
|---|---|
| `Unpackerr.dmg` | notarized universal DMG |
| `unpackerr.amd64.exe.zip` | Windows exe zip |
| `unpackerr.{amd64,386,arm,arm64}.linux.gz` | gzipped binary |
| `unpackerr.{amd64,i386,armhf,arm64}.freebsd.gz` | gzipped binary |

Linux nFPM names are conventional (`unpackerr_0.15.3-1081_amd64.deb`, `unpackerr-0.15.3-1081.x86_64.rpm`). Arches: amd64, arm64, i386, armv7 (one `armhf` / RPM `armv7hl`). Darwin min macOS 13. `CFBundleShortVersionString` and Windows FileVersion both take the `x.y.z` prefix of `{{ .Version }}` (not `.RawVersion`: that stays on the last tag during `--nightly`); Windows FileVersion’s fourth number is `REVISION`. Windows is `-H=windowsgui`. Empty `CODESIGN_URL` fails in GitHub Actions (local snapshots still skip).

## Secrets

Set on the `Unpackerr/unpackerr` repo (or org, granted to this public repo). `.github/scripts/require_secrets.sh` runs before any build job and **fails the workflow** if a secret required for that `CHANNEL` is empty. Nightly does not require Apple / AUR / packagecloud / unstable-upload secrets (those destinations are skipped).

| Secret | Used by |
|---|---|
| `GORELEASER_PRO_KEY` | every goreleaser-action |
| `GPG_SIGNING_KEY` | Linux nFPM signatures |
| `MACOS_SIGN_P12`, `MACOS_SIGN_PASSWORD` | Developer ID `.p12` (PEM or long-line base64) |
| `MACOS_NOTARY_KEY`, `MACOS_NOTARY_KEY_ID`, `MACOS_NOTARY_ISSUER_ID` | App Store Connect `.p8` |
| `CODESIGN_URL`, `CODESIGN_CLIENT_CERT`, `CODESIGN_CLIENT_KEY` | Windows Authenticode (OIDC + mTLS) |
| `DOCKERHUB_PASSWORD` | Hub login (required) |
| `PACKAGECLOUD_TOKEN` | `golift/pkgs` / `golift/unstable` |
| `AUR_DEPLOY_KEY` | AUR `unpackerr` (release channel) |
| `UNSTABLE_UPLOAD_KEY` | unstable.golift.io (unstable channel) |

`GITHUB_TOKEN` is the default Actions token (GHCR + GitHub Releases).

## Action pins

`release.yml` and `codetests.yml` pin `owner/repo@<commit-sha> # vX.Y.Z`. Floating major tags (`@v4`) are not used.

The Docker base image in `init/docker/Dockerfile` is pinned as `alpine:<tag>@sha256:<digest>`. Renovate keeps Action and Dockerfile digest pins current (`helpers:pinGitHubActionDigestsToSemver`; Dockerfile `pinDigests`). Compose examples stay unpinned.

Renovate automerges Go and Docker non-major updates, and GitHub Actions updates including majors, after a 7-day release age when checks pass.
