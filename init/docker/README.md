## Manual Builds

DockerHub turned off Automated builds unless you pay $300/year, so I had to figure out how to run the build scripts locally. Eventually I'll move this to github actions...

I [added a few lines to settings.sh](https://github.com/davidnewhall/unpackerr/commit/2fcd790a4d7544c1cb40525f06c1e922dd15f6af#diff-9766226a804c653af0e5003a333bf8c2378874ec62d11e64623e1cfb041057cf)
to make sure variables docker normally sets are there.
Then run this command to build and push a new release. Run it from the `init/docker` directory.

```
SOURCE_BRANCH=v0.10.1 bash build
```

You may omit `SOURCE_BRNCH` to pick up the current branch instead of a release.
