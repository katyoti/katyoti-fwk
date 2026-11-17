#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Runs a command and fails when it changed the person's installations,
# registries or universes: the files a test, probe, script or
# build must never write - the global root's registries and control plane
# (~/.config/katyoti, or $XDG_CONFIG_HOME/katyoti), the same under %APPDATA%,
# the registry of installations (%APPDATA%/katyoti-instances.yaml), the
# installations' own files (Programs/Katyoti/*/katyoti.yml) and each universe
# of the root (universes/<u>/: its top files, .katyoti and its config), and the
# data roots' universe folders ($XDG_DATA_HOME, %LOCALAPPDATA%, ~/.local/share:
# katyoti/universes/<u> made or removed). Each file is
# stat'ed, never read, before and after; a changed, created or removed one
# fails the run, named.
#
# Usage: bash scripts/installation-guard.sh <command> [args...]
#        (exit: the command's, or 3 when it touched the person's files)
# `make test` runs `go test ./...` under it.
set -uo pipefail

[ $# -gt 0 ] || { echo "usage: $0 <command> [args...]" >&2; exit 2; }

# universe_files lists the files directly in a universe folder, the .lock
# left out: a file created or removed changes the list, one written its stamp;
# a folder made in it (logs) does not count.
universe_files() {
    local f
    [ -d "$1" ] || return 0
    for f in "$1"/* "$1"/.[!.]*; do
        [ -f "$f" ] || continue
        [ "${f##*/}" = .lock ] && continue
        printf '%s\n' "$f"
    done
}

# watched lists the person's files: one path per line.
watched() {
    local roots=() r n
    [ -n "${XDG_CONFIG_HOME:-}" ] && roots+=("$XDG_CONFIG_HOME/katyoti")
    [ -n "${HOME:-}" ] && roots+=("$HOME/.config/katyoti")
    [ -n "${APPDATA:-}" ] && roots+=("$APPDATA/katyoti")
    for r in "${roots[@]}"; do
        for n in marketplaces.yaml infra-channel.yaml sidecars.yaml universes.yaml projects.yaml \
            control-plane/katyoti.exe control-plane/katyoti bin/store; do
            printf '%s\n' "$r/$n"
        done
        # Each universe of the root: its top files, its .katyoti (keys,
        # projects) and its config files. What a running
        # universe writes on its own (logs, cwm, the .lock) is not watched.
        for u in "$r"/universes/*/; do
            [ -d "$u" ] || continue
            u="${u%/}"
            universe_files "$u"
            universe_files "$u/.katyoti"
            universe_files "$u/.katyoti/config"
        done
    done
    # The data roots: each universe's data folder, by its presence (a
    # folder's stamp is its list of entries) - a test or a case that made or
    # removed one wrote the person's data. What a running
    # universe writes inside its own folder is not watched.
    local d
    for d in "${XDG_DATA_HOME:-}" "${LOCALAPPDATA:-}" "${HOME:+$HOME/.local/share}"; do
        [ -n "$d" ] && printf '%s\n' "$d/katyoti/universes"
    done
    [ -n "${APPDATA:-}" ] && printf '%s\n' "$APPDATA/katyoti-instances.yaml"
    [ -n "${HOME:-}" ] && printf '%s\n' "$HOME/.config/katyoti-instances.yaml"
    local programs=""
    if [ -n "${LOCALAPPDATA:-}" ]; then
        programs="$LOCALAPPDATA/Programs/Katyoti"
    elif [ -n "${HOME:-}" ]; then
        programs="$HOME/.local/opt/katyoti"
    fi
    if [ -n "$programs" ]; then
        printf '%s\n' "$programs"
        if [ -d "$programs" ]; then
            for r in "$programs"/*/; do
                [ -d "$r" ] && printf '%s\n' "${r%/}/katyoti.yml"
            done
        fi
    fi
}

# stamp is a file's size and modification time, "absent" when there is none;
# a folder's stamp is the sorted list of its entries.
stamp() {
    if [ -d "$1" ]; then
        ls -1A "$1" 2>/dev/null | tr '\n' '/'
    elif [ -e "$1" ]; then
        stat -c '%s@%Y.%y' "$1" 2>/dev/null
    else
        echo absent
    fi
}

snapshot() {
    local p
    while IFS= read -r p; do
        printf '%s\t%s\n' "$p" "$(stamp "$p")"
    done < <(watched)
}

before="$(snapshot)"
"$@"
code=$?
after="$(snapshot)"

if [ "$before" != "$after" ]; then
    echo "GUARD: the command changed the person's installation files:" >&2
    diff <(printf '%s\n' "$before") <(printf '%s\n' "$after") | grep '^>' | cut -f1 | sed 's/^> /  /' >&2
    exit 3
fi
exit "$code"
