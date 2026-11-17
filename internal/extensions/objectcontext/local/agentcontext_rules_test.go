// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	oclocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE RULES IN FORCE, on the SHIPPED rule type: a rule reaches the subtree of
// the module it sits in, a `scope: repo` rule reaches every module, and only a
// constraint is told — a recipe is fetched on demand.
func TestARuleReachesItsScopeAndOnlyAConstraintIsTold(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml": {Data: []byte("name: module\n")},
		"object-types/goal.yaml":   {Data: []byte("name: goal\n")},
		"object-types/rule.yaml":   {Data: cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/object-types/rule.yaml")},
	}), oclocal.ExtensionWith(api.ObjectContextConfig{}))
	cwmapp.Module(t, ctx, cwm, "billing")
	cwmapp.Module(t, ctx, cwm, "m")
	mk := func(module, id, typ, title, body string, fields map[string]any) {
		t.Helper()
		o := cwmapi.Object{ID: id, Type: typ, Parent: module, Name: title, Body: body, Fields: fields}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mk("billing", "billing.g1", "goal", "Invoices go out on time", "", nil)
	mk("billing", "billing.R-001", "rule", "Money is integer cents", "Floats round.", map[string]any{"statement": "MUST hold money as integer cents."})
	mk("billing", "billing.R-002", "rule", "How to add a currency", "", map[string]any{"statement": "Add it to the table.", "kind": "recipe"})
	mk("m", "m.R-001", "rule", "MessageBus only", "", map[string]any{"statement": "NEVER import the NATS client.", "scope": "repo"})
	mk("m", "m.R-002", "rule", "Subjects by builder", "", map[string]any{"statement": "Use the builders.", "scope": "module"})

	oc, err := application.UseService[api.ObjectContextService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := agentContext(t, ctx, oc, "billing.g1")
	var rules []string
	for _, s := range out.System {
		if strings.HasPrefix(s.Source, api.LayerRule+":") {
			rules = append(rules, strings.TrimPrefix(s.Source, api.LayerRule+":"))
		}
	}
	// The repo-wide rule first (farthest), then the module's own constraint;
	// m's module-scoped rule and billing's recipe are not told.
	if got, want := strings.Join(rules, " "), "m.R-001 billing.R-001"; got != want {
		t.Fatalf("rules told\n got %s\nwant %s", got, want)
	}
	for _, s := range out.System {
		if s.Source == api.LayerRule+":billing.R-001" {
			if s.Title != "Money is integer cents" || s.Text != "MUST hold money as integer cents.\n\nFloats round." {
				t.Errorf("rule section = %+v, want its title over its statement then its explanation", s)
			}
		}
	}
	// In m, its own module-scoped rule is in force too.
	mk("m", "m.g1", "goal", "M goal", "", nil)
	out = agentContext(t, ctx, oc, "m.g1")
	got := sectionIDs(out)
	if !strings.Contains(got, "rule:m.R-001") || !strings.Contains(got, "rule:m.R-002") || strings.Contains(got, "rule:billing.") {
		t.Fatalf("m sections %s, want both m rules and no billing rule", got)
	}
}
