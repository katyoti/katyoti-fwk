#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# The quick test lane, the everyday run: `go test -short -p 2`
# over the packages given (default ./...), Go's test cache kept - a package
# whose code and inputs did not change answers from it. The heavy tests, listed
# in internal/test/long, skip under -short; the full lane (scripts/test-full.sh,
# make test-full) runs them. Then the report: the packages that still took
# more than SLOW_SECONDS (default 30) - candidates for a long.Test row. The
# tests run without fsync and with a 30-minute -timeout per package
# (scripts/test-env.sh): a slow package is named, not failed.
#
# Usage: bash scripts/test-quick.sh [packages...]      (exit: go test's)
#        make test-quick [PKGS="./internal/extensions/cwm/..."]
set -uo pipefail

here="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/test-env.sh
. "$here/test-env.sh" || exit 2 # the tests skip fsync
[ $# -gt 0 ] || set -- ./...
out="$(mktemp)"
trap 'rm -f "$out"' EXIT

go test -short -p 2 -timeout "$KTW_TEST_TIMEOUT" "$@" 2>&1 | tee "$out"
code=${PIPESTATUS[0]}
echo
bash "$here/test-report.sh" "${SLOW_SECONDS:-30}" <"$out"
exit "$code"
