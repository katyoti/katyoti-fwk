// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/methodsservice"
	occontrib "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/contrib"
	panecontrib "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/contrib"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// TestATypeFixesItsReachability: a goal is always asymptotic and a feature
// always closable. Each shipped type declares a lint hook (methods/value-in)
// over its want's reachability; resolved for each object as CWM resolves it and
// run through the methods service, a goal reading closable (or unset, the
// capability's default) and a feature reading asymptotic are error findings,
// and the right ones are clean.
func TestATypeFixesItsReachability(t *testing.T) {
	ctx, cwm := cwmapp.NewStrict(t,
		occontrib.Pack(""),
		panecontrib.Pack(""),
		cwmapp.Pack(os.DirFS(filepath.Join("..", "ktw-agents", "embed", "cwm"))),
		cwmapp.Pack(os.DirFS(filepath.Join("embed", "cwm"))),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	create := func(id, typ, reach string) {
		t.Helper()
		o := cwmapi.Object{ID: id, Type: typ, Parent: "m", Name: id}
		if reach != "" {
			_ = o.SetCapability(cwmapi.CapWant, map[string]any{"reachability": reach})
		}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	cases := []struct {
		id, typ, reach, rule string // rule empty = clean
	}{
		{"m.g-asym", "goal", "asymptotic", ""},
		{"m.g-close", "goal", "closable", "goal-asymptotic"},
		{"m.g-unset", "goal", "", "goal-asymptotic"},
		{"m.f-close", "feature", "closable", ""},
		{"m.f-unset", "feature", "", ""},
		{"m.f-asym", "feature", "asymptotic", "feature-closable"},
	}
	methods := methodsservice.New()
	for _, c := range cases {
		create(c.id, c.typ, c.reach)
		got := lintValueIn(t, ctx, cwm, methods, c.id)
		switch {
		case c.rule == "" && got.Status != "pass":
			t.Errorf("%s: %+v, want clean", c.id, got)
		case c.rule != "" && (got.Status != "fail" || len(got.Findings) != 1 || got.Findings[0].Rule != c.rule || got.Findings[0].Severity != "error"):
			t.Errorf("%s: %+v, want one error finding %s", c.id, got, c.rule)
		}
	}
}

// lintValueIn runs the object's value-in lint hook as CWM would dispatch it:
// the entry its type declares, with the input resolved for the object.
func lintValueIn(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, methods methodsservice.MethodsService, id string) (out struct {
	Status   string
	Findings []struct{ Rule, Severity string }
}) {
	t.Helper()
	hooks, err := cwm.ObjectHooks(ctx, cwmapi.ObjectHooksInput{ID: id, Point: cwmapi.HookLint})
	if err != nil {
		t.Fatalf("hooks of %s: %v", id, err)
	}
	for _, h := range hooks.Hooks {
		if h.URI != "ktw-methods:methods/value-in" || !h.Applies {
			continue
		}
		raw, _ := json.Marshal(h.ResolvedInput)
		var in methodsservice.ValueInInput
		if err := json.Unmarshal(raw, &in); err != nil {
			t.Fatalf("%s: input %s: %v", id, raw, err)
		}
		r, err := methods.ValueIn(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(r)
		_ = json.Unmarshal(b, &out)
		return out
	}
	t.Fatalf("%s: no value-in lint hook in %+v", id, hooks.Hooks)
	return out
}
