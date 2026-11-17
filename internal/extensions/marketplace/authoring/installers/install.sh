#!/usr/bin/env sh
# SPDX-License-Identifier: Apache-2.0
# katyoti bootstrap (macOS / Linux).
#
#   piped (URL):   u='<channel-url>'; KTW_INFRA_CHANNEL="$u" sh -c "$(curl -fsSL "$u/install.sh")"
#   from a folder: sh '<channel-folder>/install.sh'       (or double-click install-on-mac.command)
#
# Makes an installation: a folder of its own (~/.local/opt/katyoti/
# <solution>-<version>, or KTW_INSTALLATION_DIR) whose katyoti.yml says where its
# roots are and how it starts -- read by every katyoti program of it, so it runs
# beside any other installation without touching it. Fetches only katyoti,
# verifies it against the channel's signed SHA256SUMS, then lets katyoti pull
# ktw-hub + ktw-gui from the same channel. No profile or PATH is written.
set -eu

# The channel: the variable when given, else the folder this script sits in.
# A piped script has no file, so $0 is the shell's name and there is no folder.
channel="${KTW_INFRA_CHANNEL:-}"
if [ -z "$channel" ] && [ -f "$0" ]; then
    channel="$(cd "$(dirname "$0")" && pwd)"
fi
if [ -z "$channel" ]; then
    echo "set KTW_INFRA_CHANNEL to the infra channel URL or folder (a piped script cannot know where it came from)" >&2
    exit 1
fi
channel="${channel%/}"

fetch() {
    case "$channel" in
    http://* | https://*) curl -fsSL "$channel/$1" ;;
    *) cat "$channel/$1" ;;
    esac
}

version="$(fetch channel.yaml | sed -n 's/^[[:space:]]*version[[:space:]]*:[[:space:]]*//p' | head -1)"
[ -n "$version" ] || {
    echo "channel.yaml at $channel declares no version" >&2
    exit 1
}

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in linux) os=linux ;; darwin) os=darwin ;; *)
    echo "unsupported OS: $os" >&2
    exit 1
    ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*)
    echo "unsupported arch: $(uname -m)" >&2
    exit 1
    ;;
esac
platform="${os}-${arch}"
echo "katyoti ${version} (${platform}) from ${channel}"

# The installation: named after the solution the channel opens on (its
# `solution:`, `demo:` in an older channel) and the version, so another version
# or solution is another folder. Installing the same one again repairs it.
# Nothing of it is passed to setup: setup reads the channel itself.
pointer="$(fetch channel.yaml)"
solution="$(printf '%s\n' "$pointer" | sed -n 's/^[[:space:]]*solution[[:space:]]*:[[:space:]]*//p' | head -1)"
[ -n "$solution" ] || solution="$(printf '%s\n' "$pointer" | sed -n 's/^[[:space:]]*demo[[:space:]]*:[[:space:]]*//p' | head -1)"
if [ -n "$solution" ]; then
    name="${solution%@*} ${solution#*@}"
else
    name="katyoti ${version}"
fi
slug="$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9._-][^a-z0-9._-]*/-/g; s/^-//; s/-$//')"
dir="${KTW_INSTALLATION_DIR:-${HOME}/.local/opt/katyoti/${slug}}"
root="${dir}/root"
mkdir -p "$root"
if [ ! -f "${dir}/katyoti.yml" ]; then
    {
        echo "# A katyoti installation: every katyoti program of it reads this file first."
        echo "name: ${name}"
        [ -z "$solution" ] || echo "solution: ${solution}"
        printf 'roots:\n  config: root\n  data: data\n  cache: cache\n'
    } >"${dir}/katyoti.yml"
fi
# The channel, recorded in the installation (setup pins its key on first use).
[ -f "${root}/infra-channel.yaml" ] || echo "root: ${channel}" >"${root}/infra-channel.yaml"

dest="${root}/control-plane"
mkdir -p "$dest"
fetch "${version}/${platform}/katyoti" >"${dest}/katyoti.new"

# Integrity: the channel's SHA256SUMS is signed, so a swapped binary is caught
# here rather than after it has installed everything else.
want="$(fetch "${version}/${platform}/SHA256SUMS" | grep -E '(^|[[:space:]*])katyoti$' | awk '{print $1}' | head -1)"
if [ -n "$want" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
        got="$(sha256sum "${dest}/katyoti.new" | awk '{print $1}')"
    else
        got="$(shasum -a 256 "${dest}/katyoti.new" | awk '{print $1}')"
    fi
    if [ "$got" != "$want" ]; then
        rm -f "${dest}/katyoti.new"
        echo "katyoti checksum mismatch (want $want, got $got)" >&2
        exit 1
    fi
fi

chmod +x "${dest}/katyoti.new"
mv -f "${dest}/katyoti.new" "${dest}/katyoti"
echo "installed ${dest}/katyoti (installation ${dir})"

# `setup` finishes the job: the rest of the control plane, remembering this
# channel, subscribing the marketplace beside it, and opening the GUI. One
# instruction in, a running app out -- no follow-up homework. It reads the
# installation's katyoti.yml above it; the channel is the one recorded above.
# KTW_SETUP_ARGS passes options to setup (`--no-gui` for a headless check).
# shellcheck disable=SC2086
"${dest}/katyoti" setup ${KTW_SETUP_ARGS:-}
echo "open it again with ${dest}/katyoti"
