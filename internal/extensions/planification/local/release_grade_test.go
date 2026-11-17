// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// gradedVerification answers each object's verification as the test set it:
// pass, fail, or nothing run.
type gradedVerification struct {
	verificationapi.VerificationService
	status map[string]string
}

func (v gradedVerification) Verify(_ context.Context, in verificationapi.VerifyInput) (verificationapi.VerifyOutput, error) {
	switch v.status[in.ID] {
	case "pass":
		return verificationapi.VerifyOutput{Runs: 1, Passed: 1}, nil
	case "fail":
		return verificationapi.VerifyOutput{Runs: 1, Failed: []verificationapi.VerifyOutcome{{}}}, nil
	case "pass-extended": // its criterion ran on a bound it declared
		return verificationapi.VerifyOutput{Runs: 1, Passed: 1, Outcomes: []verificationapi.VerifyOutcome{
			{ObjectID: in.ID + "-AC1", Status: "pass", Timeout: "30m"}}}, nil
	}
	return verificationapi.VerifyOutput{}, nil
}

// A release is the committed slice of what targets it: its grade is the
// verification of what judges it, else of what targets it — green only when
// every member passed. What judges it outside what targets it is a lint
// finding.
func TestAReleaseIsGradedByWhatJudgesIt(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2.0.0", Name: "Pay by card"})
	pay, refund, receipt := usecase(t, ctx, cwm, "pay"), usecase(t, ctx, cwm, "refund"), usecase(t, ctx, cwm, "receipt")
	for _, id := range []string{pay, refund, receipt} {
		if _, err := rel.Target(ctx, api.TargetInput{ID: id, Release: r.ID}); err != nil {
			t.Fatal(err)
		}
	}
	v := gradedVerification{status: map[string]string{pay: "pass", refund: "fail"}}
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default", v)

	g, err := rel.Grade(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if g.Over != api.EdgeTargets || g.Total != 3 || g.Passed != 1 || g.Status != "unmet" || len(g.Frontier) != 2 {
		t.Fatalf("grade over its targets: %+v; want 1 of 3 passed, unmet", g)
	}
	status := map[string]string{}
	for _, m := range g.Members {
		status[m.ID] = m.Status
	}
	if status[pay] != api.MemberPass || status[refund] != api.MemberFail || status[receipt] != api.MemberUnproven {
		t.Errorf("members %v; want pay pass, refund fail, receipt unproven (nothing verified)", status)
	}

	// Judged by pay alone: green.
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: r.ID, To: pay, Kind: cwmapi.EdgeType(api.EdgeJudgedBy)}); err != nil {
		t.Fatal(err)
	}
	g, err = rel.Grade(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if g.Over != api.EdgeJudgedBy || g.Total != 1 || g.Status != "green" {
		t.Fatalf("grade over what judges it: %+v; want pay alone, green", g)
	}

	// Judged by what does not target it: a lint finding.
	stray := usecase(t, ctx, cwm, "stray")
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: r.ID, To: stray, Kind: cwmapi.EdgeType(api.EdgeJudgedBy)}); err != nil {
		t.Fatal(err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	lint, err := rel.Lint(ctx, api.HookInput{Object: &got.Object})
	if err != nil {
		t.Fatal(err)
	}
	if len(lint.Findings) != 1 || lint.Findings[0].Rule != "judged-outside-targets" || !strings.Contains(lint.Findings[0].Message, stray) {
		t.Fatalf("lint %+v; want what judges it outside its targets", lint)
	}
}

// A release's grade says which check ran on an extended bound — one its
// checkable declared longer than the default — and only that one.
func TestAReleaseGradeSaysWhichCheckRanOnAnExtendedBound(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2.0.0", Name: "Pay by card"})
	slow, quick := usecase(t, ctx, cwm, "slow"), usecase(t, ctx, cwm, "quick")
	for _, id := range []string{slow, quick} {
		if _, err := rel.Target(ctx, api.TargetInput{ID: id, Release: r.ID}); err != nil {
			t.Fatal(err)
		}
	}
	v := gradedVerification{status: map[string]string{slow: "pass-extended", quick: "pass"}}
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default", v)

	g, err := rel.Grade(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	extended := map[string][]string{}
	for _, m := range g.Members {
		extended[m.ID] = m.Extended
	}
	if g.Status != "green" || len(extended[slow]) != 1 || extended[slow][0] != slow+"-AC1 (30m)" || len(extended[quick]) != 0 {
		t.Fatalf("grade %+v, extended %v; want green, %s-AC1 (30m) named under %s alone", g, extended, slow, slow)
	}
}
