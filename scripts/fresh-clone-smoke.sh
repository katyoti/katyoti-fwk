#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# The fresh-clone smoke: the programs of this checkout start on a machine with
# no KaTyoti installation.
#
#   1. ktw-hub, ktw-agents and katyoti are built from the checkout into a
#      temporary folder (go build);
#   2. the katyoti CLI answers: `katyoti --help` and `katyoti version` exit 0;
#   3. a ktw-agents command on a universe of its own (--hub stop, the default)
#      starts the hub built here - its lockfile appears -, the hub serves it
#      through the ktw-agents sidecar the universe declares, and the hub ends
#      after the command: no lockfile left, no process.
#
# Everything lives in a temporary folder: its own KaTyoti root, data, cache and
# universe - never ~/.config/katyoti, never a folder of the checkout. The smoke
# ends only the hub it saw start. No jq: the lockfile's pid is read with sed.
# KEEP=1 keeps the folder; SMOKE_HUB_BINARY=<path> runs the command with that
# program as the hub instead of the one built (a wrong one makes the smoke fail);
# SMOKE_END_SECONDS bounds the wait for the hub's end (default 30).
#
#   bash scripts/fresh-clone-smoke.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
EXT=
WINDOWS=
case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) EXT=.exe WINDOWS=1 ;; esac

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "ok   $*"; }
winpath() { if command -v cygpath >/dev/null; then cygpath -w "$1"; else echo "$1"; fi; }

W="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/fresh-clone.XXXXXX")"
W="$(cd "$W" && pwd)"
HUBPID=
cleanup() {
  [ -z "$HUBPID" ] || end_pid "$HUBPID"
  if [ -n "${KEEP:-}" ]; then echo "kept: $W"; return; fi
  rm -rf "$W" 2>/dev/null || true
}
trap cleanup EXIT
for d in "$HOME/.config/katyoti" "$REPO"; do
  case "$W/" in "$d"/*) fail "the temporary folder $W sits in $d" ;; esac
done

# alive <pid>: the hub process runs (Windows pids, as the lockfile names them).
alive() {
  if [ -n "$WINDOWS" ]; then
    tasklist //FI "PID eq $1" //NH 2>/dev/null | grep -qi "ktw-hub"
  else
    kill -0 "$1" 2>/dev/null
  fi
}
end_pid() {
  alive "$1" || return 0
  if [ -n "$WINDOWS" ]; then taskkill //PID "$1" //T //F >/dev/null 2>&1 || true; else kill "$1" 2>/dev/null || true; fi
}

# ── 1. build from the checkout ──────────────────────────────────────────────
mkdir -p "$W/bin" "$W/root" "$W/data" "$W/cache" "$W/u/.katyoti/config"
for p in ktw-hub ktw-agents katyoti; do
  (cd "$REPO" && go build -o "$W/bin/$p$EXT" "./app/$p") || fail "go build ./app/$p"
done
ok "built ktw-hub, ktw-agents and katyoti from $REPO"

HUB="${SMOKE_HUB_BINARY:-$W/bin/ktw-hub$EXT}"
cat >"$W/u/.katyoti/config/Univers.yaml" <<EOF
version: 1
name: fresh-clone
sidecars:
  - name: ktw-agents
    transport: nats
    binary: '$(winpath "$W/bin/ktw-agents$EXT")'
    args: [sc, ktsp, nats]
    ready_timeout: 1m0s
EOF
printf '{}\n' >"$W/u/.katyoti/config/Settings.yaml"
KTW_KATYOTI_ROOT="$(winpath "$W/root")"
KTW_DATA="$(winpath "$W/data")"
KTW_CACHE="$(winpath "$W/cache")"
KTW_HUB_BINARY="$(winpath "$HUB")"
export KTW_KATYOTI_ROOT KTW_DATA KTW_CACHE KTW_HUB_BINARY
export KTW_NO_INSTALLATION=1 KTW_ATOMICIO_NO_SYNC=1 KTW_HUB=stop
unset KTW_INSTALLATION KTW_INSTANCE KTW_NATS_URL KTW_DOTENV_KEYS KTW_WORKSPACE KTW_UNIVERSE KTW_HOME KTW_WORKSPACE_DIR KTW_HUB_OWNER CLAUDECODE 2>/dev/null || true
LOCK="$W/data/hub/hub.lock"

# ── 2. the katyoti CLI answers ──────────────────────────────────────────────
(cd "$W/u" && "$W/bin/katyoti$EXT" --help) >"$W/katyoti-help.log" 2>&1 || fail "katyoti --help: $(tail -5 "$W/katyoti-help.log")"
grep -q "Usage:" "$W/katyoti-help.log" || fail "katyoti --help prints no usage: $(tail -5 "$W/katyoti-help.log")"
(cd "$W/u" && "$W/bin/katyoti$EXT" version) >"$W/katyoti-version.log" 2>&1 || fail "katyoti version: $(tail -5 "$W/katyoti-version.log")"
ok "katyoti --help and katyoti version exit 0 ($(tr -d '\r' <"$W/katyoti-version.log" | head -1))"

# ── 3. a command starts the hub, which ends after it ────────────────────────
lock_pid() { [ -f "$LOCK" ] && sed -n 's/.*"pid"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$LOCK" 2>/dev/null | head -1 || true; }
# await_lock <seconds> <command pid>: the pid the lockfile names, once it
# appears, while the command still runs or has just ended.
await_lock() {
  local i pid
  for ((i = 0; i < $1 * 10; i++)); do
    pid="$(lock_pid)"
    [ -n "$pid" ] && { echo "$pid"; return 0; }
    kill -0 "$2" 2>/dev/null || { pid="$(lock_pid)"; [ -n "$pid" ] && { echo "$pid"; return 0; }; return 1; }
    sleep 0.1
  done
  return 1
}
# gone <pid> <seconds>: the hub has ended within the time.
gone() {
  local end=$((SECONDS + $2))
  while [ "$SECONDS" -lt "$end" ]; do
    alive "$1" || return 0
    sleep 0.2
  done
  ! alive "$1"
}

(cd "$W/u" && "$W/bin/ktw-agents$EXT" --home "$(winpath "$W/u")" todos modes --out json) >"$W/command.log" 2>&1 &
cmd=$!
HUBPID="$(await_lock 90 "$cmd")" || {
  wait "$cmd" || true
  fail "ktw-agents todos modes started no hub (hub binary $HUB): $(tail -10 "$W/command.log")"
}
code=0
wait "$cmd" || code=$?
[ "$code" -eq 0 ] || fail "ktw-agents todos modes exited $code: $(tail -10 "$W/command.log")"
END_WAIT="${SMOKE_END_SECONDS:-30}"
gone "$HUBPID" "$END_WAIT" || fail "the hub (pid $HUBPID) outlived its --hub stop command by $END_WAIT s"
for ((i = 0; i < 50; i++)); do [ -f "$LOCK" ] || break; sleep 0.2; done
[ ! -f "$LOCK" ] || fail "the hub ended and left its lockfile: $(cat "$LOCK")"
ok "ktw-agents todos modes started the hub (pid $HUBPID), which ended after it, no lockfile left"
HUBPID=
echo "PASS fresh-clone-smoke"
