// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// WHAT A RELEASE HOLDS — increments and direct work, never a goal or a feature
// directly; its level fits its content (a lint warns, the freeze refuses);
// its candidates are deliveries of the one release; its deliverables are the
// modules its content touched and its change-log page.

func TestAReleaseHoldsIncrementsAndDirectWork(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.1", Level: api.LevelPatch})
	f := want(t, ctx, cwm, "feature", "Send by mail")
	if _, err := rel.Target(ctx, api.TargetInput{ID: f, Release: r.ID}); err == nil || !strings.Contains(err.Error(), "increment") {
		t.Fatalf("a feature targeted a release directly: %v; want refused, pointing to an increment", err)
	}
	fix := file(t, ctx, req, api.FileInput{Title: "Fix the label", Module: "billing", Kind: api.KindFix, By: person})
	if _, err := rel.Target(ctx, api.TargetInput{ID: fix.ID, Release: r.ID}); err != nil {
		t.Fatalf("direct work targeting a release: %v; want accepted", err)
	}
	if _, err := rel.Plan(ctx, api.PlanInput{Module: "billing", Version: "1.5.0", Level: "huge"}); err == nil {
		t.Error("a level that is none of major, minor or patch was accepted")
	}
}

func TestAReleasesLevelFitsItsContent(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	rel := releases(t, ctx)
	inc := increments(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.1", Level: api.LevelPatch})
	in := increment(t, ctx, "Invoices reach their customers by mail")
	uc := usecase(t, ctx, cwm, "Send invoices by mail")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: uc, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	lint, err := verificationOf(t, ctx).Lint(ctx, verificationapi.LintInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(lint.Findings, func(f verificationapi.LintFinding) bool {
		return f.Rule == "level-mismatch" && strings.Contains(f.Message, "patch")
	}) {
		t.Errorf("a patch holding an increment: findings %+v; want level-mismatch", lint.Findings)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: person}); !errors.Is(err, cwmapi.ErrRefusedByHook) || !strings.Contains(err.Error(), "patch") {
		t.Fatalf("freezing a patch that holds an increment: %v; want refused", err)
	}
	if _, err := rel.Reschedule(ctx, api.RescheduleInput{ID: r.ID, Level: api.LevelMinor, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatalf("freezing a minor holding an increment: %v", err)
	}
	// A major needs a breaking increment.
	m := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2.0.0", Level: api.LevelMajor})
	soft, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Title: "Totals are clearer", ShipsIn: m.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: m.ID, By: person}); err == nil {
		t.Fatal("a major with no breaking increment froze")
	}
	if _, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Title: "Invoices use the new numbering", ShipsIn: m.ID, Breaking: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: m.ID, By: person}); err != nil {
		t.Fatalf("a major with a breaking increment: %v", err)
	}
	_ = soft
}

func TestAReleaseRecordsItsCandidatesAndListsItsDeliverables(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel := releases(t, ctx)
	inc := increments(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2026.10", Level: api.LevelMinor})
	if _, err := rel.Deliver(ctx, api.DeliverInput{ID: r.ID, Name: "rc1", By: person}); err == nil {
		t.Error("a planned release was delivered")
	}
	in := increment(t, ctx, "Invoices reach their customers by mail")
	uc := usecase(t, ctx, cwm, "Send invoices by mail")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: uc, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	work := file(t, ctx, req, api.FileInput{Title: "Mail them", Module: "billing", Kind: api.KindBuild, By: person})
	if _, err := req.Assess(ctx, api.AssessInput{ID: work.ID, Serves: uc, By: intake}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	for _, rc := range []string{"rc1", "rc2"} {
		if _, err := rel.Deliver(ctx, api.DeliverInput{ID: r.ID, Name: rc, By: person}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := rel.Deliver(ctx, api.DeliverInput{ID: r.ID, Name: "rc1", By: person}); err == nil {
		t.Error("a candidate name was used twice")
	}
	got, err := rel.Get(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if c := got.Release.Candidates; len(c) != 2 || c[0].Name != "rc1" || c[1].Name != "rc2" {
		t.Errorf("candidates %+v; want rc1 then rc2", c)
	}
	d, err := rel.Deliverables(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(d.Modules, []string{"billing"}) || d.ChangeLog != "docs/confluence-tew/changelog/2026.10.md" {
		t.Errorf("deliverables %+v; want billing, and the 2026.10 page", d)
	}
}
