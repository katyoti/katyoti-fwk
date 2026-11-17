// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A STREAM RELEASES BY ITS MODE — a periodic stream carries its timer and a
// cut makes its next release from the increments delivered since the last:
// the scheme's next version (1.005 → 1.006; a breaking one → 2.000; the
// calendar's month taken → rc1), its level computed, frozen; nothing
// delivered, nothing made. An on-change stream cuts when an increment is
// delivered, once per minimum interval; a manual one never by itself.
func TestAStreamReleasesByItsMode(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	stream := func(name string, f api.StreamFields) api.Stream {
		t.Helper()
		out, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: name, StreamFields: f})
		if err != nil {
			t.Fatal(err)
		}
		return out.Stream
	}
	delivered := func(title string, stream string, breaking bool) string {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title, Breaking: breaking})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: out.Increment.ID, Stream: stream}); err != nil {
			t.Fatal(err)
		}
		if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: out.Increment.ID, By: person, Force: true, Note: forcedNote}); err != nil {
			t.Fatal(err)
		}
		return out.Increment.ID
	}
	cut := func(stream string) api.CutOutput {
		t.Helper()
		out, err := sm.Cut(ctx, api.CutInput{ID: stream})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	// Periodic: its timer, then a cut holding what was delivered.
	pkg := stream("Packages", api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModePeriodic, Every: "14d"})
	o, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	if timed, ok, _ := cwmapi.DecodeCapability[eventsapi.TimedFields](o.Object, eventsapi.CapTimed); !ok || len(timed.Timers) != 2 || timed.Timers[0].Every != "336h" || timed.Timers[1].Name != "release-due" {
		t.Errorf("a periodic stream every 14d: timers %+v; want one every 336h and its daily release-due", timed.Timers)
	}
	older := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.005", Stream: pkg.ID})
	a, b := delivered("Invoices by mail", pkg.ID, false), delivered("Reminders", pkg.ID, false)
	open, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Not yet"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: open.Increment.ID, Stream: pkg.ID}); err != nil {
		t.Fatal(err)
	}
	got := cut(pkg.ID)
	if got.Release == nil {
		t.Fatalf("a cut with two delivered increments made nothing: %s", got.Note)
	}
	if r := got.Release; r.Version != "1.006" || r.Level != api.LevelMinor || r.Stream != pkg.ID || r.State != api.ReleaseFrozen {
		t.Errorf("the release cut: %s %s in %s, %s (%s); want 1.006 minor in %s, frozen", r.Version, r.Level, r.Stream, r.State, got.Note, pkg.ID)
	}
	slices.Sort(got.Increments)
	if want := []string{a, b}; !slices.Equal(got.Increments, want) {
		t.Errorf("it holds %v; want the delivered %v only", got.Increments, want)
	}
	if again := cut(pkg.ID); again.Release != nil {
		t.Errorf("a second cut with nothing delivered made %s", again.Release.ID)
	}
	// Two open already (1.005 and the cut 1.006): one goes before the next
	// cut (rule).
	if _, err := rel.Drop(ctx, api.MoveInput{ID: older.ID, By: person, Note: "not shipped"}); err != nil {
		t.Fatal(err)
	}
	delivered("New API", pkg.ID, true)
	if r := cut(pkg.ID).Release; r == nil || r.Version != "2.000" || r.Level != api.LevelMajor {
		t.Errorf("a breaking increment: %+v; want 2.000, major", r)
	}

	// Calendar: the month taken, its rc.
	af := stream("ArchiFactory", api.StreamFields{VersionScheme: api.SchemeCalendar, ReleaseMode: api.ModeManual})
	month := time.Now().Format("2006.01")
	taken := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: month, Stream: af.ID})
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: taken.ID, By: person}); err != nil {
		t.Fatal(err) // frozen: a delivered increment does not ship in it
	}
	delivered("Dossier template", af.ID, false)
	if r := cut(af.ID).Release; r == nil || r.Version != month+"-rc1" {
		t.Errorf("the calendar's month taken: %+v; want %s-rc1", r, month)
	}

	// Manual: delivering cuts nothing.
	delivered("Another template", af.ID, false)
	if rs, _ := rel.List(ctx, api.ListInput{Module: "billing"}); slices.ContainsFunc(rs.Releases, func(r api.Release) bool { return r.Version == month+"-rc2" }) {
		t.Error("a manual stream cut a release by itself")
	}

	// On change: delivered, cut at once; a second within the interval waits.
	blog := stream("Blog", api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeOnChange, MinInterval: "2d"})
	first := delivered("An article", blog.ID, false)
	one, err := inc.Get(ctx, api.GetInput{ID: first})
	if err != nil {
		t.Fatal(err)
	}
	if one.Increment.ShipsIn == "" {
		t.Fatal("an on-change stream's delivered increment is in no release")
	}
	second := delivered("Another article", blog.ID, false)
	if two, _ := inc.Get(ctx, api.GetInput{ID: second}); two.Increment.ShipsIn != "" {
		t.Errorf("a second delivery within the minimum interval was cut at once (into %s)", two.Increment.ShipsIn)
	}
}

// ONLY THE NEXT RELEASE TAKES INCREMENTS (decision) - a
// person assigns an increment to its stream's next planned release only (the
// earliest by date); a later one is refused. A done increment assigned to none
// rides along: it is listed on the next release, not assigned, and the freeze
// takes it in; one assigned stays where it is. A periodic stream's cut takes
// its done increments as before.
func TestOnlyTheNextReleaseTakesIncrements(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	open := func(title string) string {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: out.Increment.ID, Stream: st.Stream.ID}); err != nil {
			t.Fatal(err)
		}
		return out.Increment.ID
	}
	later := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2.0.0", Date: "2027-02-01", Stream: st.Stream.ID})
	next := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: st.Stream.ID})
	assigned := open("Invoices by mail")
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: assigned, In: later.ID}); err == nil {
		t.Error("an increment was assigned to a release that is not its stream's next")
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: assigned, In: next.ID}); err != nil {
		t.Fatalf("the stream's next release refused an increment: %v", err)
	}
	along := open("Reminders")
	for _, id := range []string{assigned, along} {
		if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: id, By: person, Force: true, Note: forcedNote}); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := inc.Get(ctx, api.GetInput{ID: along}); got.Increment.ShipsIn != "" {
		t.Errorf("a done increment assigned to none was shipped in %s; want it riding along, not assigned", got.Increment.ShipsIn)
	}
	def, err := releasePane(ctx, rel, next.ID)
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(def.DSLTree); !strings.Contains(string(raw), along) || !strings.Contains(string(raw), `"text":"Riding along"`) {
		t.Errorf("the next release's pane does not list %s riding along: %s", along, raw)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: next.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if got, _ := inc.Get(ctx, api.GetInput{ID: along}); got.Increment.ShipsIn != next.ID {
		t.Errorf("after the freeze %s ships in %q; want the frozen release %s to have taken it in", along, got.Increment.ShipsIn, next.ID)
	}
	if got, _ := inc.Get(ctx, api.GetInput{ID: assigned}); got.Increment.ShipsIn != next.ID {
		t.Errorf("the assigned increment moved to %q", got.Increment.ShipsIn)
	}
}

// A FROZEN RELEASE TAKES FIXES ONLY (decision) - after the
// freeze no increment is assigned to it (the next release takes it); a fix
// filed on one of its increments reopens that increment until the fix is done;
// a fix belonging to none is filed on the release itself.
func TestAFrozenReleaseTakesFixesOnly(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: st.Stream.ID})
	carried := increment(t, ctx, "Invoices by mail").ID
	if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: carried, Stream: st.Stream.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: carried, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: carried, By: person, Force: true, Note: forcedNote}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	late := increment(t, ctx, "Dunning").ID
	if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: late, Stream: st.Stream.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: late, In: r.ID}); err == nil {
		t.Error("an increment was assigned to a frozen release")
	}
	fix := file(t, ctx, req, api.FileInput{Title: "The mail lacks its attachment", Module: "billing", Kind: api.KindFix, Increment: carried})
	if got, _ := inc.Get(ctx, api.GetInput{ID: carried}); got.Increment.State != api.IncrementOpen {
		t.Errorf("a fix filed on %s, carried by the frozen release: %s; want it reopened", carried, got.Increment.State)
	}
	if fix.ID == "" {
		t.Fatal("the fix was not filed")
	}
	own := file(t, ctx, req, api.FileInput{Title: "The release notes miss a line", Kind: api.KindFix, On: r.ID})
	if own.ID != r.ID || own.Kind != api.KindFix {
		t.Errorf("a fix filed on the release itself: %s (%s); want a fix carried by %s", own.ID, own.Kind, r.ID)
	}
}

// THE FREEZE AND THE FIXES, AS RQ-201'S REVIEW ASKED -
// freezing a later release takes nothing riding along (only the next one
// does, once its freeze passed); a fix on a done increment riding along
// reopens it.
func TestTheFreezeTakesWhatRidesAlongIntoTheNextReleaseOnly(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	later := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2.0.0", Date: "2027-02-01", Stream: st.Stream.ID})
	plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: st.Stream.ID})
	done := func(title string) string {
		t.Helper()
		id := increment(t, ctx, title).ID
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: id, Stream: st.Stream.ID}); err != nil {
			t.Fatal(err)
		}
		if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: id, By: person, Force: true, Note: forcedNote}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	along := done("Reminders")
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: later.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if got, _ := inc.Get(ctx, api.GetInput{ID: along}); got.Increment.ShipsIn != "" {
		t.Errorf("freezing a later release took %s in (%s); want it still riding along in the next one", along, got.Increment.ShipsIn)
	}
	// A fix on it, riding along, reopens it.
	file(t, ctx, req, api.FileInput{Title: "Reminders lack the due date", Module: "billing", Kind: api.KindFix, Increment: along})
	if got, _ := inc.Get(ctx, api.GetInput{ID: along}); got.Increment.State != api.IncrementOpen {
		t.Errorf("a fix on %s riding along: %s; want it reopened", along, got.Increment.State)
	}
}
