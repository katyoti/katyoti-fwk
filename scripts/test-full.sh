#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# The full test lane: the final gate of an increment, or
# the night - one at a time (scripts/test-lock.sh: a second full run is refused
# while one runs). It tests a COMMIT, never the working tree: several sessions
# edit the shared tree at once, and a gate that read it would measure their
# half-written files. The commit (HEAD unless --commit names another) is
# checked out clean into a temporary folder (scripts/clean-checkout.sh: no
# edited, untracked or ignored file), its bin/ is built there (make build) for
# the tests that start programs, with the GUI frontend ktw-gui embeds, and
# the folder is removed after, whatever the outcome. Go's module and build
# caches are the user's, shared, and the folder is the same at each run, so a
# run recompiles only what changed since the last.
#
# In the snapshot: every Go test, the heavy ones of internal/test/long
# included, run again (-count=1) two packages at a time, under the guard that
# fails the run when a test or the build wrote the person's installations
# (scripts/installation-guard.sh), without fsync and with a 30-minute
# -timeout per package (scripts/test-env.sh);
# then the frontend suites, one at a time.
# The run's package lines and its report are kept in the working tree's
# bin/test-runs/full-<UTC stamp>.txt; the report names the commit tested, the
# slowest packages and those slower than the previous run
# (scripts/test-report.sh). Everyday runs use the quick lane
# (scripts/test-affected.sh, scripts/test-quick.sh).
#
# Usage: bash scripts/test-full.sh [--commit <ref>] [--packages "<pkg> ..."] [--no-build] [--no-frontend]
#        make test-full [COMMIT=<ref>]
#   --commit <ref>   the commit to test (default HEAD)
#   --packages ...   the Go packages to test (default ./...) - a partial run, to prove the gate
#                    itself, kept as bin/test-runs/partial-<stamp>.txt
#   --no-build       no bin/ in the snapshot (the tests that start programs skip)
#   --no-frontend    no bun suites
# Exit: 0 = every suite passed; 1 = one failed; 2 = usage or no snapshot; 4 = a full run is running.
set -uo pipefail

here="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd -- "$here/.." && pwd)"

cd "$root" || exit 2
if [ "${KTW_TEST_FULL_LOCKED:-}" != 1 ]; then
    # the lock is this repository's (test-lock.sh reads git's common dir from here)
    KTW_TEST_FULL_LOCKED=1 exec bash "$here/test-lock.sh" test-full bash "${BASH_SOURCE[0]}" "$@"
fi

commit=HEAD packages="./..." build=1 frontend=1
while [ $# -gt 0 ]; do
    case "$1" in
    --commit) commit="${2:?--commit needs a ref}"; shift 2 ;;
    --packages) packages="${2:?--packages needs packages}"; shift 2 ;;
    --no-build) build=0; shift ;;
    --no-frontend) frontend=0; shift ;;
    *) echo "test-full: unknown argument $1" >&2; exit 2 ;;
    esac
done

runs="$root/bin/test-runs"
mkdir -p "$runs"
previous="$(ls -1 "$runs"/full-*.txt 2>/dev/null | sort | tail -1)"
# a run of some packages only is kept apart: never the previous full run
kind=full; [ "$packages" = "./..." ] || kind=partial
kept="$runs/$kind-$(date -u +%Y%m%dT%H%M%SZ).txt"
out="$(mktemp)"
# The snapshot's folder is the same from one run to the next (the lock makes it
# this run's alone): Go's build cache keys a compiled package by its folder too,
# so a fixed folder lets a run reuse what the previous one compiled. A folder a
# killed run left is removed first.
work="${TMPDIR:-/tmp}/ktw-test-full-$(printf '%s' "$root" | cksum | cut -d' ' -f1)"
snap="$work/src"

# cleanup removes the snapshot whatever the outcome; a program a test left
# running can hold a file a moment on Windows, so it is tried again.
cleanup() {
    rm -f "$out"
    local i
    for i in 1 2 3 4 5; do
        rm -rf "$work" 2>/dev/null
        [ -e "$work" ] || return 0
        sleep 2
    done
    echo "test-full: could not remove the snapshot $work" >&2
}
trap cleanup EXIT
trap 'exit 130' INT TERM

rm -rf "$work" 2>/dev/null
[ ! -e "$work" ] || { echo "test-full: the snapshot folder $work cannot be removed" >&2; exit 2; }
mkdir -p "$work" || exit 2
rev="$(bash "$here/clean-checkout.sh" "$root" "$snap" "$commit")" || { echo "test-full: no snapshot of $commit" >&2; exit 2; }
subject="$(git -C "$root" log -1 --format=%s "$rev")"
echo "test-full: testing commit $rev ($commit: $subject)"
echo "test-full: snapshot $snap"
cd "$snap" || exit 2
# shellcheck source=scripts/test-env.sh
. "$here/test-env.sh" || exit 2 # the tests skip fsync
code=0
start=$(date +%s)

if [ "$frontend" = 1 ] || [ "$build" = 1 ]; then
    for suite in app/ktw-display app/ktw-gui/frontend app/ktw-ktsp-sdk; do
        (cd "$snap/$suite" && bun install --silent) || { echo "test-full: bun install failed in $suite" >&2; code=1; }
    done
fi
# ktw-gui embeds its built frontend (//go:embed all:frontend/dist), which git
# does not keep: without it app/ktw-gui and every `go list` of the tree fail.
# It is built here as Wails would; a frontend that does not build fails the run
# and leaves a placeholder, so the Go packages are still tested.
gui="$snap/app/ktw-gui/frontend"
if [ -d "$gui" ]; then
    echo "== GUI frontend build (app/ktw-gui/frontend/dist)"
    if ! (cd "$gui" && bun install --silent && bun run build) >"$work/gui-build.txt" 2>&1; then
        tail -40 "$work/gui-build.txt"
        echo "test-full: the GUI frontend does not build" >&2
        code=1
    fi
    if [ -z "$(ls -A "$gui/dist" 2>/dev/null)" ]; then
        mkdir -p "$gui/dist" && : >"$gui/dist/.embed-placeholder"
    fi
fi
if [ "$build" = 1 ]; then
    echo "== make build (snapshot bin/)"
    bash "$here/installation-guard.sh" make build >"$work/build.txt" 2>&1 || { tail -40 "$work/build.txt"; echo "test-full: make build failed" >&2; code=1; }
fi

# shellcheck disable=SC2086 # packages is a list
bash "$here/installation-guard.sh" go test -p 2 -count=1 -timeout "$KTW_TEST_TIMEOUT" $packages 2>&1 | tee "$out"
[ "${PIPESTATUS[0]}" -eq 0 ] || code=1

if [ "$frontend" = 1 ]; then
    for suite in app/ktw-display app/ktw-gui/frontend app/ktw-ktsp-sdk; do
        echo "== bun test: $suite"
        (cd "$snap/$suite" && bun run test) || code=1
    done
fi

{
    echo "# commit $rev ($commit: $subject)"
    grep -E '^(ok|FAIL|---)[[:space:]]' "$out" | grep -v '^---'
    echo
    echo "# report ($(( $(date +%s) - start ))s wall, exit $code, commit $rev)"
    bash "$here/test-report.sh" "${SLOW_SECONDS:-30}" "$previous" <"$out"
} >"$kept"
echo
sed -n '/^# report/,$p' "$kept"
echo "kept: $kept"
exit "$code"
