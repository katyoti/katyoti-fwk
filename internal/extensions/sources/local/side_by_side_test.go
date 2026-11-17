// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// : one object holds the application sources of two systems
// side by side - both offered and used, both picked, both synced into the
// carrier's input each in its own folder, both listed with their own items,
// and the source following applications by code reads the applications of
// both.
func TestAnObjectHoldsTwoApplicationSourcesSideBySide(t *testing.T) {
	root := t.TempDir()
	acts := roleActions{mu: &sync.Mutex{}, seen: map[string]*api.FromItems{}}
	apps := loader.NewEmbedFSLoader(fstest.MapFS{
		"connectors/map.yaml":     {Data: []byte(roleMap)},
		"connectors/catalog.yaml": {Data: []byte(roleCatalog)},
	}, ".")
	watch := loader.NewEmbedFSLoader(fstest.MapFS{
		"connectors/watch.yaml": {Data: []byte(roleWatch)},
	}, ".")
	ctx, cwm := cwmapp.NewAt(t, root,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-map", apps, "connectors"),
		contributeConnectors("ktw-watch", watch, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return acts, nil
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

	// Both connectors are used on the one object: two sources, side by side.
	ids := map[string]string{}
	for _, c := range []string{"map", "catalog", "watch"} {
		use, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: c})
		if err != nil || !use.OK || !use.Created || use.Source == "" {
			t.Fatalf("use %s: %+v %v", c, use, err)
		}
		ids[c] = use.Source
	}
	if ids["map"] == ids["catalog"] {
		t.Fatalf("the two application connectors share one source: %v", ids)
	}

	// Both are picked, then both are read in; the follower after them.
	picks := map[string]api.SelectedItem{"map": {Key: "erp", Title: "Gestion ERP"}, "catalog": {Key: "crm", Title: "Relation client"}}
	for _, c := range []string{"map", "catalog"} {
		sel, err := svc.SourceSelect(ctx, api.SourceSelectInput{ID: ids[c], Items: []api.SelectedItem{picks[c]}})
		if err != nil || !sel.OK || sel.Items != 1 {
			t.Fatalf("%s pick: %+v %v", c, sel, err)
		}
	}
	for _, c := range []string{"map", "catalog", "watch"} {
		if res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: ids[c]}); err != nil || !res.OK {
			t.Fatalf("%s sync: %+v %v", c, res, err)
		}
	}

	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	// Both connectors are offered as used, each by its own source.
	used := map[string]string{}
	for _, cu := range of.Connectors {
		if cu.Used {
			used[cu.Name] = cu.Source
		}
	}
	for _, c := range []string{"map", "catalog"} {
		if used[c] != ids[c] {
			t.Errorf("the %s connector is not shown used by its source %s: %v", c, ids[c], used)
		}
	}
	// Both are listed, synced, each with its own pick and its own item.
	listed := map[string]api.SourceSummary{}
	for _, s := range of.Sources {
		listed[s.ID] = s
	}
	for _, c := range []string{"map", "catalog"} {
		s, ok := listed[ids[c]]
		if !ok || s.Connector != c || s.State != api.SourceStateSynced || s.Items != 1 || s.Picked != 1 || !strings.Contains(s.PickedLabel, picks[c].Title) {
			t.Errorf("the %s source is not listed synced with its one pick and item: %+v", c, s)
		}
	}
	// The knowledge base input keeps their items apart: each source's
	// folder holds its system's application and not the other's.
	for c, own := range map[string]string{"map": "erp.data.json", "catalog": "crm.data.json"} {
		other := map[string]string{"map": "crm.data.json", "catalog": "erp.data.json"}[c]
		dir := filepath.Join(of.InputPath, localName(ids[c]))
		if _, err := os.Stat(filepath.Join(dir, own)); err != nil {
			t.Errorf("the %s source's item is not in its own folder: %v", c, err)
		}
		if _, err := os.Stat(filepath.Join(dir, other)); err == nil {
			t.Errorf("the %s source's folder holds the other system's item %s", c, other)
		}
	}
	// The source following applications by code reads the applications of both.
	f := acts.seen["ktw-watch:watch/source-sync"]
	if f == nil || strings.Join(sorted(f.Values), ",") != "CRM,ERP" {
		t.Errorf("the following source does not read the applications of both: %+v", f)
	}
}

// localName is an object id's local part - the folder its sync writes under input/.
func localName(id string) string {
	if i := strings.LastIndex(id, "."); i >= 0 {
		return id[i+1:]
	}
	return id
}
