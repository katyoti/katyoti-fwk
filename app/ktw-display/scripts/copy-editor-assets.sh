#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copy vendor/drawio/editor/ into bin/ktw-display-assets/drawio/editor/
# so the compiled ktw-display.exe finds it as a sibling of itself.
#
# Run after `bun build --compile` produces bin/ktw-display.exe — see
# Makefile target `ktw-display`.

set -euo pipefail

# Repo-rooted paths regardless of invocation dir.
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../../.." && pwd)"
SRC="$REPO/app/ktw-display/vendor/drawio/editor"
DST="$REPO/bin/ktw-display-assets/drawio/editor"

if [ ! -d "$SRC" ]; then
    echo "[ktw-display] WARN: $SRC missing; skipping editor copy" >&2
    echo "[ktw-display] run \`./app/ktw-display/vendor/drawio/setup-editor.sh\` to populate it" >&2
    exit 0
fi

mkdir -p "$(dirname "$DST")"
rm -rf "$DST"
cp -r "$SRC" "$DST"

SIZE=$(du -sh "$DST" | cut -f1)
echo "[ktw-display] copied editor assets ($SIZE) → $DST"
