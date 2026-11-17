// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
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

// treePropApps keeps applications (fromActions' sync: ERP « Gestion ERP »,
// CRM « Relation client ») and declares its system and colour; wiki picks
// pages in a tree and proposes from the holder's keywords and from the
// applications apps kept (propose_from); dirs is a tree that proposes
// nothing.
const (
	treePropApps = `name: apps
label: Applications
system: Carto
color: teal
description: The applications of the map.
sync: ktw-apps:apps/source-sync
search: ktw-apps:apps/search
picker: {unit: application, gender: feminine}
`
	treePropWiki = `name: wiki
description: Des pages.
take: Pages prises
sync: ktw-wiki:wiki/source-sync
browse: ktw-wiki:wiki/browse
search: ktw-wiki:wiki/search
propose: ktw-wiki:wiki/propose
scopes: {unit: espace, list: ktw-sp:sp/scopes}
propose_from: {connector: apps, attr: code}
picker: {unit: page, gender: feminine, layout: tree}
`
	treePropDirs = `name: dirs
description: Des dossiers.
system: Disque
sync: ktw-wiki:dirs/source-sync
browse: ktw-wiki:dirs/browse
picker: {unit: dossier, layout: tree}
`
)

// treePropActions serves apps through fromActions, the scopes through
// scopeActions, and wiki's propose through sources' ProposeBySearch over a
// made-up index, recording what each proposal was handed.
type treePropActions struct {
	from  fromActions
	mu    *sync.Mutex
	asked *[]api.ConnectorProposeInput
}

func (treePropActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a treePropActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	switch in.URI {
	case "ktw-sp:sp/scopes":
		return scopeActions{}.Run(ctx, in)
	case "ktw-wiki:wiki/propose":
		var req api.ConnectorProposeInput
		if err := json.Unmarshal(in.Input, &req); err != nil {
			return actionsapi.RunOutput{}, err
		}
		a.mu.Lock()
		*a.asked = append(*a.asked, req)
		a.mu.Unlock()
		index := map[string][]api.PickItem{
			"facturation": {{Key: "P1", Title: "Facturation", Path: []string{"AUP", "Applications"}, Below: 3, HasChildren: true}},
			"gestion erp": {{Key: "P2", Title: "Gestion ERP - exploitation", Path: []string{"AUP"}}},
		}
		out, err := api.ProposeBySearch(ctx, req, func(_ context.Context, q string) ([]api.PickItem, error) {
			return index[strings.ToLower(q)], nil
		})
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		raw, _ := json.Marshal(out)
		return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
	}
	return a.from.Run(ctx, in)
}

// : a tree source whose connector proposes opens « Pages
// prises » on mock-up v6's proposal region above its tree - « Proposé
// d'après vos mots-clés et les applications de la source Carto » (its
// propose_from), the holder's keywords as chips, one row per proposed page
// with its path, why (« mot-clé « x » ») and sub-page count, « Aucune page ne
// correspond. », « Ou choisissez dans l'arbre de l'espace AUP » and the
// search hint. Once the holder's other source keeps applications, a page
// only an application's title finds comes with « application « X » de la
// source Carto » and that source's pill. The proposals never join the saved
// selection; a tree connector that proposes nothing keeps its bare tree.
func TestATreeSourceTakesOnItsProposals(t *testing.T) {
	root := t.TempDir()
	asked := &[]api.ConnectorProposeInput{}
	apps := loader.NewEmbedFSLoader(fstest.MapFS{"connectors/apps.yaml": {Data: []byte(treePropApps)}}, ".")
	wiki := loader.NewEmbedFSLoader(fstest.MapFS{
		"connectors/wiki.yaml": {Data: []byte(treePropWiki)},
		"connectors/dirs.yaml": {Data: []byte(treePropDirs)},
	}, ".")
	ctx, cwm := cwmapp.NewAt(t, root,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-apps", apps, "connectors"),
		contributeConnectors("ktw-wiki", wiki, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return treePropActions{from: fromActions{mu: &sync.Mutex{}, seen: &[]*api.FromItems{}}, mu: &sync.Mutex{}, asked: asked}, nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "m", Capability: api.KnowledgeCapability}); err != nil {
		t.Fatal(err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	o.Capabilities[api.KnowledgeCapability] = map[string]any{"keywords": []any{"facturation"}}
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	wikiSrc := createSource(t, ctx, cwm, "wiki-src", map[string]any{"connector": "wiki"})

	take := func() (string, map[string]any, []any) {
		t.Helper()
		out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: wikiSrc, Tab: api.SourceTabTake})
		if err != nil {
			t.Fatal(err)
		}
		if err := render.ValidatePage(out.DSLTree); err != nil {
			t.Errorf("the take tab is no valid page: %v", err)
		}
		raw, _ := json.Marshal(out.DSLTree)
		var tree map[string]any
		_ = json.Unmarshal(raw, &tree)
		var props map[string]any
		for _, c := range tree["children"].([]any) {
			if n := c.(map[string]any); n["kind"] == "picker" {
				props, _ = n["proposals"].(map[string]any)
			}
		}
		sel, _ := out.Data["selected"].([]any)
		return string(raw), props, sel
	}

	// Nothing kept by apps yet: the keyword's proposal.
	raw, props, sel := take()
	for _, want := range []string{
		`"content":"Pages prises"`, `» prend dans l'espace AUP · cochez, puis enregistrez`, `"layout":"tree"`,
		`"title":"Proposé d'après vos mots-clés et les applications de la source Carto"`, `"text":"facturation"`,
		`"label":"Modifier les mots-clés"`, `"none":"Aucune page ne correspond."`,
		`"choose":"Ou choisissez dans l'arbre de l'espace AUP"`, `"search":"Chercher une page dans l'espace"`,
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("the take tab lacks %s: %s", want, raw)
		}
	}
	rows, _ := props["items"].([]any)
	if len(rows) != 1 {
		t.Fatalf("one page the keyword finds: %+v", props)
	}
	p1 := rows[0].(map[string]any)
	if p1["key"] != "P1" || p1["why"] != "mot-clé « facturation »" || p1["below"] != float64(3) ||
		strings.Join(toStrings(p1["path"]), " / ") != "AUP / Applications" || p1["pill"] != nil {
		t.Errorf("a page found by a keyword: its path, why and sub-page count, no pill: %+v", p1)
	}
	if len(sel) != 0 {
		t.Errorf("proposals never join the saved selection: %+v", sel)
	}

	// apps keeps two applications: wiki's proposal is handed them.
	appsSrc := createSource(t, ctx, cwm, "apps-src", map[string]any{"connector": "apps"})
	if res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: appsSrc}); err != nil || !res.OK {
		t.Fatalf("apps sync: %+v %v", res, err)
	}
	_, props, _ = take()
	last := (*asked)[len(*asked)-1]
	if last.ProposeFrom == nil || last.ProposeFrom.Connector != "apps" || last.ProposeFrom.Title("ERP") != "Gestion ERP" || last.From != nil {
		t.Errorf("the proposal is handed what apps kept as propose_from, never as from: %+v", last)
	}
	rows, _ = props["items"].([]any)
	if len(rows) != 2 {
		t.Fatalf("the keyword's page, then the application's: %+v", props)
	}
	p2 := rows[1].(map[string]any)
	pill, _ := p2["pill"].(map[string]any)
	if p2["key"] != "P2" || p2["why"] != "application « Gestion ERP » de la source Carto" ||
		pill["label"] != "Carto" || pill["color"] != "teal" || pill["icon"] != "link" {
		t.Errorf("a page an application's title finds: its why and the other source's pill: %+v", p2)
	}

	// A tree that proposes nothing keeps its bare tree.
	dirs, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "dirs"})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := svc.SourceTab(ctx, api.SourceTabInput{ID: dirs.Source, Tab: api.SourceTabTake})
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(plain.DSLTree); strings.Contains(string(raw), `"proposals"`) || !strings.Contains(string(raw), `» prend dans Disque · cochez`) {
		t.Errorf("a tree proposing nothing keeps its head and bare tree: %s", raw)
	}
}

// toStrings reads a JSON array of strings.
func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}
