// SPDX-License-Identifier: Apache-2.0

package hubharness

import (
	"os"
	"testing"
)

// MissingBinary reports a test cannot run because a binary is not built.
//
// # Why this is not just t.Skip
//
// The cross-process suite skips when `bin/` is empty, which is right on a
// developer's machine running `go test ./...` — nobody wants a full sidecar
// build to check one unit test.
//
// It is wrong in `make check`, which BUILDS those binaries first. There, a skip
// means something went wrong with the build or the lookup, and reporting it as
// "nothing to check" is how this suite came to assert nothing for an entire
// milestone while reporting ok.
//
// So the caller states what it needs and this decides: skip when nobody claimed
// the binaries exist, fail when KTW_E2E says they do.
func MissingBinary(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv("KTW_E2E") != "" {
		t.Fatalf("KTW_E2E is set, so this suite must run — but "+format+
			"\n(the E2E binaries are built by `make e2e-binaries`; a skip here would "+
			"report a green suite that checked nothing)", args...)
		return
	}
	t.Skipf(format+"\n(set KTW_E2E=1, or run `make e2e`, to make this a failure instead)", args...)
}
