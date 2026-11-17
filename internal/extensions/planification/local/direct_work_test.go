// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// DIRECT WORK — a fix, or a build at depth quick, serving no want: its module
// is up to date without one. A test case proves it when its behaviour
// changes, or its module-update step says why none can (no_check); its change
// log as any. A full build serving nothing is still refused, and may not skip
// its check. Direct work may ship in a release.
func TestDirectWorkIsUpToDateWithoutAWant(t *testing.T) {
	t.Parallel()
	ctx, _, req := gatedWorld(t, "2020-01-01")
	fix := file(t, ctx, req, api.FileInput{Title: "Fix the invoice label", Module: "billing", Kind: api.KindFix})
	if a, err := req.Assess(ctx, api.AssessInput{ID: fix.ID, Depth: api.DepthQuick, By: intake}); err != nil || a.Proposal != nil {
		t.Fatalf("a quick fix: proposal %+v, %v; want none (direct work serves no want)", a.Proposal, err)
	}
	out, err := req.CheckModuleUpdated(ctx, api.ModuleUpdatedInput{ID: fix.ID})
	if err != nil || out.Accept || strings.Contains(out.Reason, "serves no want") || !strings.Contains(out.Reason, "no acceptance check") {
		t.Fatalf("direct work with nothing recorded: %+v, %v; want refused on its check, not on a want", out, err)
	}
	if _, err := req.ModuleUpdate(ctx, api.ModuleUpdateInput{ID: fix.ID, NoCheck: "a label: no test sees it", Internal: true, Reason: "a typo", By: session}); err != nil {
		t.Fatal(err)
	}
	if out, err := req.CheckModuleUpdated(ctx, api.ModuleUpdatedInput{ID: fix.ID}); err != nil || !out.Accept {
		t.Fatalf("direct work saying why no check: %+v, %v; want up to date", out, err)
	}
	// It may ship in a release, as a correction.
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.1"})
	if _, err := rel.Target(ctx, api.TargetInput{ID: fix.ID, Release: r.ID}); err != nil {
		t.Errorf("direct work targeting a release: %v; want accepted", err)
	}
	if sc, _ := rel.Scope(ctx, api.GetInput{ID: r.ID}); sc.Total != 1 {
		t.Errorf("the release ships %d target(s); want the fix", sc.Total)
	}

	quick := file(t, ctx, req, api.FileInput{Title: "Rename the button", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Assess(ctx, api.AssessInput{ID: quick.ID, Depth: api.DepthQuick, By: intake}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.ModuleUpdate(ctx, api.ModuleUpdateInput{ID: quick.ID, NoCheck: "a label", By: session}); err != nil {
		t.Errorf("a quick build saying why no check: %v; want accepted", err)
	}

	full := file(t, ctx, req, api.FileInput{Title: "Send invoices by mail", Module: "billing", Kind: api.KindBuild})
	if _, err := req.ModuleUpdate(ctx, api.ModuleUpdateInput{ID: full.ID, NoCheck: "trust me", By: session}); err == nil {
		t.Error("a full build skipped its acceptance check")
	}
	if out, err := req.CheckModuleUpdated(ctx, api.ModuleUpdatedInput{ID: full.ID}); err != nil || out.Accept || !strings.Contains(out.Reason, "serves no want") {
		t.Fatalf("a full build serving nothing: %+v, %v; want refused on its want", out, err)
	}
}
