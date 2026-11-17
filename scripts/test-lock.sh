#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Runs a command alone: one at a time per lock name, for the
# whole repository and its worktrees. The lock is a file in git's common dir
# (<name>.lock) naming the holder's pid and start; a second run while the
# holder lives is refused, naming it; the lock of a holder that died is taken
# over. Used by scripts/test-full.sh so that two full test runs never overlap.
#
# Usage: bash scripts/test-lock.sh <name> <command> [args...]
# Exit: the command's; 4 when another run holds the lock.
set -uo pipefail

[ $# -ge 2 ] || { echo "usage: $0 <name> <command> [args...]" >&2; exit 2; }
name="$1"; shift
common="$(git rev-parse --git-common-dir 2>/dev/null)" || { echo "test-lock: not in a git repository" >&2; exit 2; }
lock="$common/$name.lock"

# take: the lock is a file that appears whole or not at all - the holder
# (pid, start) is written in a file of this run's own, then hard-linked to the
# lock's name, which fails when the name exists: of two runs, one wins.
take() {
    local mine="$lock.$$.new" r
    printf '%s %s
' "$$" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$mine" || return 1
    ln -T "$mine" "$lock" 2>/dev/null
    r=$?
    rm -f "$mine"
    return $r
}

# A lock that names no holder was left by a run stopped while taking it (an
# older take wrote it in two steps): live while young, taken over once older.
grace="${KTW_TEST_LOCK_GRACE:-60}"

if ! take; then
    pid="" since=""
    [ -f "$lock" ] && read -r pid since <"$lock" 2>/dev/null
    if [ -n "$pid" ]; then
        alive=0; kill -0 "$pid" 2>/dev/null && alive=1
    else
        age=$(( $(date +%s) - $(stat -c %Y "$lock" 2>/dev/null || date +%s) ))
        alive=0; [ "$age" -lt "$grace" ] && alive=1
        since="${age}s ago, no holder named"
    fi
    if [ "$alive" -eq 1 ]; then
        echo "test-lock: $name is running already (pid ${pid:-?}, since $since) - one at a time; wait for it to end" >&2
        exit 4
    fi
    # Move the dead lock aside, then check it is the one seen dead: a lock a
    # run took over meanwhile is put back.
    aside="$lock.$$.dead"
    mv -T "$lock" "$aside" 2>/dev/null || { echo "test-lock: $name was taken meanwhile" >&2; exit 4; }
    now_pid=""
    [ -f "$aside" ] && read -r now_pid _ <"$aside" 2>/dev/null
    if [ "$now_pid" != "$pid" ]; then
        ln -T "$aside" "$lock" 2>/dev/null
        rm -rf "$aside"
        echo "test-lock: $name was taken meanwhile" >&2
        exit 4
    fi
    rm -rf "$aside"
    echo "test-lock: $name's last holder (pid ${pid:-?}) is gone - taking its lock" >&2
    take || { echo "test-lock: $name was taken meanwhile" >&2; exit 4; }
fi
# release: only this run's own lock
trap 'read -r p _ <"$lock" 2>/dev/null && [ "$p" = "$$" ] && rm -f "$lock"' EXIT INT TERM
"$@"
