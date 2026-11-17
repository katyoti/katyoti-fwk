// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// : a source whose connector proposes from the holder's
// keywords opens its take tab on mock-up v6's proposal region - a head (the
// take label, what the source takes), « Proposé d'après vos mots-clés » with
// the holder's keywords as chips and « Modifier les mots-clés » opening the
// holder, each proposed item with its why, slots, detail and declared badges,
// ticked when the source holds it, the items chosen and no longer proposed
// after them, a search for another, and the save bar « N <unit> choisie(s) »
// with its idle and empty lines, saving through sources/hold with read.
// The head names the system the connector declares (« prend dans <system> »),
// the search hint its first slot's attribute (« (nom ou code) »). Nothing in
// it names a connector.
func TestAProposingSourceTakesOnItsProposals(t *testing.T) {
	ctx, cwm, svc, _ := pickWorldWith(t, map[string]string{"offers.yaml": `name: offers
label: Offres
system: Carto
take: Applications
description: Les applications de la carte.
sync: ktw-apps:offers/source-sync
search: ktw-apps:offers/search
propose: ktw-apps:offers/propose
picker:
  unit: application
  gender: feminine
  badges: {end-of-life: {label: Fin de vie, color: orange}}
kinds:
  application: {label: Application, slots: [{attr: code, label: Code applicatif}]}
`})
	// The holder's keywords, as an as-is study carries them.
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	o.Capabilities[api.KnowledgeCapability] = map[string]any{"keywords": []any{"facturation", "stock"}}
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	// The source holds one proposal and one item found by a search.
	if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "offers", Items: []api.SelectedItem{{Key: "O1", Title: "Facturation clients"}, {Key: "X9", Title: "Trouvée ailleurs"}}}); err != nil {
		t.Fatal(err)
	}
	out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: "m.offers", Tab: api.SourceTabTake})
	if err != nil {
		t.Fatal(err)
	}
	if err := render.ValidatePage(out.DSLTree); err != nil {
		t.Errorf("the take tab is no valid page: %v", err)
	}
	raw, _ := json.Marshal(out.DSLTree)
	take := string(raw)
	for _, want := range []string{
		`"id":"source-take-head"`, `"content":"Applications"`, `» prend dans Carto · cochez, puis enregistrez`,
		`"tickable":true`, `"title":"Proposé d'après vos mots-clés"`, `"text":"facturation"`, `"text":"stock"`,
		`"label":"Modifier les mots-clés"`, `"navigate":{"object":"m"}`,
		`"none":"Aucune application ne correspond aux mots-clés."`, `"search":"Chercher une autre application (nom ou code)"`,
		`"one":"application choisie"`, `"many":"applications choisies"`, `"idle":"Enregistré · pas encore lu"`, `"empty":"Rien de coché : cochez des propositions"`,
		`"apply":"ktw-agents:sources/hold"`, `"read":true`, `"dataField":"$.selected"`, `"end-of-life":{"label":"Fin de vie"`,
	} {
		if !strings.Contains(take, want) {
			t.Errorf("the take tab lacks %s: %s", want, take)
		}
	}
	sel, _ := json.Marshal(out.Data["selected"])
	var rows []map[string]any
	_ = json.Unmarshal(sel, &rows)
	if len(rows) != 3 || rows[0]["key"] != "O1" || rows[1]["key"] != "O2" || rows[2]["key"] != "X9" {
		t.Fatalf("the proposals then the chosen not proposed: %s", sel)
	}
	if rows[0]["off"] != nil || rows[0]["why"] != "« facturation · stock » dans le nom" || rows[0]["detail"] != "Émet les factures" ||
		rows[0]["attrs"].(map[string]any)["code"] != "FAC" || rows[0]["kind"] != "application" {
		t.Errorf("a held proposal comes ticked with its why, detail and slots: %+v", rows[0])
	}
	if rows[1]["off"] != true || rows[1]["badges"].([]any)[0] != "end-of-life" {
		t.Errorf("a proposal not held comes unticked with its badges: %+v", rows[1])
	}
	if rows[2]["off"] != nil {
		t.Errorf("an item chosen and not proposed stays ticked: %+v", rows[2])
	}
	// The chosen item no longer proposed is flagged proposed false - a chip
	// only, never a proposal row; a proposal carries no flag.
	if rows[2]["proposed"] != false {
		t.Errorf("an item chosen and not proposed is not flagged proposed false: %+v", rows[2])
	}
	if _, ok := rows[0]["proposed"]; ok {
		t.Errorf("a proposal carries a proposed flag: %+v", rows[0])
	}
	if _, ok := rows[1]["proposed"]; ok {
		t.Errorf("a proposal carries a proposed flag: %+v", rows[1])
	}

	// A connector that does not propose keeps its picker.
	used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "drive"})
	if err != nil {
		t.Fatalf("a source of a connector proposing nothing: %v", err)
	}
	plain, err := svc.SourceTab(ctx, api.SourceTabInput{ID: used.Source, Tab: api.SourceTabTake})
	if err != nil {
		t.Fatalf("the take tab of a connector proposing nothing: %v", err)
	}
	if raw, _ := json.Marshal(plain.DSLTree); strings.Contains(string(raw), `"proposals"`) {
		t.Errorf("a connector proposing nothing opens on a proposal region: %s", raw)
	}
}

// The words of a proposing take tab agree with the gender its connector
// declares for its unit - never guessed from the word: a masculine unit
// ending like a feminine one reads « un autre », « choisis »; a connector
// naming no system reads « prend · cochez » with no « dans ».
func TestATakeTabAgreesWithTheDeclaredGender(t *testing.T) {
	ctx, _, svc, _ := pickWorldWith(t, map[string]string{"stocks.yaml": `name: stocks
description: Les stocks.
sync: ktw-apps:stocks/source-sync
search: ktw-apps:stocks/search
propose: ktw-apps:stocks/propose
picker:
  unit: entrepôt
  gender: masculine
`, "notes.yaml": `name: notes
description: Les notes.
sync: ktw-apps:notes/source-sync
search: ktw-apps:notes/search
propose: ktw-apps:notes/propose
picker:
  unit: note
  gender: feminine
`})
	for conn, wants := range map[string][]string{
		"stocks": {`"search":"Chercher un autre entrepôt"`, `"one":"entrepôt choisi"`, `"many":"entrepôts choisis"`, `» prend · cochez, puis enregistrez`},
		"notes":  {`"search":"Chercher une autre note"`, `"one":"note choisie"`, `"many":"notes choisies"`},
	} {
		used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: conn})
		if err != nil {
			t.Fatalf("%s: %v", conn, err)
		}
		out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: used.Source, Tab: api.SourceTabTake})
		if err != nil {
			t.Fatalf("%s: %v", conn, err)
		}
		raw, _ := json.Marshal(out.DSLTree)
		for _, want := range wants {
			if !strings.Contains(string(raw), want) {
				t.Errorf("%s: the take tab lacks %s: %s", conn, want, raw)
			}
		}
	}
}
