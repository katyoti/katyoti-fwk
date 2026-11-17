#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Reads the output of a `go test` run and says where its time went
# in the test lanes: the packages that took more than LIMIT seconds,
# slowest first, then how many packages ran, how many answered from the cache
# and the seconds the run's packages summed. A cached package ("(cached)") took
# no time. Given the output of a previous run, it also names the packages that
# got slower (by half again and more than 10 s) and the change of the sum.
# A failed run ends on the tests that failed (« --- FAIL: ») and their
# packages, so their names are not lost in the logs above them.
#
# Usage: go test ... 2>&1 | bash scripts/test-report.sh [LIMIT [PREVIOUS]]   (default 30)
#        bash scripts/test-report.sh LIMIT PREVIOUS < go-test-output.txt
set -euo pipefail

limit="${1:-30}"
previous="${2:-}"
[ -z "$previous" ] || [ -f "$previous" ] || { echo "test-report: no previous run $previous" >&2; previous=""; }
awk -v limit="$limit" -v prevfile="$previous" '
function secs(s) { sub(/s$/, "", s); return s + 0 }
BEGIN {
    if (prevfile != "") {
        while ((getline line < prevfile) > 0) {
            nf = split(line, f, /[ \t]+/)
            if ((f[1] == "ok" || f[1] == "FAIL") && nf >= 3 && f[3] ~ /^[0-9.]+s$/) {
                prev[f[2]] = secs(f[3]); prevtotal += prev[f[2]]; hasprev = 1
            }
        }
        close(prevfile)
    }
}
/^[[:space:]]*--- FAIL: / { t = $0; sub(/^[[:space:]]*--- FAIL: /, "", t); failed[++nfail] = t }
$1 == "FAIL" && NF >= 2 && $2 != "" { failedpkg[++npkg] = $2 }
($1 == "ok" || $1 == "FAIL") && NF >= 3 {
    if ($3 == "(cached)") { cached++; ran++; next }
    if ($3 ~ /^[0-9.]+s$/) {
        t = secs($3)
        ran++; total += t
        if (t > limit) slow[++n] = sprintf("%9.1fs  %s", t, $2)
        if (($2 in prev) && t > prev[$2] * 1.5 && t - prev[$2] > 10)
            slower[++m] = sprintf("%9.1fs  %s (was %.1fs)", t, $2, prev[$2])
    }
}
function sortdesc(a, k,   i, j, v) {
    for (i = 2; i <= k; i++) {
        v = a[i]; j = i - 1
        while (j > 0 && a[j] + 0 < v + 0) { a[j + 1] = a[j]; j-- }
        a[j + 1] = v
    }
}
END {
    if (n > 0) {
        sortdesc(slow, n)
        printf "packages over %ss:\n", limit
        for (i = 1; i <= n; i++) print slow[i]
    }
    if (m > 0) {
        sortdesc(slower, m)
        print "slower than the previous run:"
        for (i = 1; i <= m; i++) print slower[i]
    }
    printf "%d packages, %d from the cache, %.1fs summed", ran, cached, total
    if (hasprev) printf " (previous run %.1fs)", prevtotal
    printf "\n"
    if (nfail > 0) {
        print "failed tests:"
        for (i = 1; i <= nfail; i++) print "  " failed[i]
    }
    if (npkg > 0) {
        print "failed packages:"
        for (i = 1; i <= npkg; i++) print "  " failedpkg[i]
    }
}'
