// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

func run(id, answer, errText string) cwmapi.HookRun {
	r := cwmapi.HookRun{ObjectID: id, URI: "x:check/" + id, Source: "capability:checkable", Error: errText}
	if answer != "" {
		r.Answer = json.RawMessage(answer)
	}
	return r
}

// TestOnlyPassPassesAndOnlyFailFails: every other answer — a status nobody
// counts, garbage, no answer, a hook that errored — is unproven, never folded
// into either column.
func TestOnlyPassPassesAndOnlyFailFails(t *testing.T) {
	out := readVerify(cwmapi.ObjectRunHooksOutput{Objects: 6, Runs: []cwmapi.HookRun{
		run("a", `{"status":"pass","verdict":"m.v-a"}`, ""),
		run("b", `{"status":"fail","evidence":"1 != 2"}`, ""),
		run("c", `{"status":"needs-harness"}`, ""),
		run("d", `"not a verify result"`, ""),
		run("e", "", ""),
		run("f", "", "timeout"),
	}})
	if out.Runs != 6 || out.Passed != 1 || len(out.Failed) != 1 || len(out.Unproven) != 4 || len(out.Outcomes) != 6 {
		t.Fatalf("out = runs %d passed %d failed %d unproven %d", out.Runs, out.Passed, len(out.Failed), len(out.Unproven))
	}
	if out.Outcomes[0].Verdict != "m.v-a" || out.Failed[0].Evidence != "1 != 2" {
		t.Errorf("the verdict and the evidence must be kept: %+v", out.Outcomes[:2])
	}
	for _, u := range out.Unproven[1:] {
		if u.Status != "hook-error" || u.Evidence == "" {
			t.Errorf("%s: %+v, want a hook-error saying why", u.ObjectID, u)
		}
	}
	if out.Green() {
		t.Error("a failure is not green")
	}
}

// TestGreenNeedsARun: nothing verified is not green; all passed is.
func TestGreenNeedsARun(t *testing.T) {
	if readVerify(cwmapi.ObjectRunHooksOutput{}).Green() {
		t.Error("no run, no verdict — yet green")
	}
	if !readVerify(cwmapi.ObjectRunHooksOutput{Runs: []cwmapi.HookRun{run("a", `{"status":"pass"}`, "")}}).Green() {
		t.Error("every run passed — not green")
	}
}

// TestABrokenLinterIsAnErrorFinding: a hook that errored or answered garbage
// becomes one error finding naming it; a clean answer adds nothing.
func TestABrokenLinterIsAnErrorFinding(t *testing.T) {
	out := readLint(cwmapi.ObjectRunHooksOutput{Runs: []cwmapi.HookRun{
		run("a", `{"status":"findings","findings":[{"rule":"r","message":"m","severity":"warn"}]}`, ""),
		run("b", `{"status":"ok","findings":[]}`, ""),
		run("c", `"garbage"`, ""),
		run("d", "", "unreachable"),
	}}, 4)
	if out.Runs != 4 || len(out.Findings) != 3 {
		t.Fatalf("out = %+v, want 4 runs and 3 findings", out)
	}
	if f := out.Findings[0]; f.Rule != "r" || f.Severity != "warn" || f.ObjectID != "a" {
		t.Errorf("the answered finding = %+v", f)
	}
	for _, f := range out.Findings[1:] {
		if f.Rule != "lint-hook-error" || f.Severity != "error" || f.URI == "" {
			t.Errorf("a broken hook = %+v, want an error finding naming it", f)
		}
	}
}

// TestBlocksAtIsTheBar: error by default, warn blocks warnings too, none
// blocks nothing, and a misspelt severity ranks as an error.
func TestBlocksAtIsTheBar(t *testing.T) {
	for _, c := range []struct {
		sev, bar string
		want     bool
	}{
		{"error", "", true}, {"warn", "", false}, {"info", "", false},
		{"warn", "warn", true}, {"warning", "warn", true}, {"info", "warn", false},
		{"info", "info", true},
		{"error", "none", false},
		{"eror", "", true},
	} {
		if got := BlocksAt(c.sev, c.bar); got != c.want {
			t.Errorf("BlocksAt(%q, %q) = %v, want %v", c.sev, c.bar, got, c.want)
		}
	}
}

// TestLintTakesExactlyOneScope: an input naming both an id and a module, or
// neither, or a subtree without an id, is refused by the service itself —
// before it looks for CWM, so no CWM is needed to see it.
func TestLintTakesExactlyOneScope(t *testing.T) {
	for name, in := range map[string]api.LintInput{
		"both":            {ID: "m.a", Module: "m"},
		"neither":         {},
		"subtree, module": {Module: "m", Subtree: true},
	} {
		if _, err := New().Lint(context.Background(), in); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("%s: %v, want ErrInvalidInput", name, err)
		}
	}
}
