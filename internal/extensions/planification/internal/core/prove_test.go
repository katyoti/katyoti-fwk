// SPDX-License-Identifier: Apache-2.0

package core

import "testing"

// ONE GO TEST, HOWEVER WORDED, IS ONE CHECK: -short
// -count=1, spaces, a trailing slash or quotes around the pattern change how a
// test runs, not which; another pattern or package is another check.
func TestAProbeKeyIgnoresHowAGoTestIsWorded(t *testing.T) {
	base := probeKey("go test ./billing/ -run TestPrint")
	for _, p := range []string{
		"go test ./billing/ -run TestPrint -count=1",
		"go  test -short ./billing -run 'TestPrint'",
		"go test -count 1 -run=TestPrint ./billing/",
	} {
		if k := probeKey(p); k != base {
			t.Errorf("probeKey(%q) = %q; want %q", p, k, base)
		}
	}
	for _, p := range []string{"go test ./billing/ -run TestMail", "go test ./mail/ -run TestPrint", "bash scripts/x.sh"} {
		if probeKey(p) == base {
			t.Errorf("probeKey(%q) equals the TestPrint key; want another check", p)
		}
	}
	if a, b := probeKey("bash  scripts/x.sh  --fast"), probeKey("bash scripts/x.sh --fast"); a != b {
		t.Errorf("a command's spaces count: %q vs %q", a, b)
	}
}
