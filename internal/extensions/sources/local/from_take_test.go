// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// fromTakeApps keeps applications (fromActions' sync: ERP « Gestion ERP »,
// CRM « Relation client »); mon reads from them by their code and browses
// one node per code, its entities below it.
const (
	fromTakeApps = `name: apps
label: Applications
system: Carto
mark: CA
color: teal
description: The applications of the map.
sync: ktw-apps:apps/source-sync
search: ktw-apps:apps/search
picker: {unit: application, gender: feminine}
`
	fromTakeMon = `name: mon
label: Monitored entities
description: The monitored entities of the kept applications.
system: Supervision
color: orange
take: Entités
sync: ktw-ops:mon/source-sync
browse: ktw-ops:mon/browse
from: {connector: apps, attr: code}
picker: {unit: entité, gender: feminine}
`
)

// fromTakeActions serves apps through fromActions and mon's browse: the top
// level is one node per code handed (its tag as detail), with the match
// line; ERP holds two entities, CRM none - or, once fail is set, a plain
// line and its details.
type fromTakeActions struct {
	from fromActions
	fail *bool
}

func (fromTakeActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a fromTakeActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	if in.URI != "ktw-ops:mon/browse" {
		return a.from.Run(ctx, in)
	}
	var req api.ConnectorBrowseInput
	if err := json.Unmarshal(in.Input, &req); err != nil {
		return actionsapi.RunOutput{}, err
	}
	out := api.ConnectorBrowseOutput{Items: []api.BrowseItem{}, Match: "valeur du tag app"}
	switch {
	case req.Parent == "":
		for _, v := range req.From.Values {
			out.Items = append(out.Items, api.BrowseItem{Key: "n:" + v, Title: v, Kind: "application", HasChildren: true,
				Detail: "app:" + strings.ToLower(v), Attrs: map[string]string{"code": v}})
		}
	case req.Parent == "n:ERP":
		out.Items = append(out.Items, api.BrowseItem{Key: "E1", Title: "erp-web", Kind: "service", Detail: "SERVICE"},
			api.BrowseItem{Key: "E2", Title: "erp-db", Kind: "host", Detail: "HOST"})
	case req.Parent == "n:CRM" && *a.fail:
		return actionsapi.RunOutput{}, errors.New("La supervision a refusé la demande.\ndétails : HTTP 400 bad selector")
	case req.Parent == "n:XYZ":
		out.Items = append(out.Items, api.BrowseItem{Key: "E9", Title: "xyz-api", Kind: "service"})
	}
	raw, _ := json.Marshal(out)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// : a source whose connector reads from another source's kept
// items opens its take tab on mock-up v6's Dynatrace view - the head, the
// « Lien entre sources » band, then « Rien à proposer pour l'instant » and
// « Ouvrir la source … » while nothing is kept; once the other source keeps
// values, one group per value (its title, code, tag chip, empty line) with
// the items browsed below it, the save bar in the unit's words; a value
// typed by hand is searched through the same two-level browse; a browse's
// error shows its first line, the rest under « détails ».
func TestAFromSourceTakesInGroups(t *testing.T) {
	root := t.TempDir()
	fail := new(bool)
	apps := loader.NewEmbedFSLoader(fstest.MapFS{"connectors/apps.yaml": {Data: []byte(fromTakeApps)}}, ".")
	mon := loader.NewEmbedFSLoader(fstest.MapFS{"connectors/mon.yaml": {Data: []byte(fromTakeMon)}}, ".")
	ctx, cwm := cwmapp.NewAt(t, root,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-apps", apps, "connectors"),
		contributeConnectors("ktw-ops", mon, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return fromTakeActions{from: fromActions{mu: &sync.Mutex{}, seen: &[]*api.FromItems{}}, fail: fail}, nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "m", Capability: api.KnowledgeCapability}); err != nil {
		t.Fatal(err)
	}
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	monSrc := createSource(t, ctx, cwm, "mon-src", map[string]any{"connector": "mon"})
	take := func() (string, map[string]any, []any) {
		t.Helper()
		out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: monSrc, Tab: api.SourceTabTake})
		if err != nil {
			t.Fatal(err)
		}
		if err := render.ValidatePage(out.DSLTree); err != nil {
			t.Errorf("the take tab is no valid page: %v", err)
		}
		raw, _ := json.Marshal(out.DSLTree)
		var tree map[string]any
		_ = json.Unmarshal(raw, &tree)
		var picker map[string]any
		for _, c := range tree["children"].([]any) {
			if n := c.(map[string]any); n["kind"] == "picker" {
				picker = n
			}
		}
		sel, _ := out.Data["selected"].([]any)
		return string(raw), picker, sel
	}
	has := func(raw string, wants ...string) {
		t.Helper()
		for _, w := range wants {
			if !strings.Contains(raw, w) {
				t.Errorf("the take tab lacks %s: %s", w, raw)
			}
		}
	}

	// Nothing kept by apps: the band says so, and the empty state.
	raw, picker, _ := take()
	has(raw, `"content":"Entités"`, `ce que « Monitored entities » prend dans Supervision · cochez, puis enregistrez`,
		`"id":"source-take-link"`, `"content":"Aucune source Carto"`, `"content":"donne le code"`, `"content":"rien à chercher"`,
		`"id":"source-take-empty"`, `"content":"Rien à proposer pour l'instant."`,
		`Cette source propose les entités des applications cochées dans la source Carto : il n'y en a pas encore.`)
	if picker != nil || strings.Contains(raw, "source-take-open-from") {
		t.Errorf("nothing to propose and no source to open: no picker, no button: %s", raw)
	}

	// apps keeps ERP and CRM: one group per code, ERP's entities below it.
	appsSrc := createSource(t, ctx, cwm, "apps-src", map[string]any{"connector": "apps"})
	raw, _, _ = take()
	has(raw, `"id":"source-take-open-from"`, `"label":"Ouvrir la source Carto"`)
	if res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: appsSrc}); err != nil || !res.OK {
		t.Fatalf("apps sync: %+v %v", res, err)
	}
	raw, picker, sel := take()
	has(raw, `"kind":"link","text":"apps-src"`, `"navigate":{"object":"m.apps-src"}`, `"content":"2 applications · CRM, ERP"`, `"content":"valeur du tag app"`,
		`"content":"app:crm · app:erp"`, `Une application cochée là-bas propose ici ses entités`)
	if picker == nil {
		t.Fatalf("values kept: the proposal region: %s", raw)
	}
	props, _ := picker["proposals"].(map[string]any)
	if props["title"] != "Proposé d'après les 2 applications de la source Carto (code)" {
		t.Errorf("the proposal's title: %+v", props["title"])
	}
	groups, _ := props["groups"].([]any)
	if len(groups) != 2 {
		t.Fatalf("one group per code: %+v", props)
	}
	crm, erp := groups[0].(map[string]any), groups[1].(map[string]any)
	if erp["key"] != "n:ERP" || erp["title"] != "Gestion ERP" || erp["code"] != "ERP" || erp["chip"] != "app:erp" {
		t.Errorf("ERP's group: its application's title, code and tag chip: %+v", erp)
	}
	if b, _ := erp["badge"].(map[string]any); b["label"] != "CA" || b["color"] != "teal" {
		t.Errorf("a group's badge is the other source's declared mark: %+v", erp)
	}
	// : the band's badges are each system's mark - the other
	// connector's declared one, this one's system name as it declares none.
	has(raw, `"color":"teal","kind":"badge","text":"CA"`, `"color":"orange","kind":"badge","text":"Supervision"`)
	if strings.Contains(raw, `"text":"Carto"`) {
		t.Errorf("a connector declaring a mark is drawn by it, never its system's name: %s", raw)
	}
	if crm["empty"] != "Aucune entité ne porte app:crm dans Supervision." {
		t.Errorf("CRM's empty line: %+v", crm)
	}
	if len(sel) != 2 || sel[0].(map[string]any)["group"] != "n:ERP" || sel[0].(map[string]any)["off"] != true {
		t.Errorf("ERP's entities proposed unticked in its group: %+v", sel)
	}
	bar, _ := picker["savebar"].(map[string]any)
	labels, _ := picker["labels"].(map[string]any)
	if bar["many"] != "entités choisies" || bar["one"] != "entité choisie" || labels["search"] != "Ajouter une application par son code" ||
		picker["search"] != api.SearchURI {
		t.Errorf("the save bar and the typed-code search: %+v %+v", bar, labels)
	}

	// A code typed by hand: browsed in place of what apps kept.
	got, err := svc.Search(ctx, api.SearchInput{Connector: "mon", Object: "m", Query: "XYZ"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Key != "E9" || got.Items[0].Parent != "n:XYZ" {
		t.Errorf("a typed code lists its entities: %+v", got.Items)
	}
	got, err = svc.Search(ctx, api.SearchInput{Source: monSrc, Query: "XYZ"})
	if err != nil || len(got.Items) != 1 {
		t.Errorf("a typed code from the source: %+v %v", got.Items, err)
	}

	// A browse that fails: its first line in red, the rest under « détails ».
	*fail = true
	raw, _, _ = take()
	has(raw, `"id":"source-take-problem"`, `"color":"red","content":"La supervision a refusé la demande."`,
		`"label":"détails"`, `détails : HTTP 400 bad selector`)
}

// : a from-connector source's take tab marks a group whose
// value the other source kept since this source's last save « ajoutée dans
// <System> » and that group's unticked rows « nouvelle » (drawn only while
// unticked), as mock-up v6's dtGroup; the save records the values seen; a
// source never saved marks nothing.
func TestAFromSourceMarksValuesKeptSinceItsSave(t *testing.T) {
	root := t.TempDir()
	apps := loader.NewEmbedFSLoader(fstest.MapFS{"connectors/apps.yaml": {Data: []byte(fromTakeApps)}}, ".")
	mon := loader.NewEmbedFSLoader(fstest.MapFS{"connectors/mon.yaml": {Data: []byte(fromTakeMon)}}, ".")
	ctx, cwm := cwmapp.NewAt(t, root,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-apps", apps, "connectors"),
		contributeConnectors("ktw-ops", mon, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return fromTakeActions{from: fromActions{mu: &sync.Mutex{}, seen: &[]*api.FromItems{}}, fail: new(bool)}, nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "m", Capability: api.KnowledgeCapability}); err != nil {
		t.Fatal(err)
	}
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	monSrc := createSource(t, ctx, cwm, "mon-src", map[string]any{"connector": "mon"})
	appsSrc := createSource(t, ctx, cwm, "apps-src", map[string]any{"connector": "apps"})
	if res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: appsSrc}); err != nil || !res.OK {
		t.Fatalf("apps sync: %+v %v", res, err)
	}
	// marks is the badges of each group's head and of each row, by key.
	marks := func() (map[string]any, map[string]any, map[string]any) {
		t.Helper()
		out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: monSrc, Tab: api.SourceTabTake})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(out)
		var got struct {
			DSLTree struct {
				Children []map[string]any `json:"children"`
			} `json:"dslTree"`
			Data struct {
				Selected []map[string]any `json:"selected"`
			} `json:"data"`
		}
		_ = json.Unmarshal(raw, &got)
		heads, rows := map[string]any{}, map[string]any{}
		var badges map[string]any
		for _, c := range got.DSLTree.Children {
			if c["kind"] != "picker" {
				continue
			}
			badges, _ = c["badges"].(map[string]any)
			props, _ := c["proposals"].(map[string]any)
			groups, _ := props["groups"].([]any)
			for _, g := range groups {
				gm := g.(map[string]any)
				heads[gm["code"].(string)] = gm["badges"]
			}
		}
		for _, r := range got.Data.Selected {
			rows[r["key"].(string)] = r["badges"]
		}
		if len(heads) == 0 {
			t.Fatalf("the take tab draws no group: %s", raw)
		}
		return heads, rows, badges
	}

	// Never saved: nothing marked.
	heads, rows, badges := marks()
	for k, b := range heads {
		if b != nil {
			t.Errorf("a source never saved marks no group: %s %v", k, b)
		}
	}
	for k, b := range rows {
		if b != nil {
			t.Errorf("a source never saved marks no row: %s %v", k, b)
		}
	}
	added, _ := badges["from-added"].(map[string]any)
	fresh, _ := badges["from-new"].(map[string]any)
	if added["label"] != "ajoutée dans Carto" || added["color"] != "orange" || fresh["label"] != "nouvelle" || fresh["color"] != "blue" || fresh["unticked"] != true {
		t.Errorf("the picker declares « ajoutée dans <System> » and « nouvelle » drawn while unticked, agreeing with the units: %+v", badges)
	}

	// Saved with E1 ticked while apps keeps CRM and ERP: the save records them.
	if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "mon", Items: []api.SelectedItem{{Key: "E1", Title: "erp-web"}}}); err != nil {
		t.Fatal(err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: monSrc})
	if err != nil {
		t.Fatal(err)
	}
	var f api.InformationSourceFields
	_ = json.Unmarshal(got.Object.FieldsJSON(), &f)
	if strings.Join(f.FromSeen, ",") != "CRM,ERP" && strings.Join(f.FromSeen, ",") != "ERP,CRM" {
		t.Fatalf("the save records the values the other source keeps: %+v", f.FromSeen)
	}
	heads, rows, _ = marks()
	if heads["ERP"] != nil || heads["CRM"] != nil || rows["E1"] != nil || rows["E2"] != nil {
		t.Errorf("values seen at the save: nothing marked: %+v %+v", heads, rows)
	}

	// ERP kept since the save (the save saw CRM only): its group is
	// « ajoutée », its unticked row « nouvelle », its ticked row not.
	f.FromSeen = []string{"CRM"}
	raw, _ := json.Marshal(f)
	obj := got.Object
	obj.SetFieldsJSON(raw)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: obj, Revision: got.Revision}); err != nil {
		t.Fatal(err)
	}
	heads, rows, _ = marks()
	if b, _ := heads["ERP"].([]any); len(b) != 1 || b[0] != "from-added" {
		t.Errorf("a value kept since the save: its group « ajoutée »: %+v", heads)
	}
	if heads["CRM"] != nil {
		t.Errorf("a value seen at the save: no mark: %+v", heads)
	}
	if b, _ := rows["E2"].([]any); len(b) != 1 || b[0] != "from-new" {
		t.Errorf("an unticked row of a new value: « nouvelle »: %+v", rows)
	}
	if rows["E1"] != nil {
		t.Errorf("a ticked row: no « nouvelle »: %+v", rows)
	}

	// The source's card says it (mock-up v6 srcState): the
	// unticked E2 of ERP, kept since the save, is one new proposal.
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	var card *api.ConnectorUse
	for i := range of.Connectors {
		if of.Connectors[i].Name == "mon" {
			card = &of.Connectors[i]
		}
	}
	if card == nil || card.State == nil {
		t.Fatalf("no card for the from-connector source: %+v", of.Connectors)
	}
	if card.State.Text != "1 proposition" || card.State.Color != "blue" || card.State.Kind != api.CardNews {
		t.Errorf("a value kept since the save: the card says « 1 proposition » in blue: %+v", card.State)
	}
	if want := "1 entité · 1 nouvelle d'après Gestion ERP, ajoutée dans Carto"; card.Meta != want {
		t.Errorf("the card's meta line:\n got %q\nwant %q", card.Meta, want)
	}
}
