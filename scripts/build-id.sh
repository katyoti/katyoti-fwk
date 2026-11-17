#!/usr/bin/env sh
# SPDX-License-Identifier: Apache-2.0
# Print the build id a cut stamps on every platform folder (.katyoti-build):
# "<VERSION> <short rev>", with "-dirty" when the tree has uncommitted changes.
#
# Usage:  BUILD_ID="$(VERSION="$VERSION" sh scripts/build-id.sh)"
#
# A cut computes it ONCE, before any build, and hands it to every platform
# (KATYOTI_BUILD_ID, read by build-cross.sh): a build changes tracked files
# (the GUI's wails build regenerates its bindings), so an id read again after
# it would mark every platform built later -dirty, and the publication refuses
# a tree whose platforms disagree.
set -eu
VERSION="${VERSION:-dev}"
id="${VERSION} $(git rev-parse --short HEAD 2>/dev/null || echo nogit)"
if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
    id="${id}-dirty"
fi
printf '%s\n' "$id"
