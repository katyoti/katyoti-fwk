// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// GATE SCOPE: a gate entry (before_*, on_transition) applies only where its
// provider owns the object — its type, or a capability the object carries by
// its own choice — unless the universe grants it more. Out of scope it is
// listed not applying, with the grant to add, and the write passes.

// scopeFixture: ktw-docs owns the type `doc`; ktw-notes owns `note`. ktw-spy
// contributes a before_update hook on notes, and a capability `watched` that
// attaches itself to every type with a before_update entry. Every gate refuses.
func scopeFixture(t *testing.T, grants api.Grants) (*Service, stdctx.Context, *hookActions) {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container", Provider: "ktw-docs"},
		{Name: "watched", Provider: "ktw-spy", Attach: &api.ShapeFilter{Types: []string{"*"}},
			Hooks: api.Hooks{api.HookBeforeUpdate: {{URI: "spy:watch/check"}}, api.HookOnUpdate: {{URI: "spy:watch/log"}}}},
	}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "doc", Provider: "ktw-docs", Capabilities: []string{"container"}},
		{Name: "note", Provider: "ktw-notes", Capabilities: []string{"container"}},
		{Name: "spied", Provider: "ktw-spy", Capabilities: []string{"container"}},
	}
	cfg.CatalogTypes.Hooks = []api.Hook{
		{Name: "notes-gate", Provider: "ktw-spy", Type: api.HookBeforeUpdate, URI: "spy:gate/check", Filter: api.ObjectFilter{Types: []string{"note"}}},
	}
	cfg.CatalogTypes.Grants = grants
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	f := &hookActions{replies: map[string]string{
		"spy:gate/check":  `{"accept":false,"reason":"the spy says no"}`,
		"spy:watch/check": `{"accept":false,"reason":"watched says no"}`,
	}}
	app := application.New(application.Config())
	application.PutService[actionsapi.ActionService](app, "default", f)
	ctx := application.WithApplication(stdctx.Background(), app)
	for _, o := range []api.Object{{ID: "m.n", Type: "note"}, {ID: "m.s", Type: "spied"}} {
		o.Name = o.ID
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	return svc, ctx, f
}

func renameNote(ctx stdctx.Context, svc *Service, id string) error {
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: id})
	if err != nil {
		return err
	}
	o := got.Object
	o.Description = "renamed"
	_, err = svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Object: o})
	return err
}

// A foreign before_update is listed, not applying, naming the grant; the write
// passes. The after entry of the same capability still applies (only gates
// are scoped).
func TestAForeignGateIsListedNotApplyingAndTheWritePasses(t *testing.T) {
	svc, ctx, f := scopeFixture(t, nil)
	tbl := table(t, svc, "m.n", api.HookBeforeUpdate)
	for _, uri := range []string{"spy:gate/check", "spy:watch/check"} {
		h, ok := tbl[uri]
		if !ok || h.Applies || h.Provider != "ktw-spy" || !strings.Contains(h.Reason, "grants: {gate:") {
			t.Errorf("%s on a note = %+v, want listed, not applying, from ktw-spy, naming the grant", uri, h)
		}
	}
	if h := table(t, svc, "m.n", api.HookOnUpdate)["spy:watch/log"]; !h.Applies {
		t.Errorf("an after entry is not scoped, got %+v", h)
	}
	if err := renameNote(ctx, svc, "m.n"); err != nil {
		t.Fatalf("the write was refused by an out-of-scope gate: %v", err)
	}
	if n := len(f.callsTo("spy:gate/check")) + len(f.callsTo("spy:watch/check")); n != 0 {
		t.Errorf("an out-of-scope gate was asked %d time(s)", n)
	}
}

// With a grant, the same gate applies and refuses — and the refusal names its
// refuser.
func TestAGrantedGateRefusesNamingItsRefuser(t *testing.T) {
	svc, ctx, _ := scopeFixture(t, api.Grants{"ktw-spy": cenv.GateGrant{Types: []string{"note"}}})
	if h := table(t, svc, "m.n", api.HookBeforeUpdate)["spy:gate/check"]; !h.Applies {
		t.Fatalf("a granted gate does not apply: %+v", h)
	}
	err := renameNote(ctx, svc, "m.n")
	if !errors.Is(err, api.ErrRefusedByHook) {
		t.Fatalf("err = %v, want refused by the granted gate", err)
	}
	if !strings.Contains(err.Error(), "ktw-spy") || !strings.Contains(err.Error(), "hook:ktw-spy/notes-gate") {
		t.Errorf("the refusal must name its refuser (provider and source): %v", err)
	}
}

// A capability that attaches itself everywhere keeps its gate on its
// provider's own type and loses it elsewhere.
func TestAnAttachedGateIsNarrowedToItsProvidersOwnTypes(t *testing.T) {
	svc, ctx, _ := scopeFixture(t, nil)
	if h := table(t, svc, "m.s", api.HookBeforeUpdate)["spy:watch/check"]; !h.Applies {
		t.Errorf("on its provider's own type the attached gate must apply: %+v", h)
	}
	if h := table(t, svc, "m.n", api.HookBeforeUpdate)["spy:watch/check"]; h.Applies {
		t.Errorf("on a foreign type the attached gate must not apply: %+v", h)
	}
	err := renameNote(ctx, svc, "m.s")
	if !errors.Is(err, api.ErrRefusedByHook) || !strings.Contains(err.Error(), "capability:watched") {
		t.Fatalf("on its own type the attached gate refuses, naming capability:watched: %v", err)
	}
	out, _ := svc.DeclarationFindings(ctx, api.DeclarationFindingsInput{})
	var kinds []string
	for _, f := range out.Findings {
		if f.Rule == api.FindingScope {
			kinds = append(kinds, f.Kind+"/"+f.Name)
		}
	}
	if strings.Join(kinds, ",") != "capability/watched,hook/notes-gate" {
		t.Errorf("declaration findings = %v, want the attached capability and the foreign hook", kinds)
	}
}

// shippedGrants are the gates one shipped pack holds on another pack's types,
// each with its reason. At runtime both packs are one provider (their binary),
// so the gate applies; per pack, as this test reads them, it is a grant — and
// one not listed here fails. The published packs hold none; the private build's
// packs add theirs (gate_scope_private_test.go).
type shippedGrant struct {
	grant cenv.GateGrant
	why   string
}

var shippedGrants = map[string]shippedGrant{}

// Every pack the platform ships passes the scope rule: no gate entry reaches
// an object its pack does not own, but for the grants recorded above. Each
// pack is its own provider here — the strictest reading (a binary hosts
// several packs).
func TestShippedPacksGateOnlyWhatTheyOwn(t *testing.T) {
	up := filepath.Join("..", "..", "..", "..", "..")
	packs, _ := filepath.Glob(filepath.Join(up, "app", "*", "embed", "cwm"))
	more, _ := filepath.Glob(filepath.Join(up, "internal", "extensions", "*", "contrib", "embed", "cwm"))
	packs = append(packs, more...)
	if len(packs) < 6 {
		t.Fatalf("found only %d packs — the glob is wrong", len(packs))
	}
	var d api.Declarations
	for _, dir := range packs {
		provider := filepath.ToSlash(dir)
		fsys := os.DirFS(dir)
		ots, err := storage.LoadObjectTypesLayered(fsys)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		for _, o := range ots {
			o.Provider = provider
			d.Objects = append(d.Objects, o)
		}
		caps, err := storage.LoadCapabilitiesLayered(fsys)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		for _, c := range caps {
			c.Provider = provider
			d.Capabilities = append(d.Capabilities, c)
		}
		pols, err := storage.LoadPoliciesLayered(fsys)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		for _, p := range pols {
			p.Provider = provider
			d.Policies = append(d.Policies, p)
		}
		hooks, err := storage.LoadHooksLayered(fsys)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		for _, h := range hooks {
			h.Provider = provider
			d.Hooks = append(d.Hooks, h)
		}
		hts, err := storage.LoadHookTypesLayered(fsys)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		d.HookTypes = append(d.HookTypes, hts...)
	}
	if len(d.Hooks) == 0 || len(d.Capabilities) == 0 {
		t.Fatalf("read %d hooks and %d capabilities — the loaders read nothing", len(d.Hooks), len(d.Capabilities))
	}
	grants := api.Grants{}
	for _, dir := range packs {
		if g, ok := shippedGrants[filepath.ToSlash(strings.TrimPrefix(filepath.ToSlash(dir), filepath.ToSlash(up)+"/"))]; ok {
			grants[filepath.ToSlash(dir)] = g.grant
		}
	}
	if len(grants) != len(shippedGrants) {
		t.Errorf("%d shipped grants apply to a pack, %d are recorded: a recorded pack is gone", len(grants), len(shippedGrants))
	}
	for _, f := range api.ScopeOf(d, grants) {
		t.Errorf("a shipped pack gates what it does not own: %s", f)
	}
	// Falsified: a foreign gate on a shipped type is found.
	d.Hooks = append(d.Hooks, api.Hook{Name: "probe", Provider: "ktw-probe", Type: api.HookBeforeUpdate, URI: "p:q/r",
		Filter: api.ObjectFilter{Types: []string{d.Objects[0].Name}}})
	if len(api.ScopeOf(d, grants)) != 1 {
		t.Errorf("a foreign gate on %s was not found", d.Objects[0].Name)
	}
}
