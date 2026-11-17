// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// declareTimeout sets the criterion m.ac-slow's checkable timeout.
func declareTimeout(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, timeout string) {
	t.Helper()
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.ac-slow"})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	grp, _ := o.Capability(cwmapi.CapCheckable)
	grp["timeout"] = timeout
	_ = o.SetCapability(cwmapi.CapCheckable, grp)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o, Revision: got.Revision}); err != nil {
		t.Fatalf("declare timeout %s: %v", timeout, err)
	}
}

// TestAProbeRunsToItsDeclaredTimeout: a command probe longer than the default
// bound runs to the bound its checkable declares and passes, through the bus
// and the verification service — the verify answer and the verdict say it ran
// on that extended bound; without one the same probe is stopped at the
// default; a bound that cannot be read is never guessed at: the probe is not
// run.
func TestAProbeRunsToItsDeclaredTimeout(t *testing.T) {
	const def = 1 * time.Second
	probe := sleepProbe(3 * time.Second)

	t.Run("declared", func(t *testing.T) {
		ctx, cwm := overTheBus(t, def, probe)
		declareTimeout(t, ctx, cwm, "20s")
		out, err := verificationOf(t, ctx).Verify(ctx, verificationapi.VerifyInput{ID: "m.uc"})
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if out.Passed != 1 || len(out.Outcomes) != 1 || out.Outcomes[0].Timeout != "20s" {
			t.Fatalf("outcomes %+v; want m.ac-slow passed on its extended bound 20s", out.Outcomes)
		}
		if grp := verdictOf(t, ctx, cwm); grp["status"] != "pass" || grp["timeout"] != "20s" {
			t.Fatalf("verdict = %+v, want pass on timeout 20s", grp)
		}
	})
	t.Run("default", func(t *testing.T) {
		ctx, cwm := overTheBus(t, def, probe)
		out, err := verificationOf(t, ctx).Verify(ctx, verificationapi.VerifyInput{ID: "m.uc"})
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if len(out.Failed) != 1 || !strings.Contains(out.Failed[0].Evidence, "timed out after 1s") || out.Failed[0].Timeout != "" {
			t.Fatalf("failed %+v; want m.ac-slow stopped at the default 1s, no extended bound", out.Failed)
		}
		if grp := verdictOf(t, ctx, cwm); grp["timeout"] != nil {
			t.Fatalf("verdict = %+v, want no timeout on the default bound", grp)
		}
	})
	t.Run("refused", func(t *testing.T) {
		ctx, cwm := overTheBus(t, def, "true")
		declareTimeout(t, ctx, cwm, "2h")
		start := time.Now()
		out, err := verificationOf(t, ctx).Verify(ctx, verificationapi.VerifyInput{ID: "m.uc"})
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if len(out.Unproven) != 1 || out.Unproven[0].Status != "harness-error" ||
			!strings.Contains(out.Unproven[0].Evidence, "above the ceiling") || !strings.Contains(out.Unproven[0].Evidence, "not run") {
			t.Fatalf("unproven %+v, failed %+v; want a harness error naming the ceiling, the probe not run", out.Unproven, out.Failed)
		}
		if time.Since(start) > 30*time.Second {
			t.Fatal("the refused probe took too long")
		}
	})
}
