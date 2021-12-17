# Application Builder

[https://github.com/golift/application-builder](https://github.com/golift/application-builder)

## Docker Build Hooks

The files in this folder are used by Docker Cloud to automate image builds.
Do not edit these files.

If you want to build, maintain and push multi-architecture Docker images, you may
follow the example provided here. All of the hooks are generic, and will work with
any build.

`BUILDS` must be set to the builds you're trying to perform. This repo is [currently set to](../../buildinfo.sh): `linux:armhf:arm linux:arm64:arm64 linux:amd64:amd64 linux:i386:386`
  -   The format is `os:name:arch`.
  -   `os` and `name` are passed into the Dockerfile.
  -   `os` and `arch` are passed into `docker manifest annotate`.

Keep the build simple; see screenshot. This only supports one build tag, but it creates many more.

## Manual Builds

DockerHub turned off Automated builds unless you pay $300/year, so I had to figure out how to run the build scripts locally.
I [added a few lines to settings.sh](https://github.com/davidnewhall/unpackerr/commit/2fcd790a4d7544c1cb40525f06c1e922dd15f6af#diff-9766226a804c653af0e5003a333bf8c2378874ec62d11e64623e1cfb041057cf)
to make sure variables docker normally sets are there.
Then run these commands to build and push a new release. Run them from the `init/docker` directory.

```
SOURCE_BRANCH=v0.9.10 bash hooks/build
SOURCE_BRANCH=v0.9.10 bash hooks/push
```

You may omit `SOURCE_BRNCH` to pick up the current branch instead of a release.

If you screw up somehow and need to fix a manifest, delete it first. Example:

```
docker manifest rm golift/unpackerr:0.9.8
docker manifest rm golift/unpackerr:0.9
docker manifest rm golift/unpackerr:0
docker manifest rm golift/unpackerr:latest
docker manifest rm golift/unpackerr:stable
# if you messed up a branch:
docker manifest rm golift/unpackerr:<branch_name>
```
