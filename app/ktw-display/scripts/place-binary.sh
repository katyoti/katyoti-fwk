#!/usr/bin/env sh
# SPDX-License-Identifier: Apache-2.0
# place-binary.sh <new> <target> — put a freshly compiled exe in place.
#
# Windows refuses to overwrite a RUNNING exe (bun's own move then fails with
# EPERM whenever a universe serves display widgets), but it lets one be
# renamed aside: the running process keeps its image, the new exe takes the
# name, the next start runs it. The same trick as scripts/install.sh.
# Leftovers of earlier builds (<target>.old-*) are removed once their process
# has exited — deleting one still running fails and it is kept for later.

set -eu

new="$1"
target="$2"

[ -f "$new" ] || { echo "place-binary: $new was not built" >&2; exit 1; }

for old in "$target".old-*; do
  [ -e "$old" ] && rm -f "$old" 2>/dev/null || true
done

if mv -f "$new" "$target" 2>/dev/null; then
  exit 0
fi

aside="$target.old-$$"
if ! mv -f "$target" "$aside"; then
  echo "place-binary: cannot replace $target: it is in use and cannot be renamed aside" >&2
  exit 1
fi
mv -f "$new" "$target"
echo "place-binary: $target was running; the running copy moved to $(basename "$aside") (removed by a later build once it exits)"
