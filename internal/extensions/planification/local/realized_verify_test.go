// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// A REALIZED FEATURE IS VERIFIED BY AN AGENT — the shipped feature workflow:
// an agent's `verify` takes a realized feature to done once its verification
// passes (the gate ready-to-test), and is refused, naming the gate, while it
// fails; no person's confirmation is needed.
func TestARealizedFeatureIsVerifiedByAnAgent(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	realized := func(title string) string {
		t.Helper()
		f := want(t, ctx, cwm, "feature", title)
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "realized", By: session, Note: "already in the code"}); err != nil {
			t.Fatal(err)
		}
		return f
	}
	green, red := realized("Mail the invoices"), realized("Print the invoices")
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		functionVerification{status: map[string]string{green: "pass", red: "fail"}})

	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: green, To: "done", Transition: "verify", By: session, Note: "its acceptance criteria pass"}); err != nil {
		t.Fatalf("an agent verifying a realized feature whose verification passes: %v", err)
	}
	if st := stateOf(t, ctx, cwm, green); st != "done" {
		t.Errorf("verified, %s is %q; want done", green, st)
	}
	_, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: red, To: "done", Transition: "verify", By: session, Note: "try"})
	if err == nil || !strings.Contains(err.Error(), "ready-to-test") {
		t.Errorf("verifying a feature whose verification fails: %v; want it refused by ready-to-test", err)
	}
	if st := stateOf(t, ctx, cwm, red); st != "realized" {
		t.Errorf("refused, %s is %q; want still realized", red, st)
	}
}

// functionVerification answers the gate's verify() and lint() by object.
type functionVerification struct {
	verificationapi.VerificationService
	status map[string]string
}

func (v functionVerification) VerifyFunction(_ context.Context, in cwmapi.FunctionCall) (verificationapi.VerifySummary, error) {
	switch v.status[in.Object] {
	case "pass":
		return verificationapi.VerifySummary{Runs: 1, Passed: 1, Green: true}, nil
	case "fail":
		return verificationapi.VerifySummary{Runs: 1, Failed: 1}, nil
	}
	return verificationapi.VerifySummary{}, nil
}

func (v functionVerification) LintFunction(context.Context, cwmapi.FunctionCall) (verificationapi.LintSummary, error) {
	return verificationapi.LintSummary{}, nil
}

// A REALIZED FEATURE IS LEFT AS IT IS — recorded realized, nothing is filed on
// it: its verification is planned only when an increment needs it, never for
// every realized feature at once (the person, withdrawn).
func TestARealizedFeatureFilesNothing(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "Mail the invoices")
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "realized", By: session, Note: "already in the code"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	if got, err := req.Get(ctx, api.GetInput{ID: f}); err == nil && got.Request.ID == f {
		t.Fatalf("a request was filed on the realized feature %s: %+v", f, got.Request)
	}
	if names, _ := todosOfRequest(t, ctx, f); len(names) != 0 {
		t.Fatalf("the realized feature has steps: %v; want none", names)
	}
}

// todosOfRequest is a request's to-dos.
func todosOfRequest(t *testing.T, ctx context.Context, id string) ([]string, error) {
	t.Helper()
	var out []string
	for _, n := range steps(t, ctx, id) {
		out = append(out, n)
	}
	return out, nil
}
