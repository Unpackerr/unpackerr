# Unpackerr Configuration Generator

This folder contains a [yaml](conf-builder.yml) file that describes the entire Unpackerr configuration.
This description includes all variables, their defaults, their recommendations, comments and documentation.

## Builders

Also included in this folder is a go app that comprises three builders. The app can build an
[example config file](https://github.com/Unpackerr/unpackerr/blob/main/examples/unpackerr.conf.example),
a [compose service](https://github.com/Unpackerr/unpackerr/blob/main/examples/docker-compose.yml),
and a suite of docusaurus-markdown files to create the official
[Configuration Website](https://unpackerr.zip/docs/install/configuration/).

### Config File

The config file is generated every time `go generate` runs. This happens during the build, so the
config file compiled into the application is always up to date. The repo needs to be updated manually
when the definition file changes. Just run `go generate ./...` before committing.

### Compose Service

It's not required or used by the build, but `go generate` also generates the example compose file.
The repo needs to be updated manually when the definition files changes; commit it too.

### Documentation

The [unpackerr.github.io](https://github.com/Unpackerr/unpackerr.github.io) repo contains a
[small script](https://github.com/Unpackerr/unpackerr.github.io/blob/main/generate.sh) that runs
the generator in the `main` branch every time `yarn build` or `yarn start` runs.

In other words, GitHub Actions generates the documentation from the current code when the repo changes.
It also automatically generates while you're developing locally because it's executed from
[package.json](https://github.com/Unpackerr/unpackerr.github.io/blob/main/package.json).

## YAML Requirements

- All params must have a default, even if it's `[]` or `''`.

### Top level Explanation

- `envvar_prefix`

This is the primary prefix the app uses for environment variables.

- `order`

Order describes which order items should appear. This affects all three builders.

- `def_order`

This variable controls the order for defined sections. The second-level key needs
to match a second-level key in `defs`.

- `recommendations`

This specific key is not used or read in by the generator. It's only used for YAML
anchors that get expanded elsewhere.

These are values that may populate a `<select>` with `<option>` values, or suitable
for a multi-select depending on the variable type. Not every parameter needs a
recommendation, but providing them makes it easier for a user to choose a valid option.
You will find these scattered throughout the YAML document.

- `defs`

This section contains _Defined Sections_. Those are values that get duplicated from a
base that is defined in `sections`.

The second-level key needs to match a second-level key in `sections`. When this happens,
the section is duplicated once for each value found in `defs`. The `defs` key contains
overridable parameters for each defined section. Most of the duplicated sections contain
the same data, but small manipulations can be provided with this input.

- `sections`

This is the meat and potatoes of the file. The second-level key is a section name.
The data that follows describes all the parameters for that section.

## Sections

_(write more here about the sections layout)_
