// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// Most KTSP methods read state and answer immediately; a few FETCH BYTES. The
// default request budget is sized for the former, so installing a 40 MB sidecar
// over a slow link would time out mid-download and report a failure for an
// operation that was progressing fine.
//
// The list is deliberately short. Widening it to "anything that might be slow"
// would hide real hangs behind a ten-minute wait, which is the failure this
// mechanism is supposed to prevent, inverted.
func TestLongRunningKTSP_MatchesByMethodSuffix(t *testing.T) {
	long := []string{
		"ktw-univers:marketplace/install",
		"ktw-univers:marketplace/update",
		"ktw-univers:marketplace/set-version",
		"ktw-univers:marketplace/install-solution",
		"ktw-univers:marketplace/apply-config-set",
		"ktw-univers:marketplace/configure-item",
		"ktw-univers:marketplace/refresh-catalog",
		"ktw-univers:marketplace/install-prereqs",
	}
	for _, p := range long {
		if !isLongRunningKTSP(p) {
			t.Errorf("%s transfers bytes but gets the default budget — it would fail mid-download", p)
		}
	}

	short := []string{
		"ktw-univers:marketplace/search-items",
		"ktw-univers:marketplace/list-installed",
		"ktw-univers:marketplace/outdated",
		"ktw-univers:marketplace/infra-versions",
		"ktw-univers:orchestrator/list-universes",
		// A near-miss that must NOT match: suffix matching, not substring.
		"ktw-univers:marketplace/install-something-else",
	}
	for _, p := range short {
		if isLongRunningKTSP(p) {
			t.Errorf("%s reads state but gets the long budget — a genuine hang would sit for ten minutes", p)
		}
	}
}

// Matching by SUFFIX rather than full wire path means the budget holds for any
// provider or slot serving these verbs, instead of breaking the day a method
// moves to a different slot.
func TestLongRunningKTSP_IsProviderAgnostic(t *testing.T) {
	for _, p := range []string{
		"acme-sidecar:marketplace/install",
		"whatever:some-slot/apply-config-set",
	} {
		if !isLongRunningKTSP(p) {
			t.Errorf("%s should match on the method, not the provider", p)
		}
	}
}

// oneProvider is a catalog holding one provider's manifest.
type oneProvider struct {
	mbapi.Catalog
	man *mbapi.Manifest
}

func (c oneProvider) Get(_ context.Context, name string) (*mbapi.Manifest, error) {
	if name == c.man.Provider {
		return c.man, nil
	}
	return nil, errors.New("not in the catalog")
}

// A GUI call waits as long as its method declares: the
// Ready screen's doctor and plan-prereqs, cut at the transport's 5 s on a
// fresh machine, wait their declared 300 s; a byte-transferring method keeps
// its long budget; a method declaring nothing keeps the transport's default.
func TestACallWaitsAsLongAsItsMethodDeclares(t *testing.T) {
	cat := oneProvider{man: &mbapi.Manifest{Provider: "ktw-univers", SlotTypes: []mbapi.SlotType{{
		Type:    "Marketplace",
		Methods: []mbapi.SlotMethod{{Name: "Doctor", TimeoutS: 300}, {Name: "PlanPrereqs", TimeoutS: 300}, {Name: "Status"}},
	}}}}
	ctx := context.Background()
	for path, want := range map[string]time.Duration{
		"ktw-univers:marketplace/doctor":       300 * time.Second,
		"ktw-univers:marketplace/plan-prereqs": 300 * time.Second,
		"ktw-univers:marketplace/install":      longKTSPCallBudget,
		"ktw-univers:marketplace/status":       0,
	} {
		if got := callBudget(ctx, cat.Get, path); got != want {
			t.Errorf("%s: %v; want %v", path, got, want)
		}
	}
}

// / RQ-063: an action run waits only as long as its target
// declares, plus the hop to ActionService - never a blanket ceiling. A method
// declaring nothing is cut by ActionService at the bus's default, a task
// answers at once with its run: the GUI waits that and the hop. A script's
// slot method advertises its run bound (scripts.yaml timeout_s, else the
// scripts' default), so the GUI waits the run the engine allows.
func TestAnActionRunWaitsAsLongAsItsTargetDeclaresNotABlanketCeiling(t *testing.T) {
	cat := oneProvider{man: &mbapi.Manifest{Provider: "ktw-docs", SlotTypes: []mbapi.SlotType{{
		Type:    "Docs",
		Methods: []mbapi.SlotMethod{{Name: "Render", TimeoutS: 120, Tags: []string{"long"}}, {Name: "Formats"}},
	}, {
		Type:    "scripts",
		Methods: []mbapi.SlotMethod{{Name: "build-site", TimeoutS: 900}, {Name: "quick", TimeoutS: 300}},
	}}}}
	ctx := context.Background()
	for uri, want := range map[string]time.Duration{
		"ktw-docs:docs/render":        120*time.Second + actionRunHop,
		"ktw-docs:docs/formats":       2 * actionRunHop,
		"ktw-docs:scripts/build-site": 900*time.Second + actionRunHop,
		"ktw-docs:scripts/quick":      300*time.Second + actionRunHop,
		"ktw-docs:tasks/publish":      2 * actionRunHop,
		"ktw-other:x/unknown":         2 * actionRunHop,
	} {
		if got := actionRunBudget(ctx, cat.Get, uri); got != want {
			t.Errorf("%s: %v; want %v", uri, got, want)
		}
	}
	if got := actionRunBudget(ctx, nil, "ktw-docs:docs/render"); got != 2*actionRunHop {
		t.Errorf("no catalog: %v; want the undeclared bound %v", got, 2*actionRunHop)
	}
}
