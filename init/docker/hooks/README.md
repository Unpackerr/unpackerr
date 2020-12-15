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

![Application Builder Docker Cloud Build Rules](https://raw.githubusercontent.com/wiki/unifi-poller/unifi-poller/images/unifi-poller-build-rules.png "Application Builder Docker Cloud Build Rules")
