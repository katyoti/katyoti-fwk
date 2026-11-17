// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// A source whose connector picks in a check-box tree opens its take tab on
// mock-up v6's « Pages prises »: a head - the take label, what the source
// takes and in which of its connector's scopes, « cochez, puis enregistrez »
// - then the tree picker speaking its unit, agreeing with its declared gender
// (« avec les sous-pages », « cette page seule », « incluse », the help and
// the notice), its selection handing each item's parent and whether its
// descendants come with it, held or picked, so the selection shows as a tree.
func TestATreeSourceTakesItsItemsAsATree(t *testing.T) {
	ctx, _, svc, _ := pickWorldWith(t, map[string]string{"wiki.yaml": `name: wiki
description: Des pages.
take: Pages prises
sync: ktw-apps:wiki/source-sync
browse: ktw-apps:wiki/browse
search: ktw-apps:wiki/search
scopes: {unit: espace, list: ktw-sp:sp/scopes}
picker:
  unit: page
  gender: feminine
  layout: tree
`, "files.yaml": `name: files
description: Des dossiers.
system: Disque
sync: ktw-apps:files/source-sync
browse: ktw-apps:files/browse
search: ktw-apps:files/search
picker:
  unit: dossier
  gender: masculine
  layout: tree
`})
	// Held: a page with its sub-pages, one of another alone.
	if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "wiki", Items: []api.SelectedItem{
		{Key: "P1", Title: "Overview", WithChildren: true}, {Key: "P2", Title: "Interfaces", Parent: "P9"}}}); err != nil {
		t.Fatal(err)
	}
	// Picked: a folder alone under another.
	files, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "files"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SourceSelect(ctx, api.SourceSelectInput{ID: files.Source, Items: []api.SelectedItem{{Key: "F2", Title: "Plans", Parent: "F1"}}}); err != nil {
		t.Fatal(err)
	}
	for source, c := range map[string]struct {
		rows  map[string]map[string]any
		wants []string
	}{
		"m.wiki": {map[string]map[string]any{"P1": {"with_children": true}, "P2": {"parent": "P9"}}, []string{
			`"id":"source-take-head"`, `"content":"Pages prises"`, `» prend dans l'espace AUP · cochez, puis enregistrez`, `"layout":"tree"`,
			`"withChildren":"avec les sous-pages"`, `"belowOne":"avec 1 sous-page"`, `"belowMany":"avec {n} sous-pages"`, `"self":"cette page seule"`, `"included":"incluse"`,
			`"treeHelp":"Comment se prennent les sous-pages"`,
			`"treeHelpWith":"Cocher une page la prend avec ses sous-pages, aussi celles ajoutées plus tard. « cette page seule » ne prend qu'elle."`,
			`"treeHelpIncluded":"Sous-page incluse par sa parente. La décocher fait passer la parente à « cette page seule » ; ses autres sous-pages restent prises."`,
			`"treeHelpMixed":"La page n'est pas prise, mais des pages en dessous le sont."`,
			`"treeNotice":"« {removed} » retirée : « {parent} » passe à « cette page seule », ses autres sous-pages restent prises."`,
		}},
		files.Source: {map[string]map[string]any{"F2": {"parent": "F1"}}, []string{
			`» prend dans Disque · cochez, puis enregistrez`, `"withChildren":"avec les sous-dossiers"`, `"belowOne":"avec 1 sous-dossier"`, `"belowMany":"avec {n} sous-dossiers"`, `"self":"ce dossier seul"`, `"included":"inclus"`,
			`"treeNotice":"« {removed} » retiré : « {parent} » passe à « ce dossier seul », ses autres sous-dossiers restent pris."`,
		}},
	} {
		out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: source, Tab: api.SourceTabTake})
		if err != nil {
			t.Fatalf("%s: %v", source, err)
		}
		if err := render.ValidatePage(out.DSLTree); err != nil {
			t.Errorf("%s: the take tab is no valid page: %v", source, err)
		}
		raw, _ := json.Marshal(out.DSLTree)
		for _, want := range c.wants {
			if !strings.Contains(string(raw), want) {
				t.Errorf("%s: the take tab lacks %s: %s", source, want, raw)
			}
		}
		sel, _ := json.Marshal(out.Data["selected"])
		var rows []map[string]any
		_ = json.Unmarshal(sel, &rows)
		if len(rows) != len(c.rows) {
			t.Fatalf("%s: the selection: %s", source, sel)
		}
		for _, row := range rows {
			want := c.rows[row["key"].(string)]
			if row["parent"] != want["parent"] || row["with_children"] != want["with_children"] {
				t.Errorf("%s: %v hands parent %v and with_children %v, want %v: %s", source, row["key"], row["parent"], row["with_children"], want, sel)
			}
		}
	}
}
