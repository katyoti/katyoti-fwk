// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// changeLogConfirmed is the person confirming a release's change log: the
// release's own gate, beside the candidate's.
func changeLogConfirmed(t *testing.T, ctx context.Context, release string) {
	t.Helper()
	if _, err := gatesOf(t, ctx).Confirm(ctx, gatesapi.ConfirmInput{ID: release, Gate: api.GateReadyToRelease, Requirement: "changelog-updated", By: person}); err != nil {
		t.Fatal(err)
	}
}

// gate is one gate's line of a release's gates.
func gate(t *testing.T, g api.GatesOutput, name string) api.GateLine {
	t.Helper()
	for _, l := range g.Gates {
		if l.Name == name {
			return l
		}
	}
	t.Fatalf("no gate %s in %+v", name, g.Gates)
	return api.GateLine{}
}

// A RELEASE IS PROMOTED ONLY WHEN ITS GATES PASS - on
// its candidate: every increment it carries done, every increment of another
// stream one of them depends on done, no open P0 fix on it, the end-to-end
// test green on the candidate's build, the alpha sign-off by a person; while
// one fails, the promotion is refused naming it.
func TestAReleaseIsPromotedOnlyWhenItsGatesPass(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	sm, rel, inc, plans := streams(t, ctx), releases(t, ctx), increments(t, ctx), plansOf(t, ctx)
	stream := func(name string) string {
		t.Helper()
		out, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: name, StreamFields: api.StreamFields{ReleaseMode: api.ModeManual}})
		if err != nil {
			t.Fatal(err)
		}
		return out.Stream.ID
	}
	product, platform := stream("Product"), stream("Platform")
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2026.10", Date: "2026-10-31", Stream: product})
	study := increment(t, ctx, "The study end to end").ID
	install := increment(t, ctx, "One-click install").ID
	for id, s := range map[string]string{study: product, install: platform} {
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: id, Stream: s}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: study, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: study, To: install, Kind: api.EdgeDependsOn}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Deliver(ctx, api.DeliverInput{ID: r.ID, Build: "b-101", By: person}); err != nil {
		t.Fatal(err)
	}
	fix := file(t, ctx, req, api.FileInput{Title: "The installer stops at the licence page", Kind: api.KindFix, On: r.ID, Priority: "P0"})
	minor := file(t, ctx, req, api.FileInput{Title: "A typo on the licence page", Kind: api.KindFix, Module: "billing", Priority: "P2", Increment: study})

	gates := func() api.GatesOutput {
		t.Helper()
		g, err := rel.Gates(ctx, api.GatesInput{ID: r.ID})
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	refused := func(want string) {
		t.Helper()
		_, err := rel.Promote(ctx, api.CandidateInput{ID: r.ID, Candidate: "rc1", By: person})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("promoted with %s failing: %v", want, err)
		}
	}
	g := gates()
	if g.Stage != "candidate" || g.Candidate != "rc1" || g.Build != "b-101" || g.Ready {
		t.Fatalf("its gates: %+v; want the candidate rc1 (b-101), not ready", g)
	}
	for _, name := range []string{api.GateIncrements, api.GateDependencies, api.GateNoOpenP0Fix, api.GateEndToEnd, api.GateSignOff} {
		if gate(t, g, name).Passed {
			t.Errorf("gate %s passes before anything was done", name)
		}
	}
	if !strings.Contains(gate(t, g, api.GateDependencies).Why, "One-click install (Platform)") {
		t.Errorf("the dependency gate: %q; want it naming %s of the other stream by its title and its stream's", gate(t, g, api.GateDependencies).Why, install)
	}
	refused(api.GateIncrements)
	if g := gates(); !slices.Contains(g.Filed, fix.ID) || !slices.Contains(g.Filed, minor.ID) {
		t.Errorf("what its testing filed: %v; want %s and %s", g.Filed, fix.ID, minor.ID)
	}

	if _, err := plans.Finish(ctx, api.FinishInput{ID: minor.ID, Outcome: api.PlanDone, By: person, Note: "fixed too"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{study, install} {
		if got, _ := inc.Get(ctx, api.GetInput{ID: id}); got.Increment.State == api.IncrementDone {
			continue
		}
		if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: id, By: person, Force: true, Note: forcedNote}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := plans.Finish(ctx, api.FinishInput{ID: fix.ID, Outcome: api.PlanDone, By: person, Note: "fixed"}); err != nil {
		t.Fatal(err)
	}
	// A P2 fix still open does not hold the promotion; the finished one stays among what was filed.
	if g := gates(); !gate(t, g, api.GateNoOpenP0Fix).Passed || !slices.Contains(g.Filed, fix.ID) {
		t.Errorf("after the P0 fix ended: no-open-p0-fix %+v, filed %v; want passing, the fix still listed", gate(t, g, api.GateNoOpenP0Fix), g.Filed)
	}
	g = gates()
	for _, name := range []string{api.GateIncrements, api.GateDependencies, api.GateNoOpenP0Fix} {
		if !gate(t, g, name).Passed {
			t.Errorf("gate %s: %+v; want it passing now", name, gate(t, g, name))
		}
	}
	refused(api.GateEndToEnd)
	if _, err := rel.RecordGate(ctx, api.RecordGateInput{ID: r.ID, Candidate: "rc1", Gate: api.GateEndToEnd, Passed: true, Why: "clean machine run green", By: builder}); err != nil {
		t.Fatal(err)
	}
	refused(api.GateSignOff)
	if _, err := rel.SignOff(ctx, api.CandidateInput{ID: r.ID, Candidate: "rc1", By: builder}); err == nil {
		t.Error("an agent signed a candidate off")
	}
	if _, err := rel.SignOff(ctx, api.CandidateInput{ID: r.ID, Candidate: "rc1", By: person}); err != nil {
		t.Fatal(err)
	}
	// Every gate of the candidate passes; the release's change log is still
	// to confirm: shown, and left to the release's own guard.
	if g = gates(); g.Ready || gate(t, g, api.GateChangeLog).Passed {
		t.Fatalf("every candidate gate done, the change log unconfirmed: %+v; want not ready, change-log open", g)
	}
	for _, l := range g.Gates {
		if l.Name != api.GateChangeLog && !l.Passed {
			t.Fatalf("every candidate gate done: %+v open", l)
		}
	}
	// The release's own gate still refuses: nothing is recorded promoted.
	if _, err := rel.Promote(ctx, api.CandidateInput{ID: r.ID, Candidate: "rc1", By: person}); err == nil {
		t.Fatal("promoted with its change log unconfirmed")
	}
	if got, _ := rel.Get(ctx, api.GetInput{ID: r.ID}); got.Release.Candidates[0].Promoted || got.Release.State != api.ReleaseFrozen {
		t.Fatalf("a refused promotion left %+v (%s); want nothing promoted, the release frozen", got.Release.Candidates[0], got.Release.State)
	}
	changeLogConfirmed(t, ctx, r.ID)
	if g = gates(); !g.Ready {
		t.Fatalf("every gate done, its change log confirmed: %+v; want ready", g)
	}
	out, err := rel.Promote(ctx, api.CandidateInput{ID: r.ID, Candidate: "rc1", By: person})
	if err != nil {
		t.Fatal(err)
	}
	if out.Release.State != api.ReleaseReleased {
		t.Errorf("promoted: %s; want released", out.Release.State)
	}
}

// CANDIDATES ARE DELIVERIES PROMOTED WITHOUT REBUILD - a
// frozen release's deliveries are numbered rc1, rc2... when unnamed, each with
// its build; a planned release has none; the promoted candidate keeps its
// build and the release keeps its official version.
func TestCandidatesAreDeliveriesPromotedWithoutRebuild(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2026.11"})
	if _, err := rel.Deliver(ctx, api.DeliverInput{ID: r.ID, Build: "b-1", By: person}); err == nil {
		t.Error("a planned release was delivered: a candidate is a delivery of a frozen release")
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	for _, build := range []string{"b-201", "b-202"} {
		if _, err := rel.Deliver(ctx, api.DeliverInput{ID: r.ID, Build: build, By: person}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := rel.Get(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	cs := got.Release.Candidates
	if len(cs) != 2 || cs[0].Name != "rc1" || cs[1].Name != "rc2" || cs[1].Build != "b-202" {
		t.Fatalf("its candidates: %+v; want rc1 then rc2, each with its build", cs)
	}
	if _, err := rel.RecordGate(ctx, api.RecordGateInput{ID: r.ID, Candidate: "rc2", Gate: api.GateEndToEnd, Passed: true, By: builder}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.SignOff(ctx, api.CandidateInput{ID: r.ID, Candidate: "rc2", By: person}); err != nil {
		t.Fatal(err)
	}
	changeLogConfirmed(t, ctx, r.ID)
	if _, err := rel.Promote(ctx, api.CandidateInput{ID: r.ID, Candidate: "rc1", By: person}); err == nil {
		t.Error("rc1 was promoted while rc2, the last candidate, exists")
	}
	if _, err := rel.Promote(ctx, api.CandidateInput{ID: r.ID, Candidate: "rc2", By: person}); err != nil {
		t.Fatal(err)
	}
	got, _ = rel.Get(ctx, api.GetInput{ID: r.ID})
	if p := got.Release.Candidates[1]; !p.Promoted || p.Build != "b-202" || got.Release.Version != "2026.11" || got.Release.State != api.ReleaseReleased {
		t.Errorf("promoted rc2: %+v, version %s, %s; want rc2 promoted with its build b-202, the release 2026.11 released", p, got.Release.Version, got.Release.State)
	}
	if got.Release.Candidates[0].Promoted {
		t.Error("rc1 was marked promoted")
	}
}

// THE RELEASE OVERVIEW ANSWERS « CAN I SHIP? » - opening a
// candidate release shows its stage, its verdict with what is open and its
// next action; its gauges; each gate with why it fails; the steps waiting on
// a person; its increments by priority with what they wait on; the increments
// of other streams they need, apart; and an open P2 fix does not hold it.
func TestTheReleaseOverviewAnswersCanIShip(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	stream := func(name string) string {
		t.Helper()
		out, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: name, StreamFields: api.StreamFields{ReleaseMode: api.ModeManual}})
		if err != nil {
			t.Fatal(err)
		}
		return out.Stream.ID
	}
	product, platform := stream("Product"), stream("Platform")
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2026.10", Date: "2026-10-31", Stream: product})
	study := increment(t, ctx, "The study end to end").ID
	install := increment(t, ctx, "One-click install").ID
	for id, s := range map[string]string{study: product, install: platform} {
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: id, Stream: s}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: study, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	// A request in the increment waiting on a person: its analysis done, its
	// acceptance a person's - a preparation step, which the increment's wait
	// on another does not hold.
	asks := file(t, ctx, req, api.FileInput{Title: "Review the study", Module: "billing", Kind: api.KindAnalyse, Increment: study})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, asks.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: study, To: install, Kind: api.EdgeDependsOn}); err != nil {
		t.Fatal(err)
	}
	done := increment(t, ctx, "Exports").ID
	if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: done, Stream: product}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: done, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: done, By: person, Force: true, Note: forcedNote}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Deliver(ctx, api.DeliverInput{ID: r.ID, Build: "b-101", By: person}); err != nil {
		t.Fatal(err)
	}
	// A P2 fix on the release, open.
	minor := file(t, ctx, req, api.FileInput{Title: "A typo on the cover", Kind: api.KindFix, On: r.ID, Priority: "P2"})
	// A second increment, done (it folds to the bottom); a request running, one blocked, one asking a person.
	running := file(t, ctx, req, api.FileInput{Title: "Generate the sections", Module: "billing", Kind: api.KindBuild, Increment: study})
	if got, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: stepOf(t, ctx, running.ID, api.StepIntake).ID, By: session}); err != nil || got.Todo == nil {
		t.Fatalf("pull its intake: %v %+v", err, got)
	}
	stuck := file(t, ctx, req, api.FileInput{Title: "Draw the flows", Module: "billing", Kind: api.KindBuild, Increment: study})
	if _, err := plansOf(t, ctx).Block(ctx, api.BlockInput{ID: stuck.ID, Note: "the drawing engine is down", By: session}); err != nil {
		t.Fatal(err)
	}

	def, err := releasePane(ctx, rel, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("the overview must display: %v", err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	page := string(raw)
	for _, want := range []string{
		"NOT READY TO PROMOTE - 5 open: increments", "stage candidate", "candidate rc1 (build b-101)",
		"left: ", "Generate the sections (", `"state":"running"`, `"label":"to do","value":`, " to do, ", " · since ", `"state":"blocked"`, "the drawing engine is down",
		"Next: close its open gates", `"label":"Dependencies"`, `"label":"Awaiting a person"`,
		"✗ **dependencies** - open: One-click install (Platform)", "✓ **no-open-p0-fix**", "Filed since the candidate: ",
		`"state":"awaiting a person"`, `"label":"Awaiting a person","value":2`, "unblock · Draw the flows", "Increments of this release", "Dependencies (other streams)", "stream Platform · needed by The study end to end",
		"Running now", "Blocked",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the overview lacks %q", want)
		}
	}
	if !strings.Contains(page, minor.ID) {
		t.Errorf("the overview does not list %s among what was filed since the candidate", minor.ID)
	}
	if i, j := strings.Index(page, `"id":"`+study+`"`), strings.Index(page, `"id":"`+done+`"`); i < 0 || j < 0 || i > j {
		t.Errorf("the done increment comes before the open one (%d, %d): done ones fold to the bottom", i, j)
	}
}

// A RELEASED RELEASE SHOWS WHAT IT DELIVERED, AND A PLANNED ONE WHETHER IT CAN
// FREEZE - planned, its verdict asks only what freezing
// needs; released, its pane lists its candidates (the promoted one marked) and
// its change log, and no verdict to reach.
func TestAReleasedReleaseShowsWhatItDelivered(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	rel, chg := releases(t, ctx), changesOf(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2026.11"})
	page := func() string {
		t.Helper()
		def, err := releasePane(ctx, rel, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(def.DSLTree)
		return string(raw)
	}
	if p := page(); !strings.Contains(p, "READY TO FREEZE") || !strings.Contains(p, "Next: freeze it") {
		t.Errorf("a planned release with nothing open: want READY TO FREEZE and the freeze as next action")
	}
	fix := file(t, ctx, req, api.FileInput{Title: "A typo", Kind: api.KindFix, On: r.ID})
	if _, err := chg.Add(ctx, api.AddChangeInput{Release: r.ID, Title: "Une coquille corrigée", Domain: "Dossier", Value: api.ValueFix, Text: "x", Requests: []string{fix.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: fix.ID, Outcome: api.PlanDone, By: person, Note: "fixed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	for _, b := range []string{"b-1", "b-2"} {
		if _, err := rel.Deliver(ctx, api.DeliverInput{ID: r.ID, Build: b, By: person}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := rel.RecordGate(ctx, api.RecordGateInput{ID: r.ID, Candidate: "rc2", Gate: api.GateEndToEnd, Passed: true, By: builder}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.SignOff(ctx, api.CandidateInput{ID: r.ID, Candidate: "rc2", By: person}); err != nil {
		t.Fatal(err)
	}
	changeLogConfirmed(t, ctx, r.ID)
	if _, err := rel.Promote(ctx, api.CandidateInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	p := page()
	for _, want := range []string{"RELEASED", "Its candidates", `"title":"rc1"`, `"title":"rc2"`, "promoted: the official version", "(build b-2)", "Its change log", "Une coquille corrigée (correction)"} {
		if !strings.Contains(p, want) {
			t.Errorf("the released release lacks %q", want)
		}
	}
}

// A RELEASE'S NOTE IS ITS INCREMENTS' CHANGES - a
// change entry belongs to the increment whose work made it (its requests): a
// release's note lists exactly the entries of the increments it carries and
// of its own fixes, whatever release the entry was recorded in.
func TestAReleasesNoteIsItsIncrementsChanges(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	chg := changesOf(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2026.10", Date: "2026-10-31", Stream: st.Stream.ID})
	other := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2026.12"})
	carried := increment(t, ctx, "The study end to end").ID
	if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: carried, Stream: st.Stream.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: carried, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	work := file(t, ctx, req, api.FileInput{Title: "Generate the sections", Module: "billing", Kind: api.KindBuild, Increment: carried})
	fix := file(t, ctx, req, api.FileInput{Title: "A typo on the cover", Kind: api.KindFix, On: r.ID})
	elsewhere := file(t, ctx, req, api.FileInput{Title: "Something else", Module: "billing", Kind: api.KindBuild})
	add := func(title, value, rq, release string) {
		t.Helper()
		if _, err := chg.Add(ctx, api.AddChangeInput{Release: release, Title: title, Domain: "Dossier", Value: value, Text: "x", Requests: []string{rq}}); err != nil {
			t.Fatal(err)
		}
	}
	add("Les sections se génèrent", api.ValueMajor, work.ID, other.ID) // recorded elsewhere, still the increment's
	add("Une coquille corrigée", api.ValueFix, fix.ID, r.ID)
	add("Autre chose", api.ValueNotable, elsewhere.ID, r.ID)
	note, err := rel.Note(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(note.Increments) != 1 || len(note.Increments[0].Changes) != 1 || note.Increments[0].Changes[0].Title != "Les sections se génèrent" {
		t.Errorf("the note's increments: %+v; want %s with its one change", note.Increments, carried)
	}
	if len(note.Fixes) != 1 || note.Fixes[0].Title != "Une coquille corrigée" {
		t.Errorf("the note's fixes: %+v; want the release's own fix only", note.Fixes)
	}
}

// A DELIVERY WRITES ITS LETTER - a candidate's letter is
// its « Quoi de neuf »: per increment its title and its major and notable
// changes, then its corrections in one line; kept on the candidate.
func TestADeliveryWritesItsLetter(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	chg := changesOf(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2026.10", Date: "2026-10-31", Stream: st.Stream.ID})
	carried := increment(t, ctx, "L'étude de bout en bout").ID
	if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: carried, Stream: st.Stream.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: carried, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	work := file(t, ctx, req, api.FileInput{Title: "Generate the sections", Module: "billing", Kind: api.KindBuild, Increment: carried})
	fix := file(t, ctx, req, api.FileInput{Title: "A typo", Kind: api.KindFix, On: r.ID})
	for _, c := range []struct{ title, value, rq string }{
		{"Les sections se génèrent seules", api.ValueMajor, work.ID},
		{"Un détail d'affichage", api.ValueImprovement, work.ID},
		{"Une coquille corrigée", api.ValueFix, fix.ID},
	} {
		if _, err := chg.Add(ctx, api.AddChangeInput{Release: r.ID, Title: c.title, Domain: "Dossier", Value: c.value, Text: "x", Requests: []string{c.rq}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	delivered, err := rel.Deliver(ctx, api.DeliverInput{ID: r.ID, Build: "b-1", By: person})
	if err != nil {
		t.Fatal(err)
	}
	// The delivery wrote it; the sign-off approves it, and then it stays.
	letter := delivered.Release.Candidates[0].Letter
	if _, err := rel.SignOff(ctx, api.CandidateInput{ID: r.ID, Candidate: "rc1", By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Letter(ctx, api.CandidateInput{ID: r.ID, Candidate: "rc1", By: builder}); err == nil {
		t.Error("a signed-off candidate's letter was written again: it is what was approved")
	}
	for _, want := range []string{"# Quoi de neuf", "(rc1)", "## L'étude de bout en bout", "- Les sections se génèrent seules", "**Corrections** : 1 correction,"} {
		if !strings.Contains(letter, want) {
			t.Errorf("the letter lacks %q:\n%s", want, letter)
		}
	}
	if strings.Contains(letter, "Un détail d'affichage") {
		t.Errorf("the letter lists an improvement - main points only:\n%s", letter)
	}
}

// A CANDIDATE'S LETTER IS EXPORTED - written to a markdown
// file as it is on the candidate, for a delivery's build to publish as its
// solution's « Quoi de neuf »; saying whether the sign-off approved it; an
// unknown candidate refused.
func TestACandidatesLetterIsExported(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	sm, rel := streams(t, ctx), releases(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2026.10", Date: "2026-10-31", Stream: st.Stream.ID})
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	delivered, err := rel.Deliver(ctx, api.DeliverInput{ID: r.ID, Build: "b-1", By: person})
	if err != nil {
		t.Fatal(err)
	}
	to := filepath.Join(t.TempDir(), "whats-new.md")
	out, err := rel.ExportLetter(ctx, api.ExportLetterInput{ID: r.ID, Candidate: "rc1", To: to})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(to)
	if err != nil {
		t.Fatal(err)
	}
	if want := delivered.Release.Candidates[0].Letter; want == "" || string(got) != want || out.SignedOff {
		t.Errorf("exported %q (signed off %v); want the candidate's letter %q, not yet signed off", got, out.SignedOff, want)
	}
	if _, err := rel.SignOff(ctx, api.CandidateInput{ID: r.ID, Candidate: "rc1", By: person}); err != nil {
		t.Fatal(err)
	}
	if out, err := rel.ExportLetter(ctx, api.ExportLetterInput{ID: r.ID, Candidate: "rc1", To: to}); err != nil || !out.SignedOff {
		t.Errorf("after the sign-off: %+v %v; want it said signed off", out, err)
	}
	if _, err := rel.ExportLetter(ctx, api.ExportLetterInput{ID: r.ID, Candidate: "rc9", To: to}); err == nil {
		t.Error("an unknown candidate's letter was exported")
	}
}
