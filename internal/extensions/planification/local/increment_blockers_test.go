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
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// WHAT BLOCKS AN INCREMENT - `increments blockers <id>`
// follows the increment's waits through its dependencies, not one hop, down
// to what holds them at the root - here a person's answer in another
// increment - and lists each root once, under its kind, with who acts, its
// age and the work of the increment it holds.

// incrementFeature is a feature aimed at a new increment, its build filed.
func incrementFeature(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, req api.RequestService, incTitle, title string) (api.Increment, string) {
	t.Helper()
	in := increment(t, ctx, incTitle)
	f := want(t, ctx, cwm, "feature", title)
	accepted(t, ctx, cwm, f)
	if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	return in, f
}

func blockersOf(t *testing.T, ctx context.Context, id string) api.IncrementBlockersOutput {
	t.Helper()
	out, err := increments(t, ctx).Blockers(ctx, api.GetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// groupOf is the group of a kind; nil when none.
func groupOf(out api.IncrementBlockersOutput, kind string) *api.IncrementBlockerGroup {
	for i := range out.Groups {
		if out.Groups[i].Kind == kind {
			return &out.Groups[i]
		}
	}
	return nil
}

func TestAnIncrementsBlockersFollowItsDependenciesToThePersonsAnswer(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	_, numbering := incrementFeature(t, ctx, cwm, req, "Invoices are numbered", "Number the invoices")
	personStep(t, ctx, numbering, "give-numbering", api.InteractionProvide)
	mail, mailing := incrementFeature(t, ctx, cwm, req, "Invoices reach their customers by mail", "Mail the numbered invoices")
	if _, err := req.Depend(ctx, api.DependInput{ID: mailing, On: numbering}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, mailing, api.StepIntake).ID, session, schedapi.Result{Summary: "a build"})

	// One hop stops at the increment's edge: the check calls it outside.
	if b := blockerOf(t, check(t, ctx, api.PlanningCheckInput{Increment: mail.ID}), mailing); b.Kind != api.BlockerOutside {
		t.Fatalf("the check of %s: %s (%s); want the one-hop reading outside", mail.ID, b.Kind, b.Reason)
	}
	out := blockersOf(t, ctx, mail.ID)
	if out.Increment != mail.ID {
		t.Errorf("increment %q; want %s", out.Increment, mail.ID)
	}
	g := groupOf(out, api.BlockerAnswer)
	if g == nil || len(g.Blockers) != 1 {
		t.Fatalf("the person's answer: %+v; want the answer %s waits on (groups %+v)", g, numbering, out.Groups)
	}
	if g.Class != "K1" || g.Title != "the person's answer" {
		t.Errorf("the group is %s %q; want K1 « the person's answer »", g.Class, g.Title)
	}
	b := g.Blockers[0]
	wantKind(t, b.Blocker, api.BlockerAnswer, api.ActorPerson, "person")
	if b.Work != numbering || b.Todo == "" {
		t.Errorf("the root is %s (to-do %q); want the answer step of %s", b.Work, b.Todo, numbering)
	}
	if b.Age == "" {
		t.Errorf("the answer has no age (since %q)", b.Since)
	}
	if !slices.Equal(b.Holds, []string{mailing}) {
		t.Errorf("the answer holds %v; want the increment's %s", b.Holds, mailing)
	}
	if !strings.HasPrefix(b.Outside, numbering) {
		t.Errorf("the answer lies in %q; want where %s is scheduled", b.Outside, numbering)
	}
	// Each root once: the held work is not repeated as a group of its own.
	if out.Total != 1 || out.Counts[api.BlockerAnswer] != 1 || groupOf(out, api.BlockerOutside) != nil || groupOf(out, api.BlockerHeld) != nil {
		t.Errorf("total %d, counts %v, groups %+v; want the one answer only", out.Total, out.Counts, out.Groups)
	}
	// A second piece of the increment held by the same answer: still one
	// root, now holding both.
	reminding := want(t, ctx, cwm, "feature", "Mail the payment reminders")
	accepted(t, ctx, cwm, reminding)
	if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: reminding, Increment: mail.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: reminding, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Depend(ctx, api.DependInput{ID: reminding, On: numbering}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, reminding, api.StepIntake).ID, session, schedapi.Result{Summary: "a build"})
	both := []string{mailing, reminding}
	slices.Sort(both)
	out = blockersOf(t, ctx, mail.ID)
	if g := groupOf(out, api.BlockerAnswer); out.Total != 1 || g == nil || len(g.Blockers) != 1 || !slices.Equal(g.Blockers[0].Holds, both) {
		t.Errorf("total %d, groups %+v; want the one answer holding %v", out.Total, out.Groups, both)
	}
	if _, err := increments(t, ctx).Blockers(ctx, api.GetInput{ID: mailing}); err == nil {
		t.Errorf("the blockers of %s, a feature: accepted; want an increment", mailing)
	}
}

// The incidents' list on the increment's work joins its incidents, with how
// many sweeps saw each; the repairs it keeps on that work - a week - are what
// the reconciler already repaired, the tab's badge their total.
func TestAnIncrementsBlockersListTheOpenIncidentsOnItsWork(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	in, f := incrementFeature(t, ctx, cwm, req, "Invoices reach their customers by mail", "Mail the invoices")
	other := file(t, ctx, req, api.FileInput{Title: "Round the totals", Module: "billing"})
	now := time.Now().UTC().Format(time.RFC3339Nano)
	list := []api.IncidentRecord{
		{Incident: api.Incident{Key: "orphan-wait:" + f, Check: "orphan-wait", Work: f, Reason: "it waits on nothing that moves", Actor: "the person: remove the wait"}, Since: now, Seen: now, Sweeps: 3},
		{Incident: api.Incident{Key: "orphan-wait:" + other.ID, Check: "orphan-wait", Work: other.ID, Reason: "not the increment's"}, Since: now, Seen: now, Sweeps: 1},
		{Incident: api.Incident{Key: "closed:" + f, Check: "closed", Work: f, Reason: "closed already"}, Since: now, Seen: now, Sweeps: 2, Closed: now},
	}
	week := time.Now().Add(-8 * 24 * time.Hour).UTC().Format(time.RFC3339)
	repaired := []api.RepairDone{
		{Repair: api.Repair{Repair: api.RepairEndPlan, Work: f, Reason: "its lifecycle is done while its plan is still ready", Action: "end its plan done", Applied: true}, At: time.Now().UTC().Format(time.RFC3339)},
		{Repair: api.Repair{Repair: api.RepairEndPlan, Work: other.ID, Reason: "not the increment's", Applied: true}, At: time.Now().UTC().Format(time.RFC3339)},
		{Repair: api.Repair{Repair: api.RepairPutStep, Work: f, Reason: "older than a week", Applied: true}, At: week},
	}
	raw, _ := json.Marshal(map[string]any{"incidents": list, "repaired": repaired})
	path := filepath.Join(application.EnvOf(projectRootOf(t, ctx, cwm, f)).HomeDir(), "planification", "incidents.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	g := groupOf(blockersOf(t, ctx, in.ID), api.BlockerIncident)
	if g == nil || len(g.Blockers) != 1 {
		t.Fatalf("the incidents: %+v; want the one open record on %s", g, f)
	}
	b := g.Blockers[0]
	if b.Work != f || b.Incident != "orphan-wait:"+f || b.Sweeps != 3 || b.ActorKind != api.ActorPerson || b.Since != now {
		t.Errorf("the incident: %+v; want its record on %s, seen by 3 sweeps", b, f)
	}
	out := blockersOf(t, ctx, in.ID)
	if len(out.Repaired) != 1 || out.Repaired[0].Work != f || out.Repaired[0].Action != "end its plan done" || out.Repaired[0].Title == "" {
		t.Errorf("the repairs: %+v; want the one of this week on %s, with its title", out.Repaired, f)
	}
	c, err := increments(t, ctx).BlockerCount(ctx, api.IncrementPaneInput{ID: in.ID, Tab: "blockers"})
	if err != nil || c.Count != out.Total || c.Count == 0 || c.Color != "red" {
		t.Errorf("the Blockers tab's badge: %+v, %v; want %d in red", c, err, out.Total)
	}
}

// An open record of the incidents' list on the work of a K6 root joins that
// root: the root - here a feature of the increment
// nobody works on - is listed once, carrying the record's key and its sweeps,
// never a second time as an incident of its own.
func TestAnOpenIncidentOnAK6RootJoinsThatRoot(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	in := increment(t, ctx, "Invoices are numbered")
	numbering := want(t, ctx, cwm, "feature", "Number the invoices")
	accepted(t, ctx, cwm, numbering)
	if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: numbering, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	before := groupOf(blockersOf(t, ctx, in.ID), api.BlockerIncident)
	if before == nil || len(before.Blockers) != 1 || before.Blockers[0].Work != numbering || before.Blockers[0].Incident != "" {
		t.Fatalf("the K6 root before any record: %+v; want the one root %s, nothing open working on it", before, numbering)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	key := "orphan-want:" + numbering
	list := []api.IncidentRecord{{Incident: api.Incident{Key: key, Check: "orphan-want", Work: numbering, Reason: "nothing open works on it", Actor: "the person: plan its work"}, Since: now, Seen: now, Sweeps: 4}}
	raw, _ := json.Marshal(map[string]any{"incidents": list})
	path := filepath.Join(application.EnvOf(projectRootOf(t, ctx, cwm, numbering)).HomeDir(), "planification", "incidents.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	out := blockersOf(t, ctx, in.ID)
	g := groupOf(out, api.BlockerIncident)
	if g == nil || len(g.Blockers) != 1 {
		t.Fatalf("the K6 group: %+v; want the one root %s, the record joined to it", g, numbering)
	}
	b := g.Blockers[0]
	if b.Work != numbering || b.Incident != key || b.Sweeps != 4 || b.Reason != before.Blockers[0].Reason {
		t.Errorf("the K6 root: %+v; want the root of %s (%q) carrying the record %s seen by 4 sweeps", b, numbering, before.Blockers[0].Reason, key)
	}
	if out.Total != 1 || out.Counts[api.BlockerIncident] != 1 {
		t.Errorf("total %d, counts %v; want the one root only", out.Total, out.Counts)
	}
}
