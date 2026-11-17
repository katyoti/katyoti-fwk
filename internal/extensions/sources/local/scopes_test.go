// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

// spaces searches in scopes - its configuration's default one and those a
// carrier adds; fixed declares scopes no carrier adds to; plain declares none
const (
	scopedSpaces = `name: spaces
label: Spaces
description: Pages searched within spaces.
sync: ktw-sp:sp/source-sync
search: ktw-sp:sp/search
scopes: {unit: espace, list: ktw-sp:sp/scopes, add: true}
`
	scopedFixed = `name: fixed
label: Fixed
description: Pages searched within the configured spaces only.
sync: ktw-sp:fixed/source-sync
search: ktw-sp:fixed/search
scopes: {unit: espace, list: ktw-sp:sp/scopes}
`
	scopedPlain = `name: plain
label: Plain
description: No scopes.
sync: ktw-sp:plain/source-sync
search: ktw-sp:plain/search
`
)

// scopeActions answers the scopes method as a connector does: the
// configuration's default space, then the keys its location keeps.
type scopeActions struct{}

func (scopeActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (scopeActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	var value any
	switch in.URI {
	case "ktw-sp:sp/scopes":
		var sin api.ConnectorScopesInput
		_ = json.Unmarshal(in.Input, &sin)
		out := api.ConnectorScopesOutput{Scopes: []api.Scope{{Key: "AUP", Title: "AUP", Default: true}}}
		if keys, ok := sin.Location[api.LocationScopes].([]any); ok {
			for _, k := range keys {
				out.Scopes = append(out.Scopes, api.Scope{Key: fmt.Sprint(k), Title: fmt.Sprint(k)})
			}
		}
		value = out
	case "ktw-agents:sources/source-check":
		svc, err := application.UseService[api.SourcesService](ctx)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		var cin api.SourceCheckInput
		if err := json.Unmarshal(in.Input, &cin); err != nil {
			return actionsapi.RunOutput{}, err
		}
		if value, err = svc.SourceCheck(ctx, cin); err != nil {
			return actionsapi.RunOutput{}, err
		}
	default:
		return actionsapi.RunOutput{}, fmt.Errorf("no method %s", in.URI)
	}
	raw, _ := json.Marshal(value)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// : a connector picker on the Sources tab shows its scopes -
// the panel's picker node carries scopes {list, unit, add, save} bound to
// sources' Scopes and ScopeAdd; a scope the carrier adds is kept in its own
// source's location, once, and the next list answers it beside the default.
func TestAPickerListsACarriersScopesAndKeepsOneItAdds(t *testing.T) {
	root := t.TempDir()
	decls := loader.NewEmbedFSLoader(fstest.MapFS{
		"connectors/spaces.yaml": {Data: []byte(scopedSpaces)},
		"connectors/fixed.yaml":  {Data: []byte(scopedFixed)},
		"connectors/plain.yaml":  {Data: []byte(scopedPlain)},
	}, ".")
	ctx, cwm := cwmapp.NewAt(t, root,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-sp", decls, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return scopeActions{}, nil
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
	src := createSource(t, ctx, cwm, "spaces-src", map[string]any{"connector": "spaces"})
	// A second own source of the same connector - kept by
	// a holder from before one source per connector, the rule gating a
	// creation only: the list and the add both work on the first, never one
	// each.
	second := createSource(t, ctx, cwm, "spaces-src-2", map[string]any{})
	if got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: second}); err != nil {
		t.Fatal(err)
	} else {
		o := got.Object
		o.Fields = map[string]any{"connector": "spaces"}
		if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
			t.Fatal(err)
		}
	}
	createSource(t, ctx, cwm, "fixed-src", map[string]any{"connector": "fixed"})
	createSource(t, ctx, cwm, "plain-src", map[string]any{"connector": "plain"})

	// The panel's picker carries the scopes, bound to sources' methods.
	scopes := func(connector string) map[string]any {
		t.Helper()
		panel, err := svc.ConnectorPanel(ctx, api.ConnectorPanelInput{ID: "m", Connector: connector})
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range panel.DSLTree["children"].([]any) {
			if node, ok := n.(map[string]any); ok && node["kind"] == "picker" {
				s, _ := node["scopes"].(map[string]any)
				return s
			}
		}
		t.Fatalf("%s: the panel holds no picker", connector)
		return nil
	}
	if got := scopes("spaces"); got == nil || got["list"] != api.ScopesURI || got["unit"] != "espace" || got["add"] != true || got["save"] != api.ScopeAddURI {
		t.Errorf("a connector declaring scopes a carrier adds to: picker scopes {list, unit, add, save}: %v", got)
	}
	if got := scopes("fixed"); got == nil || got["list"] != api.ScopesURI || got["add"] != nil || got["save"] != nil {
		t.Errorf("a connector whose scopes no carrier adds to: list only, no add, no save: %v", got)
	}
	if got := scopes("plain"); got != nil {
		t.Errorf("a connector declaring no scopes: no scopes on its picker: %v", got)
	}

	keys := func() string {
		t.Helper()
		out, err := svc.Scopes(ctx, api.ScopesInput{Connector: "spaces", Object: "m"})
		if err != nil {
			t.Fatal(err)
		}
		var k []string
		for _, s := range out.Scopes {
			k = append(k, s.Key)
		}
		return strings.Join(k, ",")
	}
	if got := keys(); got != "AUP" {
		t.Fatalf("before anything is added: the configuration's default: %q", got)
	}
	// A scope added is kept in the carrier's own source location, once.
	for range 2 {
		if res, err := svc.ScopeAdd(ctx, api.ScopeAddInput{Connector: "spaces", Object: "m", Scope: " ARCHI "}); err != nil || !res.OK {
			t.Fatalf("add ARCHI: %+v %v", res, err)
		}
	}
	if got := keys(); got != "AUP,ARCHI" {
		t.Errorf("the next list answers the added scope beside the default, once: %q", got)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: src})
	if err != nil {
		t.Fatal(err)
	}
	var f api.InformationSourceFields
	if err := json.Unmarshal(got.Object.FieldsJSON(), &f); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(f.Location[api.LocationScopes]) != "[ARCHI]" {
		t.Errorf("kept in the source's location scopes field: %v", f.Location)
	}
	got, err = cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: second})
	if err != nil {
		t.Fatal(err)
	}
	var f2 api.InformationSourceFields
	if err := json.Unmarshal(got.Object.FieldsJSON(), &f2); err != nil {
		t.Fatal(err)
	}
	if f2.Location[api.LocationScopes] != nil {
		t.Errorf("the second own source of the connector keeps no added scope: %v", f2.Location)
	}
	// Refused: a connector no carrier adds to, a blank key.
	if _, err := svc.ScopeAdd(ctx, api.ScopeAddInput{Connector: "fixed", Object: "m", Scope: "X"}); err == nil {
		t.Error("a connector whose scopes no carrier adds to refuses an added scope")
	}
	if _, err := svc.ScopeAdd(ctx, api.ScopeAddInput{Connector: "spaces", Object: "m", Scope: "  "}); err == nil {
		t.Error("a blank scope is refused")
	}
	// A connector declaring no scopes lists none.
	if out, err := svc.Scopes(ctx, api.ScopesInput{Connector: "plain", Object: "m"}); err != nil || len(out.Scopes) != 0 {
		t.Errorf("no scopes declared: none listed: %+v %v", out, err)
	}
}
