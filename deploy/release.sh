#!/usr/bin/env bash
# Writes a release of the current commit to stdout as a tarball: the
# linux/amd64 binary with the commit stamped in, the landing page and agent
# files from deploy/site, and a VERSION file. CI pipes it to the deploy user;
# a person can too, see deploy/README.md.
#
# amd64 because the shop VM is an E2.1.Micro. Change it with the shape.
set -euo pipefail
cd "$(dirname "$0")/.."

# VERSION names a commit, so the tree has to be that commit.
if [ -n "$(git status --porcelain)" ]; then
  echo "release: working tree is dirty; commit first" >&2
  exit 1
fi

version=$(git rev-parse HEAD)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w -X main.version=$version" -o "$work/ssh-bookshop" .
cp -R deploy/site "$work/site"
printf '%s\n' "$version" >"$work/VERSION"

tar -C "$work" -cf - ssh-bookshop site VERSION
