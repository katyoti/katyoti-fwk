// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE LANE CARDS AND THE INCREMENT'S DRAWER SPEAK THE GUI'S LANGUAGE
// : a stream's lane - its scheme and mode chip, its
// next cut, each release card's summary and what is planned for it - and an
// increment's detail - its eyebrow and where it is assigned - are in the
// person's language.
func TestTheLaneCardsAndTheDrawerSpeakTheGUIsLanguage(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2026-11-15", Stream: st.Stream.ID})
	later := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.2.0", Name: "Spring", Date: "2026-12-15", Stream: st.Stream.ID})
	got, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Print invoices", By: person})
	if err != nil {
		t.Fatal(err)
	}
	planned := got.Increment.ID
	if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: planned, Stream: st.Stream.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: planned, In: later.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	read := func(lang string) (api.StreamLane, api.IncrementDetail) {
		t.Helper()
		lanes, err := sm.Lanes(ctx, api.LanesInput{ID: st.Stream.ID, Language: lang})
		if err != nil || len(lanes.Lanes) != 1 || len(lanes.Lanes[0].Releases) != 2 {
			t.Fatalf("lanes: %+v, %v", lanes, err)
		}
		d, err := inc.Detail(ctx, api.IncrementDetailInput{ID: planned, Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		return lanes.Lanes[0], d
	}

	l, d := read("fr")
	for _, c := range []struct{ what, got, want string }{
		{"the scheme and mode chip", l.SchemeMode, "semver · manuel"},
		{"the next cut", l.NextText, "prochaine coupe : à la main"},
		{"the later card's summary", l.Releases[1].Summary, "2026-12-15 · 0/0 faits"},
		{"what is planned for it", l.Releases[1].PlannedText, "+1 prévu"},
		{"the drawer's eyebrow", d.Eyebrow, "Incrément · " + planned},
	} {
		if c.got != c.want {
			t.Errorf("in French, %s is %q; want %q", c.what, c.got, c.want)
		}
	}
	if !strings.HasPrefix(d.Assigned, "Prévu pour 1.2.0 — Spring") || !strings.Contains(d.Assigned, "livré dès que 1.1.0 est sortie") {
		t.Errorf("in French, where it is assigned: %q; want « Prévu pour … - livré dès que 1.1.0 est sortie »", d.Assigned)
	}
}
