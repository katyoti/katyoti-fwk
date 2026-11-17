// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

func streams(t *testing.T, ctx context.Context) api.StreamService {
	t.Helper()
	svc, err := application.UseService[api.StreamService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// A STREAM NUMBERS ITS RELEASES BY ONE SCHEME AND MAKES THEM BY ONE MODE — a
// stream is created with its scheme and mode; a release joins it only with a
// version in the scheme (semver, calendar with rc, date), at Plan or after;
// a second stream replaces the first; an ended release is exempt; an
// increment names its stream; a module in a stream is a lint warning, a
// release off its scheme too.
func TestAStreamHoldsItsReleasesByItsVersionScheme(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	sm, rel := streams(t, ctx), releases(t, ctx)
	create := func(name, scheme, mode, every string) api.Stream {
		t.Helper()
		out, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: name,
			StreamFields: api.StreamFields{VersionScheme: scheme, ReleaseMode: mode, Every: every}})
		if err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
		return out.Stream
	}
	pkg := create("Packages", api.SchemeSemver, api.ModeManual, "")
	site := create("Site", api.SchemeCalendar, api.ModePeriodic, "14d")
	blog := create("Blog", api.SchemeDate, api.ModeOnChange, "")
	if got, _ := sm.Get(ctx, api.GetInput{ID: site.ID}); got.Stream.ReleaseMode != api.ModePeriodic || got.Stream.Every != "14d" {
		t.Errorf("the site's mode: %+v; want periodic every 14d", got.Stream.StreamFields)
	}
	// Read back, it says when it next cuts.
	if got, _ := sm.Get(ctx, api.GetInput{ID: site.ID}); got.Stream.Cut.Mode != api.ModePeriodic || got.Stream.Cut.At == "" || !strings.Contains(got.Stream.Cut.Text, "every 14d") {
		t.Errorf("the site's next cut: %+v; want periodic every 14d, with its day", got.Stream.Cut)
	}
	if got, _ := sm.Get(ctx, api.GetInput{ID: pkg.ID}); got.Stream.Cut.At != "" || !strings.Contains(got.Stream.Cut.Text, "a person cuts it") {
		t.Errorf("the packages' next cut: %+v; want none, a person cutting it", got.Stream.Cut)
	}
	if _, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "No period",
		StreamFields: api.StreamFields{ReleaseMode: api.ModePeriodic}}); err == nil {
		t.Error("a periodic stream with no period was created")
	}

	// The version follows the scheme, at Plan.
	for _, c := range []struct {
		stream, version string
		ok              bool
	}{
		{pkg.ID, "1.4.0", true}, {pkg.ID, "1.005", true}, {pkg.ID, "2.0.0-rc1", true}, {pkg.ID, "2026.10", true}, {pkg.ID, "v1", false},
		{site.ID, "2026.11", true}, {site.ID, "2026.12-rc2", true}, {site.ID, "2026.13", false}, {site.ID, "1.4.0", false},
		{blog.ID, "2026-10-10", true}, {blog.ID, "2026.10", false},
	} {
		out, err := rel.Plan(ctx, api.PlanInput{Module: "billing", Version: c.version, Stream: c.stream})
		if c.ok && (err != nil || out.Release.Stream != c.stream) {
			t.Errorf("Plan %s in %s: %v (stream %q); want it in", c.version, c.stream, err, out.Release.Stream)
		}
		if err == nil {
			// Dropped at once: a stream keeps two open releases only (rule
			// ) - this loop checks the scheme.
			if _, err := rel.Drop(ctx, api.MoveInput{ID: out.Release.ID, By: person}); err != nil {
				t.Fatal(err)
			}
		}
		if !c.ok && err == nil {
			t.Errorf("Plan %s in %s: planned; want it refused - off the scheme", c.version, c.stream)
		}
	}

	// After Plan: ReleaseIn checks the scheme; a second stream replaces the first.
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2027.01"})
	if _, err := sm.ReleaseIn(ctx, api.StreamInInput{ID: r.ID, Stream: pkg.ID}); err != nil {
		t.Fatalf("2027.01 in a semver stream (major.minor): %v", err)
	}
	if _, err := sm.ReleaseIn(ctx, api.StreamInInput{ID: r.ID, Stream: blog.ID}); err == nil {
		t.Error("2027.01 joined the date stream")
	}
	got, err := sm.ReleaseIn(ctx, api.StreamInInput{ID: r.ID, Stream: site.ID})
	if err != nil {
		t.Fatal(err)
	}
	o, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	var in []string
	for _, e := range o.Object.Edges {
		if e.Kind == api.EdgeInStream {
			in = append(in, e.To)
		}
	}
	if got.Release.Stream != site.ID || !slices.Equal(in, []string{site.ID}) {
		t.Errorf("after a second stream: in %v; want %s only - one stream", in, site.ID)
	}

	// An ended release is exempt: its version is history.
	old := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "sessions-1"})
	if err := move(ctx, cwm, old.ID, api.ReleaseDropped, person); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.ReleaseIn(ctx, api.StreamInInput{ID: old.ID, Stream: pkg.ID}); err != nil {
		t.Errorf("a dropped release named sessions-1 into a semver stream: %v; want it in - ended releases are exempt", err)
	}

	// An increment names its stream.
	inc := increment(t, ctx, "Invoices reach their customers by mail")
	if out, err := sm.AimIncrement(ctx, api.StreamInInput{ID: inc.ID, Stream: pkg.ID}); err != nil || out.Increment.Stream != pkg.ID {
		t.Errorf("AimIncrement: %v (stream %q); want %s", err, out.Increment.Stream, pkg.ID)
	}

	// A module in a stream is a lint warning.
	mod, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	m := mod.Object
	m.Edges = append(m.Edges, cwmapi.ObjectEdge{Kind: api.EdgeInStream, To: pkg.ID})
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: m}); err != nil {
		t.Fatal(err)
	}
	so, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	lint, err := sm.Lint(ctx, api.HookInput{Object: &so.Object})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(lint.Findings, func(f api.LintFinding) bool { return f.Rule == "module-in-stream" }) {
		t.Errorf("a module in a stream: findings %+v; want module-in-stream", lint.Findings)
	}

	// A release off its scheme (the scheme changed under it) is a lint warning.
	if _, err := sm.Set(ctx, api.SetStreamInput{ID: site.ID, StreamFields: api.StreamFields{VersionScheme: api.SchemeDate}}); err != nil {
		t.Fatal(err)
	}
	ro, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	rl, err := rel.Lint(ctx, api.HookInput{Object: &ro.Object})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(rl.Findings, func(f api.LintFinding) bool {
		return f.Rule == "version-off-scheme" && strings.Contains(f.Message, "2027.01")
	}) {
		t.Errorf("2027.01 in a stream now by date: findings %+v; want version-off-scheme", rl.Findings)
	}
}

// : an increment not done by its release's date moves to
// its stream's next release - planned for it, saying why - when the stream's
// daily timer runs; every stream carries that timer, and it never cuts.
func TestAnIncrementNotDoneByItsReleasesDateMovesOn(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	s, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Packages",
		StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	o, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: s.Stream.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fmt.Sprint(o.Object.Capabilities["timed"]), "release-due") {
		t.Fatalf("a manual stream carries no daily timer: %v", o.Object.Capabilities["timed"])
	}
	past := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	later := time.Now().AddDate(0, 1, 0).Format("2006-01-02")
	due := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0", Date: past, Stream: s.Stream.ID})
	next := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.5.0", Date: later, Stream: s.Stream.ID})
	late := increment(t, ctx, "Invoices reach their customers by mail")
	if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: late.ID, Stream: s.Stream.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: late.ID, In: due.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := sm.Cut(ctx, api.CutInput{ID: s.Stream.ID, Timer: "release-due"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Release != nil || len(got.Increments) != 1 || got.Increments[0] != late.ID {
		t.Fatalf("the daily timer: %+v; want %s moved and no release made", got, late.ID)
	}
	moved, err := inc.Get(ctx, api.GetInput{ID: late.ID})
	if err != nil {
		t.Fatal(err)
	}
	m := moved.Increment
	if m.ShipsIn != "" || m.PlannedFor != next.ID || !strings.Contains(m.Moved, "not done by") || !strings.Contains(m.Moved, past) {
		t.Fatalf("the increment after its release's date: ships in %q, planned for %q, moved %q; want planned for %s saying why", m.ShipsIn, m.PlannedFor, m.Moved, next.ID)
	}
	if again, _ := sm.Cut(ctx, api.CutInput{ID: s.Stream.ID, Timer: "release-due"}); len(again.Increments) != 0 {
		t.Errorf("the timer moved again: %+v", again)
	}
}
