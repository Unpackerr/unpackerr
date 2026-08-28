#!/usr/bin/env python3
"""Build FreeBSD pkgng .txz packages from GoReleaser freebsd binaries.

nFPM has no freebsd target. The old Makefile used fpm -t freebsd, which wrote
pkgng +MANIFEST / +COMPACT_MANIFEST and a xz-compressed tar (a real .txz, not
a renamed tar.xz). install.sh and pkg/update/check.go still look for assets
ending in amd64.txz / i386.txz / armhf.txz / arm64.txz.

This runs after `goreleaser --split` on the freebsd job and appends Archive
entries to dist/freebsd/artifacts.json so merge uploads them.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path

DESC = "Extracts downloads so Radarr, Sonarr, Lidarr or Readarr may import them."
MAINTAINER = "David Newhall II <captain at golift dot io>"
ORIGIN = "https://github.com/Unpackerr/unpackerr"
WWW = "https://unpackerr.zip"
OSVERSION = "13"

# Filename arch (install.sh / check.go) and pkgng ABI arch.
ARCH_MAP = {
    "amd64": ("amd64", "amd64"),
    "386": ("i386", "i386"),
    "arm": ("armhf", "arm"),
    "arm64": ("arm64", "arm64"),
}


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def repo_root() -> Path:
    return Path(__file__).resolve().parents[2]


def load_version(dist: Path, override: str) -> str:
    if override:
        return override
    meta = dist / "metadata.json"
    if not meta.is_file():
        sys.exit(f"missing {meta} (and no --version)")
    data = json.loads(meta.read_text())
    version = data.get("version") or ""
    if not version or version in {"unknown", "unstable"}:
        sys.exit(f"refusing version {version!r} from {meta}")
    return version


def freebsd_binaries(artifacts: list[dict]) -> list[dict]:
    found: list[dict] = []
    for art in artifacts:
        if art.get("type") != "Binary" or art.get("goos") != "freebsd":
            continue
        if str(art.get("goarch") or "") == "arm" and str(art.get("goarm") or "") == "6":
            continue
        found.append(art)
    if not found:
        sys.exit("no freebsd Binary artifacts (skipping GOARM 6)")
    return found


def resolve_binary(dist: Path, path: str) -> Path:
    p = Path(path)
    if p.is_file():
        return p
    cand = dist / path
    if cand.is_file():
        return cand
    sys.exit(f"freebsd binary missing: {path}")


def stage_files(repo: Path, binary: Path, staging: Path) -> list[str]:
    """Populate a pkgng staging dir. Returns tar member names (no leading /)."""
    files: list[tuple[str, Path, int]] = []

    def put(rel: str, src: Path, mode: int) -> None:
        dest = staging / rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(src, dest)
        dest.chmod(mode)
        files.append((rel, dest, mode))

    bin_mode = 0o755
    data_mode = 0o644
    rc_mode = 0o755

    put("usr/local/bin/unpackerr", binary, bin_mode)
    put("usr/local/etc/rc.d/unpackerr", repo / "init/bsd/freebsd.rc.d", rc_mode)
    conf = repo / "examples/unpackerr.conf.example"
    put("usr/local/etc/unpackerr/unpackerr.conf", conf, data_mode)
    put("usr/local/etc/unpackerr/unpackerr.conf.example", conf, data_mode)

    man = repo / "unpackerr.1.gz"
    if not man.is_file():
        sys.exit(f"missing {man} (goreleaser before-hook should gzip the man page)")
    put("usr/local/share/man/man1/unpackerr.1.gz", man, data_mode)
    put("usr/local/share/doc/unpackerr/LICENSE", repo / "LICENSE", data_mode)

    docs = [
        (repo / "examples/MANUAL.md", "usr/local/share/doc/unpackerr/MANUAL.md"),
        (repo / "examples/MANUAL.html", "usr/local/share/doc/unpackerr/unpackerr_manual.html"),
        (repo / "examples/docker-compose.yml", "usr/local/share/doc/unpackerr/docker-compose.yml"),
        (repo / "examples/unpackerr.conf.example", "usr/local/share/doc/unpackerr/unpackerr.conf.example"),
        (repo / "README.html", "usr/local/share/doc/unpackerr/README.html"),
    ]
    for src, rel in docs:
        if src.is_file():
            put(rel, src, data_mode)

    return [rel for rel, _, _ in files]


def write_manifests(
    staging: Path,
    *,
    version: str,
    abi_arch: str,
    members: list[str],
    scripts: dict[str, str],
) -> None:
    checksums = {}
    for rel in members:
        checksums["/" + rel] = sha256(staging / rel)

    pkgdata = {
        "arch": f"FreeBSD:{OSVERSION}:{abi_arch}",
        "name": "unpackerr",
        "version": version,
        "comment": DESC,
        "desc": DESC,
        "origin": ORIGIN,
        "maintainer": MAINTAINER,
        "www": WWW,
        "prefix": "/",
    }
    compact = staging / "+COMPACT_MANIFEST"
    compact.write_text(json.dumps(pkgdata, separators=(",", ":")) + "\n", encoding="utf-8")
    pkgdata["files"] = checksums
    pkgdata["scripts"] = scripts
    manifest = staging / "+MANIFEST"
    manifest.write_text(json.dumps(pkgdata, separators=(",", ":")) + "\n", encoding="utf-8")


def load_scripts(repo: Path) -> dict[str, str]:
    mapping = {
        "pre-install": repo / "init/systemd/before-install.sh",
        "post-install": repo / "init/systemd/after-install.sh",
        "pre-deinstall": repo / "init/systemd/before-remove.sh",
    }
    out = {}
    for name, path in mapping.items():
        if not path.is_file():
            sys.exit(f"missing pkg script {path}")
        out[name] = path.read_text(encoding="utf-8")
    return out


def gnu_tar() -> str | None:
    for name in ("tar", "gtar", "gnutar"):
        path = shutil.which(name)
        if not path:
            continue
        try:
            proc = subprocess.run([path, "--version"], capture_output=True, text=True, check=False)
        except OSError:
            continue
        if "GNU tar" in (proc.stdout + proc.stderr):
            return path
    return None


def pack_txz(staging: Path, members: list[str], dest: Path) -> None:
    dest.parent.mkdir(parents=True, exist_ok=True)
    file_list = [ "+COMPACT_MANIFEST", "+MANIFEST", *members ]
    tar = gnu_tar()
    if tar:
        # Match fpm: no ./ prefix; leading / on payload files; +MANIFEST at top.
        list_file = staging / ".file_list"
        list_file.write_text("\n".join(file_list) + "\n", encoding="utf-8")
        subprocess.run(
            [
                tar,
                "-Jcf",
                str(dest),
                "-C",
                str(staging),
                "--files-from",
                str(list_file),
                "--transform",
                r"s|^\([^+]\)|/\1|",
            ],
            check=True,
        )
        list_file.unlink(missing_ok=True)
        return

    with tarfile.open(dest, "w:xz", format=tarfile.GNU_FORMAT, dereference=True) as tf:
        for name in file_list:
            path = staging / name
            info = tf.gettarinfo(str(path), arcname=name)
            info.uname = "root"
            info.gname = "wheel"
            info.uid = 0
            info.gid = 0
            if not name.startswith("+"):
                info.name = "/" + name
            with path.open("rb") as fh:
                tf.addfile(info, fh)


def build_one(
    *,
    repo: Path,
    dist: Path,
    binary: Path,
    version: str,
    goarch: str,
    scripts: dict[str, str],
) -> Path:
    if goarch not in ARCH_MAP:
        sys.exit(f"unsupported freebsd goarch {goarch}")
    pkgarch, abi_arch = ARCH_MAP[goarch]
    dest = dist / f"unpackerr-{version}.{pkgarch}.txz"
    with tempfile.TemporaryDirectory(prefix="unpackerr-txz-") as tmp:
        staging = Path(tmp)
        members = stage_files(repo, binary, staging)
        write_manifests(staging, version=version, abi_arch=abi_arch, members=members, scripts=scripts)
        pack_txz(staging, members, dest)
    dest.chmod(stat.S_IRUSR | stat.S_IWUSR | stat.S_IRGRP | stat.S_IROTH)
    print(f"wrote {dest.name} from {binary} ({dest.stat().st_size} bytes)", file=sys.stderr)
    return dest


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("dist", type=Path, help="dist/freebsd (split output)")
    parser.add_argument("--repo", type=Path, default=repo_root())
    parser.add_argument("--version", default="")
    args = parser.parse_args()
    dist = args.dist.resolve()
    repo = args.repo.resolve()
    artifacts_path = dist / "artifacts.json"
    if not artifacts_path.is_file():
        sys.exit(f"missing {artifacts_path}")

    artifacts = json.loads(artifacts_path.read_text())
    if not isinstance(artifacts, list):
        sys.exit(f"{artifacts_path} is not a JSON array")
    version = load_version(dist, args.version)
    scripts = load_scripts(repo)
    built: list[tuple[dict, Path]] = []
    seen_arch: set[str] = set()
    for art in freebsd_binaries(artifacts):
        goarch = str(art.get("goarch") or "")
        if goarch in seen_arch:
            continue
        seen_arch.add(goarch)
        binary = resolve_binary(dist, str(art.get("path") or ""))
        dest = build_one(
            repo=repo,
            dist=dist,
            binary=binary,
            version=version,
            goarch=goarch,
            scripts=scripts,
        )
        built.append((art, dest))

    expected = {"amd64", "386", "arm", "arm64"}
    if not expected.issubset(seen_arch):
        sys.exit(f"freebsd txz missing arches {sorted(expected - seen_arch)}; built {sorted(seen_arch)}")

    for art, dest in built:
        extra = dict(art.get("extra") or {})
        extra["ID"] = "freebsd-pkg"
        extra["Format"] = "txz"
        artifacts.append(
            {
                "name": dest.name,
                "path": dest.name,
                "goos": "freebsd",
                "goarch": art.get("goarch"),
                "goarm": art.get("goarm"),
                "goamd64": art.get("goamd64"),
                "type": "Archive",
                "extra": extra,
            }
        )
    artifacts_path.write_text(json.dumps(artifacts, indent=2) + "\n", encoding="utf-8")
    print(f"appended {len(built)} txz archives to {artifacts_path}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except subprocess.CalledProcessError as exc:
        sys.exit(f"tar failed: {exc}")
