// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"
	"testing/fstest"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// The Reviews overview: a row per reviewable module, a cell per dimension of
// its review types — the worst current rating, its trend against the previous
// review of the type, its age — and its open findings; the KPIs count the
// modules reviewed and the critical ratings; given a module, only it and the
// modules under it.
func TestTheOverviewShowsEveryModuleByDimension(t *testing.T) {
	w := setupWith(t, fstest.MapFS{
		"review-types/security.yaml": {Data: []byte("name: security\ntitle: Security\ndimension: security\nrunner: {skill: x}\n")},
		"review-types/threats.yaml":  {Data: []byte("name: threats\ntitle: Threats\ndimension: security\nrunner: {skill: x}\n")},
		"review-types/ux.yaml":       {Data: []byte("name: ux\ntitle: Usability\ndimension: usability\nrunner: {skill: x}\n")},
	})
	cwmapp.Module(t, w.ctx, w.cwm, "n")
	recent := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	older := time.Now().UTC().Add(-240 * time.Hour).Format(time.RFC3339)
	mk := func(id, typ, at, rating string) {
		w.create(t, cwmapi.Object{ID: id, Type: api.ReviewType, Parent: "m", Name: id,
			Fields: map[string]any{"type": typ, "reviewed_at": at, "rating": rating}})
	}
	mk("m.s1", "security", older, "good")
	mk("m.s2", "security", recent, "critical")
	mk("m.t1", "threats", recent, "good")
	mk("m.u1", "ux", older, "good")
	w.create(t, cwmapi.Object{ID: "m.f1", Type: api.FindingType, Parent: "m", Name: "open one",
		Fields: map[string]any{"severity": "high", "found_by": "security"}, Edges: []cwmapi.ObjectEdge{{Kind: api.EdgeFoundIn, To: "m.s2"}, {Kind: api.EdgeSeenIn, To: "m.s2"}}})

	all, err := w.svc.Overview(w.ctx, api.ObjectInput{ID: ""})
	if err != nil {
		t.Fatal(err)
	}
	page := asJSON(t, all)
	for _, want := range []string{`"value":"1 / 2"`, `"label":"Critical ratings","value":1`, `"text":"critical ▼ · 2 d"`,
		`"text":"good = · 10 d"`, `"text":"1 high"`, `"text":"not reviewed"`, `"navigate":{"object":"m.s2"`, `"content":"usability"`} {
		if !strings.Contains(page, want) {
			t.Errorf("overview lacks %s:\n%s", want, page)
		}
	}
	one, err := w.svc.Overview(w.ctx, api.ObjectInput{ID: "n"})
	if err != nil {
		t.Fatal(err)
	}
	if page := asJSON(t, one); strings.Contains(page, `"object":"m"`) || !strings.Contains(page, `"value":"0 / 1"`) {
		t.Errorf("a module's overview holds it and those under it only:\n%s", page)
	}
}

// The Reviews overview is placed where a person reads it: a tab of the Global
// strip and of every module's pane, sourced from the reviews service — the
// page TestTheOverviewShowsEveryModuleByDimension proves.
func TestTheOverviewIsInTheGlobalStripAndOnEveryModule(t *testing.T) {
	w := setup(t)
	global, err := w.pane.GlobalPanels(w.ctx, paneapi.GlobalPanelsInput{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range global.Panels {
		if p.Source == "ktw-test:reviews/overview" {
			found = true
		}
	}
	if !found {
		t.Errorf("the Global strip has no Reviews overview: %+v", global.Panels)
	}
	if p, ok := w.panels(t, "m")["tab:reviews-overview"]; !ok || p.Source != "ktw-test:reviews/overview" {
		t.Errorf("a module's pane has the Reviews overview: %+v (%v)", p, w.panels(t, "m"))
	}
}

// : a module is listed by the module level
// capability its type carries, whatever the type's name.
func TestTheOverviewListsAModuleOfAnotherTypeName(t *testing.T) {
	w := setupWith(t, fstest.MapFS{
		"object-types/software-module.yaml": {Data: []byte("name: software-module\nlabel: Software module\ntree: node\ncapabilities: [module]\n")},
	})
	if _, err := w.cwm.ObjectCreate(w.ctx, cwmapi.ObjectCreateInput{Module: "sw", Object: cwmapi.Object{ID: "sw", Type: "software-module", Name: "Software"}}); err != nil {
		t.Fatal(err)
	}
	all, err := w.svc.Overview(w.ctx, api.ObjectInput{ID: ""})
	if err != nil {
		t.Fatal(err)
	}
	if page := asJSON(t, all); !strings.Contains(page, `"object":"sw"`) {
		t.Errorf("the overview lacks the software module sw:\n%s", page)
	}
}
