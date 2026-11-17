// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/features/api"
	featureslocal "github.com/katyoti/katyoti-fwk/internal/extensions/features/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

func pack() application.Extension {
	return cwmapp.Pack(fstest.MapFS{
		"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\nbadges: [{field: state, colors: {idea: gray, proposed: yellow}}]\n")},
		"capabilities/feature.yaml":   {Data: []byte("name: feature\n")},
		"workflows/ladder.yaml": {Data: []byte("name: ladder\ninitial: idea\nstates: [{name: idea}, {name: proposed}]\n" +
			"transitions: [{name: propose, from: [idea], to: proposed}]\n")},
		"object-types/goal.yaml":    {Data: []byte("name: goal\ncapabilities: [lifecycle]\nworkflow: ladder\n")},
		"object-types/feature.yaml": {Data: []byte("name: feature\ncapabilities: [feature, lifecycle]\nworkflow: ladder\n")},
		"edge-types/serves.yaml":    {Data: []byte("name: serves\nfrom: any\nto: any\ncardinality: M:N\ntarget: object\nobject_storage: edges\n")},
	})
}

// THE FEATURES TAB: every feature of the module's subtree — a sub-feature
// under a feature, another module's left out — highest cost of delay first,
// each with its state, its cost of delay and the goals it serves; the state
// filter re-reads it. Locally and with CWM in another process, as in the
// binary hosting it.
func TestFeaturesBoardListsTheWholeSubtree(t *testing.T) {
	ctx, cwm := cwmapp.New(t, featureslocal.Extension(), pack())
	boardScenario(t, ctx, cwm)
}

func TestFeaturesBoardOverTheBus(t *testing.T) {
	ctx, cwm := cwmapp.RemoteHosting(t, []application.Extension{pack()}, featureslocal.Extension())
	boardScenario(t, ctx, cwm)
}

func boardScenario(t *testing.T, ctx context.Context, cwm cwmapi.CWMService) {
	t.Helper()
	cwmapp.Module(t, ctx, cwm, "m")
	cwmapp.Module(t, ctx, cwm, "n")
	mk := func(module string, o cwmapi.Object) {
		t.Helper()
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	mk("m", cwmapi.Object{ID: "m.g", Type: "goal", Name: "Ship it", Parent: "m"})
	mk("m", cwmapi.Object{ID: "m.f1", Type: "feature", Name: "Export", Parent: "m",
		Capabilities: map[string]map[string]any{"feature": {"cod": 3}}, Edges: []cwmapi.ObjectEdge{{Kind: "serves", To: "m.g"}}})
	mk("m", cwmapi.Object{ID: "m.f2", Type: "feature", Name: "Export to PDF", Parent: "m.f1",
		Capabilities: map[string]map[string]any{"feature": {"cod": 8}}})
	mk("n", cwmapi.Object{ID: "n.f9", Type: "feature", Name: "Elsewhere", Parent: "n"})
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "m.f1", To: "proposed", By: cwmapi.Actor{ID: "p", Kind: "person"}}); err != nil {
		t.Fatal(err)
	}
	svc, err := application.UseService[api.FeaturesService](ctx)
	if err != nil {
		t.Fatal(err)
	}

	page := board(t, ctx, svc, api.BoardInput{ID: "m"})
	for _, want := range []string{`"text":"CoD 8"`, `"text":"CoD 3"`, `"text":"proposed"`, `"text":"idea"`, `"text":"Ship it"`,
		`"navigate":{"object":"m.g"}`, `"text":"serves no goal"`, `"create":{"type":"feature"}`, `"badge":2`, `"↳ Export"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the board lacks %s", want)
		}
	}
	if strings.Contains(page, "Elsewhere") {
		t.Error("another module's feature is listed")
	}
	if strings.Index(page, `"object":"m.f2"`) > strings.Index(page, `"object":"m.f1"`) {
		t.Error("the highest cost of delay does not come first")
	}
	page = board(t, ctx, svc, api.BoardInput{ID: "m", State: "proposed", Language: "fr"})
	if !strings.Contains(page, `"badge":1`) || !strings.Contains(page, "Fonctionnalités") || strings.Contains(page, `"object":"m.f2"`) {
		t.Errorf("the state filter in French: %s", page)
	}
}

func board(t *testing.T, ctx context.Context, svc api.FeaturesService, in api.BoardInput) string {
	t.Helper()
	d, err := svc.Board(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("the page is off the grammar: %v", err)
	}
	raw, _ := json.Marshal(d)
	return string(raw)
}
