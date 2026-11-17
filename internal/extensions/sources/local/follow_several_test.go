// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// A second application connector beside treePropApps (Carto): Registre
// keeps HR « Gestion RH »; pages proposes from both.
const (
	severalRegistry = `name: registry
label: Registre
system: Registre
color: grape
description: The applications of another registry.
sync: ktw-reg:reg/source-sync
picker: {unit: application, gender: feminine}
`
	severalPages = `name: pages
description: Des pages.
take: Pages prises
sync: ktw-wiki:pages/source-sync
browse: ktw-wiki:pages/browse
propose: ktw-wiki:pages/propose
scopes: {unit: espace, list: ktw-sp:sp/scopes}
propose_from: {connectors: [apps, registry], attr: code}
picker: {unit: page, gender: feminine, layout: tree}
`
)

// severalActions serves apps through fromActions, Registre's sync, the
// scopes, and pages' propose through ProposeBySearch over a made-up index,
// recording what each proposal was handed.
type severalActions struct {
	from  fromActions
	mu    *sync.Mutex
	asked *[]api.ConnectorProposeInput
}

func (severalActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a severalActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	var value any
	switch in.URI {
	case "ktw-sp:sp/scopes":
		return scopeActions{}.Run(ctx, in)
	case "ktw-reg:reg/source-sync":
		var sin api.ConnectorSyncInput
		_ = json.Unmarshal(in.Input, &sin)
		if err := os.WriteFile(filepath.Join(sin.Dest, "hr.data.json"), []byte(`{"code":"HR"}`), 0o644); err != nil {
			return actionsapi.RunOutput{}, err
		}
		value = api.ConnectorSyncOutput{Items: []api.ConnectorItem{{Kind: "app", Data: "hr.data.json", URI: "reg://hr", Title: "Gestion RH", Key: "hr"}}}
	case "ktw-wiki:pages/propose":
		var req api.ConnectorProposeInput
		if err := json.Unmarshal(in.Input, &req); err != nil {
			return actionsapi.RunOutput{}, err
		}
		a.mu.Lock()
		*a.asked = append(*a.asked, req)
		a.mu.Unlock()
		index := map[string][]api.PickItem{
			"gestion erp": {{Key: "P2", Title: "Gestion ERP - exploitation", Path: []string{"AUP"}}},
			"gestion rh":  {{Key: "P3", Title: "Gestion RH - paie", Path: []string{"AUP"}}},
		}
		out, err := api.ProposeBySearch(ctx, req, func(_ context.Context, q string) ([]api.PickItem, error) {
			return index[strings.ToLower(q)], nil
		})
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		value = out
	default:
		return a.from.Run(ctx, in)
	}
	raw, _ := json.Marshal(value)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// : a carrier keeping applications in two systems' sources -
// Carto and Registre -, a source proposing from both hands each kept value
// with its own connector: a page only Registre's application finds carries
// Registre's pill and why, never Carto's, and the take tab's proposal title
// names both systems.
func TestAFollowingSourceNamesEachValuesOwnSystem(t *testing.T) {
	root := t.TempDir()
	asked := &[]api.ConnectorProposeInput{}
	apps := loader.NewEmbedFSLoader(fstest.MapFS{
		"connectors/apps.yaml":     {Data: []byte(treePropApps)},
		"connectors/registry.yaml": {Data: []byte(severalRegistry)},
	}, ".")
	wiki := loader.NewEmbedFSLoader(fstest.MapFS{"connectors/pages.yaml": {Data: []byte(severalPages)}}, ".")
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
			return severalActions{from: fromActions{mu: &sync.Mutex{}, seen: &[]*api.FromItems{}}, mu: &sync.Mutex{}, asked: asked}, nil
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
	pagesSrc := createSource(t, ctx, cwm, "pages-src", map[string]any{"connector": "pages"})
	for _, l := range []struct{ local, connector string }{{"apps-src", "apps"}, {"reg-src", "registry"}} {
		id := createSource(t, ctx, cwm, l.local, map[string]any{"connector": l.connector})
		if res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: id}); err != nil || !res.OK {
			t.Fatalf("%s sync: %+v %v", l.connector, res, err)
		}
	}

	out, err := svc.SourceTab(ctx, api.SourceTabInput{ID: pagesSrc, Tab: api.SourceTabTake})
	if err != nil {
		t.Fatal(err)
	}
	if len(*asked) == 0 {
		t.Fatal("the take tab asks the proposal")
	}
	pf := (*asked)[len(*asked)-1].ProposeFrom
	if pf.ConnectorOf("ERP") != "apps" || pf.ConnectorOf("HR") != "registry" {
		t.Errorf("each kept value is handed with its own connector: %+v", pf)
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
	if props["title"] != "Proposé d'après vos mots-clés et les applications des sources Carto et Registre" {
		t.Errorf("the proposal title names every followed system: %v", props["title"])
	}
	rows, _ := props["items"].([]any)
	byKey := map[string]map[string]any{}
	for _, r := range rows {
		m := r.(map[string]any)
		byKey[m["key"].(string)] = m
	}
	for key, want := range map[string]struct{ why, label, color string }{
		"P2": {"application « Gestion ERP » de la source Carto", "Carto", "teal"},
		"P3": {"application « Gestion RH » de la source Registre", "Registre", "grape"},
	} {
		row := byKey[key]
		pill, _ := row["pill"].(map[string]any)
		if row == nil || row["why"] != want.why || pill["label"] != want.label || pill["color"] != want.color {
			t.Errorf("page %s names the system that kept its application (%s): %+v", key, want.label, row)
		}
	}
}
