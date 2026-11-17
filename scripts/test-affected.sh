#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# The everyday test run of a change: only the packages it
# touches - those of the files changed (the working tree and the untracked
# files against HEAD, or everything since a ref) and the packages importing
# them (tools/test-affected names them) - run in the quick lane
# (scripts/test-quick.sh: -short, -p 2, Go's test cache kept), which ends on
# the slowest packages. An everyday run stays minutes long: an importer that
# took more than SLOW_SKIP_SECONDS (default 120) in the last full run's report
# (bin/test-runs/full-*.txt) is left out and named - its owner tags its heavy
# tests long - unless --all; a package that changed itself always runs.
# With no report at all it stays so: a
# whole-repo suite (internal/test/arch, internal/test/catalog: a package under
# internal/test/ holding tests only) is the full lane's, and a changed package
# with more than FANOUT (default 20) importers brings only those of its owning
# module - the others are named for the full lane; --all runs them. The run
# ends on its wall time.
#
# Usage: bash scripts/test-affected.sh [--since <ref>] [--deep] [--all] [--list] [packages...] [--paths <file>...]
#        make test-affected [SINCE=<ref>] [PATHS="<file> ..."]
#   --since <ref>  the changes since that ref (committed ones included)
#   --paths <f>... the changed files themselves, everything after it (the files
#                  a step changed, in a working tree others change too); none: git's
#   --deep         every package that depends on a changed one, not only its importers
#   --all          nothing left out: slow importers, whole-repo suites, wide fan-outs
#   --list         name the packages, run nothing
#   packages...    added to the run as they are
# Exit: go test's; 0 with nothing to run.
set -uo pipefail

here="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd -- "$here/.." && pwd)"
flags=() list=0 all=0 extra=() paths=()
while [ $# -gt 0 ]; do
    case "$1" in
        --since) [ -z "${2:-}" ] || flags+=(-since "$2"); shift ;;  # empty: none (make)
        --deep) flags+=(-deep) ;;
        --all) all=1 ;;
        --list) list=1 ;;
        --paths) shift; paths=("$@"); break ;;
        -h|--help) sed -n '2,28p' "${BASH_SOURCE[0]}"; exit 0 ;;
        *) extra+=("$1") ;;
    esac
    shift
done

start=$SECONDS
wall() { echo "test-affected: wall time $((SECONDS - start)) s" >&2; }
cd "$root" || exit 2
flags+=(-fanout "${FANOUT:-20}")
if [ "$all" -eq 1 ]; then
    flags+=(-all)
else
    last="$(ls -1 "$root"/bin/test-runs/full-*.txt 2>/dev/null | sort | tail -1)"
    [ -z "$last" ] || flags+=(-slow "$last" -bound "${SLOW_SKIP_SECONDS:-120}")
fi
said="$(mktemp)"
trap 'rm -f "$said"' EXIT
pkgs="$(go run ./tools/test-affected -root . "${flags[@]}" "${paths[@]}" 2>"$said")" || { cat "$said" >&2; exit 2; }
cat "$said" >&2
run=()
while read -r p; do [ -n "$p" ] && run+=("$p"); done <<<"$pkgs"
run+=("${extra[@]}")

if [ "${#run[@]}" -eq 0 ]; then
    echo "test-affected: no Go package changed - nothing to run"
    wall
    exit 0
fi
if [ "$list" -eq 1 ]; then
    printf '%s\n' "${run[@]}"
    wall
    exit 0
fi
bash "$here/test-quick.sh" "${run[@]}"
code=$?
# what was left out, said again where the run ends
grep '^test-affected: skipped' "$said" >&2
wall
exit "$code"
