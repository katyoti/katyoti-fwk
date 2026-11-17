#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Regenerate ktw-gui's typed KTSP clients from a live sidecar descriptor.
#
# The GUI calls services through ONE generic Wails dispatcher (CallKTSPMethod);
# the TYPING is generated from `sc ktsp describe --out json` — the same descriptor
# the wire is built from — so the client cannot drift from the ServiceDef.
# Publishing a service method needs no Go change in ktw-gui: publish, run this,
# call it typed.
#
# Lives in a script rather than the Makefile because gomake splits recipe args on
# whitespace and cannot hold the temp-file plumbing.
set -euo pipefail

cd "$(dirname -- "${BASH_SOURCE[0]}")/.."

SIDECAR="${SIDECAR:-./bin/ktw-univers.exe}"
OUT="${OUT:-app/ktw-gui/frontend/src/ktsp}"
SLOTS="${SLOTS:-marketplace orchestrator profile}"

[ -x "$SIDECAR" ] || { echo "ERROR: $SIDECAR not built — run 'make ktw-univers'" >&2; exit 1; }
[ -x ./bin/ktw-utils.exe ] || { echo "ERROR: ./bin/ktw-utils.exe not built — run 'make ktw-utils'" >&2; exit 1; }

TMP="$(mktemp -t ktsp-descriptor.XXXXXX.json)"
trap 'rm -f "$TMP"' EXIT

echo "==> Describing $SIDECAR"
"$SIDECAR" sc ktsp describe --out json > "$TMP"

ARGS=()
for s in $SLOTS; do ARGS+=(--slot-type "$s"); done

echo "==> Generating typed clients into $OUT/"
./bin/ktw-utils.exe gen-ktsp-client "$TMP" --out "$OUT" "${ARGS[@]}"

# The hub's catalog-service (the translation bundle the GUI's `contrib`
# namespace loads): ktw-hub is no sidecar, its descriptor is `ktsp describe`.
HUB="${HUB:-./bin/ktw-hub.exe}"
if [ -x "$HUB" ]; then
  echo "==> Describing $HUB"
  "$HUB" ktsp describe --out json > "$TMP"
  ./bin/ktw-utils.exe gen-ktsp-client "$TMP" --out "$OUT" --slot-type catalog-service
else
  echo "WARN: $HUB not built — catalog-service.ts not regenerated ('make ktw-hub')" >&2
fi

echo "==> Done. Check the result in; do not hand-edit the generated files."
