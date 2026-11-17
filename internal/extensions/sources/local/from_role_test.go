// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
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

// Two application connectors of two systems, both playing the application
// role; watch follows the role, probe names both connectors, legacy keeps
// one connector - the shape every declaration had before.
const (
	roleMap = `name: map
label: Map
description: The applications of one map.
sync: ktw-map:map/source-sync
roles: [application]
`
	roleCatalog = `name: catalog
label: Catalog
description: The applications of another catalog.
sync: ktw-catalog:catalog/source-sync
roles: [application]
`
	roleWatch = `name: watch
label: Watch
description: The monitored entities of the kept applications.
sync: ktw-watch:watch/source-sync
from: {role: application, attr: code}
`
	roleProbe = `name: probe
label: Probe
description: The probes of the kept applications.
sync: ktw-watch:probe/source-sync
from: {connectors: [map, catalog], attr: code}
`
	roleLegacy = `name: legacy
label: Legacy
description: The entities of the map's applications only.
sync: ktw-watch:legacy/source-sync
from: {connector: map, attr: code}
`
)

// roleActions brings applications with their code from map (ERP) and
// catalog (CRM), and records what each following connector's sync received.
type roleActions struct {
	mu   *sync.Mutex
	seen map[string]*api.FromItems
}

func (roleActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a roleActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	var sin api.ConnectorSyncInput
	_ = json.Unmarshal(in.Input, &sin)
	bring := func(code, title string) (any, error) {
		name := strings.ToLower(code)
		if err := os.WriteFile(filepath.Join(sin.Dest, name+".data.json"), []byte(`{"code":"`+code+`"}`), 0o644); err != nil {
			return nil, err
		}
		return api.ConnectorSyncOutput{Items: []api.ConnectorItem{{Kind: "app", Data: name + ".data.json", URI: "example://" + name, Title: title, Key: name}}}, nil
	}
	var value any
	var err error
	switch in.URI {
	case "ktw-map:map/source-sync":
		value, err = bring("ERP", "Gestion ERP")
	case "ktw-catalog:catalog/source-sync":
		value, err = bring("CRM", "Relation client")
	case "ktw-watch:watch/source-sync", "ktw-watch:probe/source-sync", "ktw-watch:legacy/source-sync":
		a.mu.Lock()
		a.seen[in.URI] = sin.From
		a.mu.Unlock()
		if err = os.WriteFile(filepath.Join(sin.Dest, "host.md"), []byte("# host\n"), 0o644); err == nil {
			value = api.ConnectorSyncOutput{Items: []api.ConnectorItem{{Text: "host.md", URI: "example://host", Title: "host"}}}
		}
	case "ktw-agents:sources/source-check":
		svc, e := application.UseService[api.SourcesService](ctx)
		if e != nil {
			return actionsapi.RunOutput{}, e
		}
		var cin api.SourceCheckInput
		if e := json.Unmarshal(in.Input, &cin); e != nil {
			return actionsapi.RunOutput{}, e
		}
		value, err = svc.SourceCheck(ctx, cin)
	default:
		return actionsapi.RunOutput{}, fmt.Errorf("no method %s", in.URI)
	}
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	raw, _ := json.Marshal(value)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// : a source reading from another follows every application
// source of its carrier by its code, whichever connector brought it - by
// the role its connectors play, or by naming several connectors; a
// declaration naming one connector still follows that one alone.
func TestAFollowingSourceFollowsEveryApplicationSource(t *testing.T) {
	root := t.TempDir()
	acts := roleActions{mu: &sync.Mutex{}, seen: map[string]*api.FromItems{}}
	apps := loader.NewEmbedFSLoader(fstest.MapFS{
		"connectors/map.yaml":     {Data: []byte(roleMap)},
		"connectors/catalog.yaml": {Data: []byte(roleCatalog)},
	}, ".")
	watch := loader.NewEmbedFSLoader(fstest.MapFS{
		"connectors/watch.yaml":  {Data: []byte(roleWatch)},
		"connectors/probe.yaml":  {Data: []byte(roleProbe)},
		"connectors/legacy.yaml": {Data: []byte(roleLegacy)},
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

	// The role is resolved when the connectors are listed: watch follows
	// both application connectors, legacy its one.
	list, err := svc.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string][]string{}
	for _, c := range list.Connectors {
		names[c.Name] = c.From.Names()
	}
	if got := strings.Join(sorted(names["watch"]), ","); got != "catalog,map" {
		t.Errorf("a from naming the application role follows every connector playing it: %q", got)
	}
	if got := strings.Join(names["legacy"], ","); got != "map" {
		t.Errorf("a from naming one connector reads as a list of one: %q", got)
	}

	// One carrier holds an application source of each system.
	for local, connector := range map[string]string{"map-src": "map", "catalog-src": "catalog"} {
		id := createSource(t, ctx, cwm, local, map[string]any{"connector": connector})
		if res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: id}); err != nil || !res.OK {
			t.Fatalf("%s sync: %+v %v", connector, res, err)
		}
	}
	for local, connector := range map[string]string{"watch-src": "watch", "probe-src": "probe", "legacy-src": "legacy"} {
		id := createSource(t, ctx, cwm, local, map[string]any{"connector": connector})
		if res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: id}); err != nil || !res.OK {
			t.Fatalf("%s sync: %+v %v", connector, res, err)
		}
	}
	for uri, want := range map[string]string{
		"ktw-watch:watch/source-sync":  "CRM,ERP",
		"ktw-watch:probe/source-sync":  "CRM,ERP",
		"ktw-watch:legacy/source-sync": "ERP",
	} {
		f := acts.seen[uri]
		if f == nil || f.Attr != "code" || strings.Join(sorted(f.Values), ",") != want {
			t.Errorf("%s receives the codes %s: %+v", uri, want, f)
			continue
		}
		for _, code := range strings.Split(want, ",") {
			if f.Title(code) == code {
				t.Errorf("%s names %s by its application's title: %+v", uri, code, f)
			}
		}
	}
}
