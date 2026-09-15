#!/bin/sh

# Build the SPA into dist/ so the Go binary can embed it.
# go generate ./frontend (and go generate ./...) runs this from frontend.go.

set -e
cd "$(dirname "$0")"

# Reuse node_modules when Docker already ran npm ci for this lockfile.
if [ ! -d node_modules ]; then
	npm ci
fi
npm run build

# Vite empties dist/; restore the git-tracked stub used by go:embed in unit tests.
printf '%s\n*\n' '# Placeholder so //go:embed all:dist compiles before npm run build.' > dist/.gitignore
