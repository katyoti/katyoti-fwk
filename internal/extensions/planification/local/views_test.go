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
)

// MANAGEMENT VIEWS, end to end on a real CWM: the pages are generated from the
// objects — nothing is written by hand.

func views(t *testing.T, ctx context.Context) api.ViewService {
	t.Helper()
	svc, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func text(t *testing.T, out api.DocumentsOutput, key string) string {
	t.Helper()
	for _, d := range out.Documents {
		if d.Meta[api.MetaKey] == key {
			return d.Text()
		}
	}
	t.Fatalf("no page %q in %d documents", key, len(out.Documents))
	return ""
}

func contains(t *testing.T, page, what string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(page, w) {
			t.Errorf("%s lacks %q:\n%s", what, w, page)
		}
	}
}

// The change log shows a released release with its done work and verified
// deliverables, and work done outside a release.
func TestTheChangeLogIsGenerated(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0", Name: "Invoices", Date: "2026-10-01"})
	shipped := file(t, ctx, req, api.FileInput{Title: "Send the invoices", Deliverables: []api.Deliverable{{Title: "invoices sent"}}})
	loose := file(t, ctx, req, api.FileInput{Title: "Fix a typo"})
	if _, err := rel.Target(ctx, api.TargetInput{ID: shipped.ID, Release: r.ID}); err != nil {
		t.Fatal(err)
	}
	walk(t, ctx, cwm, shipped.ID, api.PlanReady)
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: shipped.ID})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	f, _, _ := cwmapi.DecodeCapability[api.RequestFields](o, api.CapRequest)
	f.Deliverables[0].Verified = true
	_ = o.SetCapability(api.CapRequest, f)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	walk(t, ctx, cwm, shipped.ID, api.StateDone)
	walk(t, ctx, cwm, loose.ID, api.PlanReady, api.StateDone)
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: intake}); err != nil {
		t.Fatal(err)
	}
	changelogListed(t, ctx, cwm, r.ID)
	if _, err := rel.Release(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	out, err := views(t, ctx).ChangeLog(ctx, api.ChangeLogInput{})
	if err != nil {
		t.Fatal(err)
	}
	page := text(t, out, "changelog")
	contains(t, page, "the change log", "1.4.0 — Invoices (2026-10-01)", "✅ Send the invoices", "  - ✅ invoices sent",
		"Delivered outside a release", "Fix a typo")
	if strings.Index(page, "Send the invoices") > strings.Index(page, "Fix a typo") {
		t.Error("the released work comes after the loose work")
	}
	if n := strings.Count(page, "Send the invoices"); n != 1 {
		t.Errorf("the released request shows %d times, want once (in its release, not outside one)", n)
	}
	// The loose work is dated by its move to done — the journal's record.
	today := time.Now().UTC().Format("2006-01-02")
	eventually(t, "the loose work dated", func() (string, bool) {
		out, err := views(t, ctx).ChangeLog(ctx, api.ChangeLogInput{})
		if err != nil {
			return err.Error(), false
		}
		page := text(t, out, "changelog")
		return page, strings.Contains(page, "- "+today+" — Fix a typo")
	})
	fr, err := views(t, ctx).ChangeLog(ctx, api.ChangeLogInput{Lang: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	contains(t, text(t, fr, "changelog"), "the French change log", "## Versions livrées", "## Livré hors version")
}

// A release's plan page shows its sprints in order, what each holds, the
// network drawn, the critical path and what can start now.
func TestAReleasePlanShowsItsSprints(t *testing.T) {
	t.Parallel()
	ctx, cwm, req, tr, inc := planWorld(t)
	a := fileEst(t, ctx, req, tr, "billing", "Rework billing", inc.ID, 2)
	b := fileEst(t, ctx, req, tr, "shipping", "Route parcels", inc.ID, 1)
	c := fileEst(t, ctx, req, tr, "billing", "Invoice parcels", inc.ID, 1, a, b)
	if _, err := tr.AddStep(ctx, api.AddStepInput{Release: inc.ID, Name: "Parcels billed", Produces: "a release candidate", After: []string{c}}); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.AddStep(ctx, api.AddStepInput{Release: inc.ID, Name: "Billing reworked", After: []string{a}}); err != nil {
		t.Fatal(err)
	}
	walk(t, ctx, cwm, b, api.PlanReady, api.StateDone)
	out, err := views(t, ctx).ReleasePlan(ctx, api.ViewInput{ID: inc.ID})
	if err != nil {
		t.Fatal(err)
	}
	page := text(t, out, "plan/"+inc.ID)
	contains(t, page, "the release plan", "## Sprint 1 — Billing reworked (open)", "## Sprint 2 — Parcels billed (open)",
		"**Produces:** a release candidate", "- ✅ Route parcels _(done)_", "```mermaid", "flowchart LR",
		"Rework billing → Invoice parcels → Parcels billed", "## Can start now\n\n- Rework billing")
	if out.Documents[0].Title != "billing 1.4.0 — Plan" {
		t.Errorf("title = %q", out.Documents[0].Title)
	}
}

// The stream is the whole page set, parents before children, each title
// carrying the stream's name so it is unique in the target space.
func TestTheStreamIsAPageSet(t *testing.T) {
	t.Parallel()
	ctx, _, _, _, inc := planWorld(t)
	out, err := views(t, ctx).Stream(ctx, api.StreamInput{Name: "KaTyoti"})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	seen := map[string]bool{}
	for _, d := range out.Documents {
		key, _ := d.Meta[api.MetaKey].(string)
		parent, _ := d.Meta[api.MetaParent].(string)
		if parent != "" && !seen[parent] {
			t.Errorf("%s comes before its parent %s", key, parent)
		}
		if !strings.HasPrefix(d.Title, "KaTyoti") {
			t.Errorf("%s: title %q does not name the stream", key, d.Title)
		}
		seen[key] = true
		keys = append(keys, key)
	}
	want := []string{"stream", "changelog", "releases", "release/" + inc.ID, "plan/" + inc.ID}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("pages = %v, want %v", keys, want)
	}
	contains(t, text(t, out, "release/"+inc.ID), "the release board", "| `billing` | planned |", "## Plan", "| $50 | 0/0 | 0 |")
	contains(t, text(t, out, "releases"), "the releases index", "| 1.4.0 1.4.0 | `billing` | planned |")
}
