#!/usr/bin/env sh
# SPDX-License-Identifier: Apache-2.0
# Print the version string a build should stamp, from $VERSION.
#
# Usage:  RESOLVED_VERSION="$(VERSION="$VERSION" sh scripts/resolve-version.sh)"
#
# ONE ANSWER FOR ONE BUILD. This existed inline in build-cross.sh and nowhere
# else, so the infra channel — added later — stamped raw $VERSION instead. A
# single `make release` then published the control plane as "dev" while the
# catalog published its sidecars as "a1b2c3d4-dirty": same command, same tree,
# same moment, two identities. That makes "which infra goes with which
# sidecars" unanswerable, which is exactly the question a compatibility contract
# has to be able to answer.
#
# The rule: an explicit VERSION is used as given (a release stamps a semver from
# a git tag). The default "dev" resolves to the VCS revision, matching what Go
# binaries derive at runtime from debug.ReadBuildInfo — so a dev build's version
# still identifies the commit it came from.
set -eu

VERSION="${VERSION:-dev}"

if [ "$VERSION" != "dev" ]; then
    printf '%s\n' "$VERSION"
    exit 0
fi

REV="$(git rev-parse --short=8 HEAD 2>/dev/null || echo '')"
if [ -z "$REV" ]; then
    # No git (a tarball, a sandbox): "dev" is all we can honestly say.
    printf 'dev\n'
    exit 0
fi
if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
    printf '%s-dirty\n' "$REV"
else
    printf '%s\n' "$REV"
fi
