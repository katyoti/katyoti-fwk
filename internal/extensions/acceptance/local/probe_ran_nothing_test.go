// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/acceptance/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// fakeGo puts first on PATH a `go` that prints out and exits 0, as a go test
// does whose -run matches no test (or one that ran its test).
func fakeGo(t *testing.T, out string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\n' '" + out + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestCheckableProbeThatRunsNoTestIsNoPass: a criterion's command probe whose
// go test exits 0 having run no test (a -run naming a test renamed or not yet
// written: "[no tests to run]") proves nothing - its verdict is a fail saying
// why, never a pass; the same probe whose go test ran its test passes.
func TestCheckableProbeThatRunsNoTestIsNoPass(t *testing.T) {
	const probe = "go test ./pkg/ -run TestGone"

	t.Run("ran no test", func(t *testing.T) {
		fakeGo(t, "ok  \tgithub.com/x/pkg\t0.5s [no tests to run]")
		ctx, cwm := overTheBus(t, api.ProbeTimeout, probe)
		out, err := verificationOf(t, ctx).Verify(ctx, verificationapi.VerifyInput{ID: "m.uc"})
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if out.Passed != 0 || len(out.Failed) != 1 || !strings.Contains(out.Failed[0].Evidence, "ran no test") {
			t.Fatalf("passed %d failed %+v unproven %+v; want m.ac-slow failed, saying the go test ran no test", out.Passed, out.Failed, out.Unproven)
		}
		if grp := verdictOf(t, ctx, cwm); grp["status"] != "fail" {
			t.Fatalf("verdict = %+v, want fail", grp)
		}
	})
	t.Run("ran its test", func(t *testing.T) {
		fakeGo(t, "ok  \tgithub.com/x/pkg\t0.5s")
		ctx, cwm := overTheBus(t, api.ProbeTimeout, probe)
		out, err := verificationOf(t, ctx).Verify(ctx, verificationapi.VerifyInput{ID: "m.uc"})
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if out.Passed != 1 {
			t.Fatalf("passed %d failed %+v unproven %+v; want m.ac-slow passed", out.Passed, out.Failed, out.Unproven)
		}
		if grp := verdictOf(t, ctx, cwm); grp["status"] != "pass" {
			t.Fatalf("verdict = %+v, want pass", grp)
		}
	})
}
