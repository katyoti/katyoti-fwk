// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// RELEASES, end to end on a real CWM: the release type, its workflow, the
// targets / ships-in edges and the gate and lint hooks as shipped.

func releases(t *testing.T, ctx context.Context) api.ReleaseService {
	t.Helper()
	svc, err := application.UseService[api.ReleaseService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// usecase creates a use case in billing, in the manual workflow's initial
// state: work a release can target, done by its `done` state.
func usecase(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, title string) string {
	t.Helper()
	o := cwmapi.Object{Type: "use-case"}
	o.Parent = "billing"
	o.Name = title
	_ = o.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "closable"})
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o})
	if err != nil {
		t.Fatalf("create use case %q: %v", title, err)
	}
	return out.Object.ID
}

func plan(t *testing.T, ctx context.Context, rel api.ReleaseService, in api.PlanInput) api.Release {
	t.Helper()
	out, err := rel.Plan(ctx, in)
	if err != nil {
		t.Fatalf("Plan %+v: %v", in, err)
	}
	return out.Release
}

// changelogListed is a person confirming the release's change log lists what
// it ships — ready-to-release's one requirement.
func changelogListed(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) {
	t.Helper()
	planlocal.Settle(ctx, 5*time.Second) // the planner writes on the object after a refusal: CWM writes are not atomic (object-update-loses-concurrent-writes)
	if _, err := gatesOf(t, ctx).Confirm(ctx, gatesapi.ConfirmInput{ID: id, Gate: "ready-to-release", Requirement: "changelog-updated", By: person}); err != nil {
		t.Fatalf("confirm the change log of %s: %v", id, err)
	}
}

func finish(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) {
	t.Helper()
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: api.TargetDone, By: intake}); err != nil {
		t.Fatalf("finish %s: %v", id, err)
	}
}

// A module release lives in the module's planning space; a project release in
// sys-planification. A version is unique within a module.
func TestPlanFilesReleasesWhereTheyBelong(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	rel := releases(t, ctx)
	out, err := rel.Plan(ctx, api.PlanInput{Module: "billing", Version: "1.4.0", Name: "Invoices"})
	if err != nil {
		t.Fatal(err)
	}
	if dir := filepath.ToSlash(filepath.Dir(out.Path)); !strings.HasSuffix(dir, "modules/billing/.ktw/releases") {
		t.Errorf("filed at %s, want under modules/billing/.ktw/releases", out.Path)
	}
	if r := out.Release; r.State != api.ReleasePlanned || r.Version != "1.4.0" || r.Name != "Invoices" || r.Project {
		t.Errorf("release = %+v", r)
	}
	if _, err := rel.Plan(ctx, api.PlanInput{Module: "billing", Version: "1.4.0"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("a second 1.4.0 in billing: %v, want refused", err)
	}
	p := plan(t, ctx, rel, api.PlanInput{Version: "2026.10"})
	if !p.Project || p.Module != api.SystemModule {
		t.Errorf("project release = %+v, want in %s", p, api.SystemModule)
	}
}

// An object targets one release: retargeting replaces the edge. Re-versioning a
// release keeps every target — the edge names the release, not its version.
func TestTargetIsOneReleaseAndSurvivesReversioning(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	rel := releases(t, ctx)
	a := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	b := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.5.0"})
	g := usecase(t, ctx, cwm, "Invoices are sent")
	if _, err := rel.Target(ctx, api.TargetInput{ID: g, Release: a.ID}); err != nil {
		t.Fatal(err)
	}
	moved, err := rel.Target(ctx, api.TargetInput{ID: g, Release: b.ID})
	if err != nil || moved.Replaced != a.ID {
		t.Fatalf("retarget: %+v, %v; want it to replace %s", moved, err, a.ID)
	}
	if s, _ := rel.Scope(ctx, api.GetInput{ID: a.ID}); s.Total != 0 {
		t.Errorf("%s still ships %d target(s) after the retarget", a.ID, s.Total)
	}
	// Re-version b by hand: its target does not move.
	o, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: b.ID})
	if err != nil {
		t.Fatal(err)
	}
	obj := o.Object
	_ = obj.SetCapability(api.CapRelease, api.ReleaseFields{Version: "1.5.1"})
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: obj}); err != nil {
		t.Fatal(err)
	}
	s, err := rel.Scope(ctx, api.GetInput{ID: b.ID})
	if err != nil || s.Release.Version != "1.5.1" || s.Total != 1 || s.Targets[0].ID != g {
		t.Fatalf("after re-versioning: %+v, %v; want %s still in it", s, err, g)
	}
}

// Frozen: no new target. Only a person unfreezes; after that, targeting works
// again and the refreeze records the new scope.
func TestFreezeRefusesNewTargetsUntilAPersonUnfreezes(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	g1 := usecase(t, ctx, cwm, "one")
	g2 := usecase(t, ctx, cwm, "two")
	if _, err := rel.Target(ctx, api.TargetInput{ID: g1, Release: r.ID}); err != nil {
		t.Fatal(err)
	}
	frozen, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: intake})
	if err != nil || frozen.Release.State != api.ReleaseFrozen || len(frozen.Release.FrozenScope) != 1 {
		t.Fatalf("Freeze: %+v, %v", frozen.Release, err)
	}
	if _, err := rel.Target(ctx, api.TargetInput{ID: g2, Release: r.ID}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("target a frozen release: %v, want refused", err)
	}
	if _, err := rel.Unfreeze(ctx, api.MoveInput{ID: r.ID, By: intake}); !errors.Is(err, cwmapi.ErrTransitionRefused) {
		t.Fatalf("an agent unfreezing: %v, want refused (a person only)", err)
	}
	if got, _ := rel.Get(ctx, api.GetInput{ID: r.ID}); len(got.Release.FrozenScope) != 1 {
		t.Errorf("a refused unfreeze cleared the frozen scope: %+v", got.Release)
	}
	if _, err := rel.Unfreeze(ctx, api.MoveInput{ID: r.ID, By: person, Note: "add two"}); err != nil {
		t.Fatalf("a person unfreezing: %v", err)
	}
	if _, err := rel.Target(ctx, api.TargetInput{ID: g2, Release: r.ID}); err != nil {
		t.Fatalf("target after unfreeze: %v", err)
	}
	again, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: intake})
	if err != nil || len(again.Release.FrozenScope) != 2 {
		t.Fatalf("refreeze: %+v, %v; want both targets in the scope", again.Release, err)
	}
}

// A raw freeze — not through Freeze, so without its scope — is refused.
func TestAFreezeMustRecordItsScope(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	if _, err := rel.Target(ctx, api.TargetInput{ID: usecase(t, ctx, cwm, "one"), Release: r.ID}); err != nil {
		t.Fatal(err)
	}
	_, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: r.ID, To: api.ReleaseFrozen, By: intake})
	if !errors.Is(err, cwmapi.ErrRefusedByHook) {
		t.Fatalf("raw freeze without a scope: %v, want refused by the hook", err)
	}
}

// A release ships only when every target is done; a target added behind the
// service's back after the freeze is a lint finding and blocks the release.
func TestReleaseWaitsOnEveryTargetAndItsFrozenScope(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	g1 := usecase(t, ctx, cwm, "one")
	if _, err := rel.Target(ctx, api.TargetInput{ID: g1, Release: r.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: intake}); err != nil {
		t.Fatal(err)
	}
	changelogListed(t, ctx, cwm, r.ID)
	if _, err := rel.Release(ctx, api.MoveInput{ID: r.ID, By: person}); !errors.Is(err, cwmapi.ErrRefusedByHook) || !strings.Contains(err.Error(), g1) {
		t.Fatalf("release with %s not done: %v, want refused naming it", g1, err)
	}
	// Behind the service's back: a takes edge written straight onto the
	// frozen release.
	sneak := usecase(t, ctx, cwm, "sneaked in")
	taken(t, ctx, cwm, r.ID, sneak)
	lint, err := verificationOf(t, ctx).Lint(ctx, verificationapi.LintInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range lint.Findings {
		found = found || (f.Rule == "target-after-freeze" && strings.Contains(f.Message, sneak))
	}
	if !found {
		t.Errorf("lint findings = %+v, want target-after-freeze naming %s", lint.Findings, sneak)
	}
	finish(t, ctx, cwm, g1)
	finish(t, ctx, cwm, sneak)
	if _, err := rel.Release(ctx, api.MoveInput{ID: r.ID, By: person}); !errors.Is(err, cwmapi.ErrRefusedByHook) || !strings.Contains(err.Error(), "after the freeze") {
		t.Fatalf("release with a target outside its frozen scope: %v, want refused", err)
	}
	if _, err := rel.Untarget(ctx, api.UntargetInput{ID: sneak}); err != nil {
		t.Fatal(err)
	}
	out, err := rel.Release(ctx, api.MoveInput{ID: r.ID, By: person})
	if err != nil || out.Release.State != api.ReleaseReleased {
		t.Fatalf("release with every target done: %+v, %v", out.Release, err)
	}
}

// A target that ships nothing - dropped - holds the release back no more than
// it counts in its scope: the released guard leaves it out.
func TestADroppedTargetDoesNotHoldTheReleaseBack(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	g1 := usecase(t, ctx, cwm, "one")
	if _, err := rel.Target(ctx, api.TargetInput{ID: g1, Release: r.ID}); err != nil {
		t.Fatal(err)
	}
	gone := want(t, ctx, cwm, "feature", "Print the invoices")
	taken(t, ctx, cwm, r.ID, gone)
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: gone, To: api.StateDropped, By: person, Note: "not this way"}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: intake}); err != nil {
		t.Fatal(err)
	}
	finish(t, ctx, cwm, g1)
	changelogListed(t, ctx, cwm, r.ID)
	out, err := rel.Release(ctx, api.MoveInput{ID: r.ID, By: person})
	if err != nil || out.Release.State != api.ReleaseReleased {
		t.Fatalf("release with its one live target done and %s dropped: %+v, %v; want released", gone, out.Release, err)
	}
}

// Only corrections enter a frozen release: a fix request linked after the
// freeze is in its scope - taken by Target, no target-after-freeze finding,
// not named by the released guard - while a build request linked behind the
// service's back still is off its scope.
func TestAFixLinkedAfterTheFreezeIsInTheReleaseScope(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	if _, err := rel.Target(ctx, api.TargetInput{ID: usecase(t, ctx, cwm, "one"), Release: r.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: intake}); err != nil {
		t.Fatal(err)
	}
	fix := file(t, ctx, req, api.FileInput{Title: "The totals are rounded wrong", Kind: api.KindFix}).ID
	build := file(t, ctx, req, api.FileInput{Title: "Mail the invoices", Kind: api.KindBuild}).ID
	if _, err := rel.Target(ctx, api.TargetInput{ID: fix, Release: r.ID}); err != nil {
		t.Fatalf("target a fix at the frozen release: %v, want taken", err)
	}
	if _, err := rel.Target(ctx, api.TargetInput{ID: build, Release: r.ID}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("target a build request at the frozen release: %v, want refused", err)
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: build, To: r.ID, Kind: api.EdgeTargets}); err != nil {
		t.Fatal(err)
	}
	scope, err := rel.Scope(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(scope.OffScope, build) || slices.Contains(scope.OffScope, fix) {
		t.Errorf("off scope = %v; want %s (a build linked after the freeze) and not %s (a fix)", scope.OffScope, build, fix)
	}
	lint, err := verificationOf(t, ctx).Lint(ctx, verificationapi.LintInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	var after []string
	for _, f := range lint.Findings {
		if f.Rule == "target-after-freeze" {
			after = append(after, f.Message)
		}
	}
	if len(after) != 1 || !strings.Contains(after[0], build) {
		t.Errorf("target-after-freeze findings = %v; want one, naming %s only", after, build)
	}
	changelogListed(t, ctx, cwm, r.ID)
	_, err = rel.Release(ctx, api.MoveInput{ID: r.ID, By: person})
	if !errors.Is(err, cwmapi.ErrRefusedByHook) || !strings.Contains(err.Error(), build+" was added after the freeze") || strings.Contains(err.Error(), fix+" was added after the freeze") {
		t.Fatalf("release refused: %v; want the build named after the freeze, never the fix", err)
	}
}

// A project release ships its own targets and its module releases'; it is
// released only once each module release is.
func TestAProjectReleaseIncludesItsModuleReleases(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	rel := releases(t, ctx)
	p := plan(t, ctx, rel, api.PlanInput{Version: "2026.10"})
	m := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0", ShipsIn: p.ID})
	if m.ShipsIn != p.ID {
		t.Fatalf("module release ships in %q, want %s", m.ShipsIn, p.ID)
	}
	g := usecase(t, ctx, cwm, "in the module release")
	if _, err := rel.Target(ctx, api.TargetInput{ID: g, Release: m.ID}); err != nil {
		t.Fatal(err)
	}
	s, err := rel.Scope(ctx, api.GetInput{ID: p.ID})
	if err != nil || s.Total != 1 || s.Targets[0].Via != m.ID || len(s.Modules) != 1 {
		t.Fatalf("project scope = %+v, %v; want %s via %s", s, err, g, m.ID)
	}
	if _, err := rel.ShipIn(ctx, api.ShipInInput{ID: p.ID, In: p.ID}); err == nil {
		t.Error("a project release shipping in a project release was accepted")
	}
	finish(t, ctx, cwm, g)
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: p.ID, By: intake}); err != nil {
		t.Fatal(err)
	}
	changelogListed(t, ctx, cwm, p.ID)
	if _, err := rel.Release(ctx, api.MoveInput{ID: p.ID, By: person}); !errors.Is(err, cwmapi.ErrRefusedByHook) || !strings.Contains(err.Error(), m.ID) {
		t.Fatalf("project release before its module release: %v, want refused naming %s", err, m.ID)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: m.ID, By: intake}); err != nil {
		t.Fatal(err)
	}
	changelogListed(t, ctx, cwm, m.ID)
	if _, err := rel.Release(ctx, api.MoveInput{ID: m.ID, By: person}); err != nil {
		t.Fatalf("module release: %v", err)
	}
	if _, err := rel.Release(ctx, api.MoveInput{ID: p.ID, By: person}); err != nil {
		t.Fatalf("project release once its module release shipped: %v", err)
	}
}

// The backlog is the module's unfinished work that targets nothing: requests
// and what it wants — not what is targeted, not what is done.
func TestBacklogIsTheUntargetedUnfinishedWork(t *testing.T) {
	t.Parallel()
	ctx, cwm, svc := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	targeted := usecase(t, ctx, cwm, "targeted")
	loose := usecase(t, ctx, cwm, "loose")
	finished := usecase(t, ctx, cwm, "finished")
	req := file(t, ctx, svc, api.FileInput{Title: "a request"})
	if _, err := rel.Target(ctx, api.TargetInput{ID: targeted, Release: r.ID}); err != nil {
		t.Fatal(err)
	}
	finish(t, ctx, cwm, finished)
	out, err := rel.Backlog(ctx, api.BacklogInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, it := range out.Items {
		got[it.ID] = true
	}
	if !got[loose] || !got[req.ID] || got[targeted] || got[finished] || len(out.Items) != 2 {
		t.Fatalf("backlog = %+v, want exactly %s and %s", out.Items, loose, req.ID)
	}
}

// A release ships only once a person confirmed its change log lists what it
// ships (ready-to-release); the refusal names the requirement.
func TestAReleaseWaitsOnItsChangeLog(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: intake}); err != nil {
		t.Fatal(err)
	}
	_, err := rel.Release(ctx, api.MoveInput{ID: r.ID, By: person})
	cl, refused := gatesapi.RefusedChecklist(err)
	if !refused || cl.Gate != "ready-to-release" || len(cl.Unmet()) != 1 || cl.Unmet()[0].Name != "changelog-updated" {
		t.Fatalf("release before its change log is confirmed: %v, want ready-to-release refusing changelog-updated", err)
	}
	changelogListed(t, ctx, cwm, r.ID)
	if out, err := rel.Release(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil || out.Release.State != api.ReleaseReleased {
		t.Fatalf("release once its change log is confirmed: %+v, %v", out.Release, err)
	}
}

// gatesOf is the gates service the test world registers.
func gatesOf(t *testing.T, ctx context.Context) gatesapi.GatesService {
	t.Helper()
	svc, err := application.UseService[gatesapi.GatesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// A release can be rescheduled while it has not shipped: its date, its budget,
// only what is given; a bad date or a negative budget is refused, and a release
// already released keeps what it had.
func TestAReleaseIsRescheduled(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0", Date: "2026-11-01", BudgetUSD: 50})
	got, err := rel.Reschedule(ctx, api.RescheduleInput{ID: r.ID, Date: "2026-12-15", By: person})
	if err != nil {
		t.Fatal(err)
	}
	if got.Release.Date != "2026-12-15" || got.Release.BudgetUSD != 50 {
		t.Fatalf("after a new date: %s, $%g; want 2026-12-15 and the budget unchanged", got.Release.Date, got.Release.BudgetUSD)
	}
	budget := 80.0
	if got, err = rel.Reschedule(ctx, api.RescheduleInput{ID: r.ID, BudgetUSD: &budget, By: person}); err != nil {
		t.Fatal(err)
	}
	if got.Release.Date != "2026-12-15" || got.Release.BudgetUSD != 80 {
		t.Fatalf("after a new budget: %s, $%g; want the date unchanged and $80", got.Release.Date, got.Release.BudgetUSD)
	}
	negative := -1.0
	for _, bad := range []api.RescheduleInput{
		{ID: r.ID, Date: "15/12/2026", By: person},
		{ID: r.ID, BudgetUSD: &negative, By: person},
		{ID: r.ID, By: person},
	} {
		if _, err := rel.Reschedule(ctx, bad); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("Reschedule(%+v) = %v, want refused", bad, err)
		}
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: intake}); err != nil {
		t.Fatal(err)
	}
	changelogListed(t, ctx, cwm, r.ID)
	if _, err := rel.Release(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Reschedule(ctx, api.RescheduleInput{ID: r.ID, Date: "2027-01-01", By: person}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("rescheduling a released release = %v, want refused", err)
	}
}
