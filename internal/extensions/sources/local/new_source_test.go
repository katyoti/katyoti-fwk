// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// (mock-up v6 view 11): an information source is created from
// its own page under its holder - the type's create_task -: one choice card
// per contributed connector (its colour, its label, its description), a
// connector a holder asking for one source per connector
// already reads through greyed « Une seule par module »
// with its pill « déjà dans le module » and why on hover (« l'étude » for an
// étude, the article read from the holder's type label); a card names the
// source after its connector; a connector a carrier adds scopes to asks its
// first one, only when chosen - a choice of the scopes its configuration
// joins, the first chosen; « Annuler » closes the dialog the page is drawn
// in; « Créer » waits for a connector and a name and
// runs sources/use with them, which names the source, keeps the scope and
// opens it on its take tab.
func TestAnInformationSourceIsCreatedFromItsPage(t *testing.T) {
	ctx, cwm, svc, _ := pickWorldWith(t, map[string]string{"spaces.yaml": scopedSpaces})
	page := func() (map[string]map[string]any, string) {
		t.Helper()
		out, err := svc.NewSourcePage(ctx, api.NewSourcePageInput{Parent: "m"})
		if err != nil {
			t.Fatal(err)
		}
		if err := render.ValidatePage(out.DSLTree); err != nil {
			t.Errorf("the create page is no valid page: %v", err)
		}
		raw, _ := json.Marshal(out.DSLTree)
		var tree map[string]any
		_ = json.Unmarshal(raw, &tree)
		cards := map[string]map[string]any{}
		for _, c := range tree["children"].([]any) {
			if n := c.(map[string]any); n["kind"] == "choice-cards" {
				for _, o := range n["options"].([]any) {
					cards[o.(map[string]any)["value"].(string)] = o.(map[string]any)
				}
			}
		}
		return cards, string(raw)
	}
	cards, tree := page()
	if strings.Contains(tree, "celles déjà dans") {
		t.Errorf("the head sentence shows while the holder has no source: %s", tree)
	}
	if c := cards["apps"]; c == nil || c["label"] != "Applications" || c["disabled"] == true || c["sets"].(map[string]any)["name"] != "Applications" {
		t.Errorf("an untaken connector is a choice naming the source after it: %+v", c)
	}
	for _, want := range []string{`"kind":"choice-cards","label":"Ce qu'elle lit"`, `"key":"connector"`, `"kind":"text-field","label":"Nom"`,
		`"key":"scope","kind":"text-field","label":"Espace"`, `"when":"connector=spaces"`,
		`"options":[{"label":"AUP","value":"AUP"}]`, `"value":"AUP"`, `Les espaces joints dans la connexion Spaces.`,
		`"close":true`, `"id":"new-source-cancel"`, `"label":"Annuler"`,
		`"uri":"ktw-agents:sources/use"`, `"connector":"${selection.connector}"`, `"name":"${selection.name}"`, `"scope":"${selection.scope}"`,
		`"requires":["connector","name"]`, `"label":"Créer"`, `Elle s'ouvre sur ses propositions`} {
		if !strings.Contains(tree, want) {
			t.Errorf("the page lacks %s: %s", want, tree)
		}
	}

	// « Créer »: the source is named, keeps its first scope and opens on its take tab.
	made, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "spaces", Name: "Espaces — architecture", Scope: "ARCHI"})
	if err != nil || !made.Created || made.Open == nil || made.Open.Object != made.Source || made.Open.Panel != api.SourceTakePanel {
		t.Fatalf("Créer makes the source and opens it on its take tab: %+v %v", made, err)
	}
	src, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: made.Source})
	if err != nil || src.Object.Name != "Espaces — architecture" || !strings.Contains(string(src.Object.FieldsJSON()), "ARCHI") {
		t.Errorf("the source carries its name and its scope: %+v %v", src.Object, err)
	}
	if _, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "drive", Scope: "X"}); err == nil {
		t.Errorf("a scope for a connector no carrier adds scopes to is refused")
	}
	if made, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "apps"}); err != nil || !made.Created {
		t.Fatal(made, err)
	}

	// A plain module does not ask for one source per connector: nothing is
	// greyed.
	if cards, _ := page(); cards["apps"]["disabled"] == true {
		t.Errorf("a holder without one-source-per-connector greys a connector it has: %+v", cards["apps"])
	}
	// Once it asks for it (the capability the as-is study v2 composes), a
	// connector it has is greyed.
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "m", Capability: api.OnePerConnectorCapability}); err != nil {
		t.Fatal(err)
	}
	cards, tree = page()
	if c := cards["apps"]; c["disabled"] != true || c["pill"] != "déjà dans le module" || c["description"] != "Une seule par module" ||
		!strings.Contains(fmt.Sprint(c["tooltip"]), "ouvrez celle du module") {
		t.Errorf("a connector the holder has is greyed with its pill and why: %+v", c)
	}
	if c := cards["drive"]; c == nil || c["disabled"] == true {
		t.Errorf("a connector the holder lacks is offered: %+v", c)
	}
	if !strings.Contains(tree, "celles déjà dans le module sont grisées") || strings.Contains(tree, `"when":"connector=spaces"`) {
		t.Errorf("once it has sources: the head sentence, and no scope asked for a connector it holds: %s", tree)
	}
	for _, bad := range []api.NewSourcePageInput{{}, {Parent: "m.apps"}} {
		if _, err := svc.NewSourcePage(ctx, bad); err == nil {
			t.Errorf("%+v is refused: no module holder", bad)
		}
	}
	got, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: api.InformationSourceType})
	if err != nil {
		t.Fatal(err)
	}
	if got.ObjectType.CreateTask != api.NewSourcePageURI {
		t.Errorf("the type's create_task is its page: %q", got.ObjectType.CreateTask)
	}
}
