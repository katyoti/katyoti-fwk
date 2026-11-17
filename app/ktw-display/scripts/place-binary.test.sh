#!/usr/bin/env sh
# SPDX-License-Identifier: Apache-2.0
# place-binary.test.sh — place-binary.sh puts a new exe in place while the
# old one RUNS (Windows), leaves the running process alone, and reaps the
# leftovers of an earlier build. Prints PASS and exits 0, or says what failed.
#
# Windows only (a running exe is locked there); elsewhere it says so and passes.

set -eu

case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) ;;
  *) echo "SKIP: a running exe is only locked on Windows"; echo PASS; exit 0 ;;
esac

HERE="$(cd "$(dirname "$0")" && pwd)"
T="$(mktemp -d)"
trap 'powershell -NoProfile -Command "Get-Process target -ErrorAction SilentlyContinue | Where-Object { \$_.Path -like \"*$(basename "$T")*\" } | Stop-Process -Force" >/dev/null 2>&1; rm -rf "$T" 2>/dev/null || true' EXIT

sys="$(cygpath -u "${SYSTEMROOT:-C:/Windows}")/System32"
cp "$sys/PING.EXE" "$T/target.exe"          # the running copy
cp "$sys/HOSTNAME.EXE" "$T/target.new.exe"  # the freshly built one
: > "$T/target.exe.old-1"                   # a leftover whose process has exited

powershell -NoProfile -Command "Start-Process -FilePath '$(cygpath -w "$T")\\target.exe' -ArgumentList '-n','30','127.0.0.1' -WindowStyle Hidden"
powershell -NoProfile -Command "Start-Sleep -Milliseconds 500"

sh "$HERE/place-binary.sh" "$T/target.new.exe" "$T/target.exe"

out="$("$T/target.exe" 2>/dev/null | head -1)" || true
[ "$out" = "$(hostname)" ] || { echo "FAIL: target.exe is not the new exe (ran: $out)"; exit 1; }
[ ! -e "$T/target.new.exe" ] || { echo "FAIL: the new exe was copied, not moved"; exit 1; }
[ ! -e "$T/target.exe.old-1" ] || { echo "FAIL: the exited leftover was not reaped"; exit 1; }
running="$(powershell -NoProfile -Command "@(Get-Process target -ErrorAction SilentlyContinue | Where-Object { \$_.Path -like '*$(basename "$T")*' }).Count")"
[ "$(echo "$running" | tr -d '\r ')" -ge 1 ] || { echo "FAIL: the running process did not survive"; exit 1; }
echo PASS
