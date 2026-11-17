// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// INTAKE: what the intake agent does to a request — its assessment, and the
// tools it is given.

// Assess records the kind, priority and estimate; what it leaves empty is
// kept, and an ended request is not assessed.
func TestIntakeAssessesARequest(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Round the totals", Module: "billing", Kind: api.KindBuild, Estimate: 2})
	got, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Kind: api.KindFix, Priority: "P1", Estimate: 3, By: intake})
	if err != nil {
		t.Fatal(err)
	}
	if f := got.Request; f.Kind != api.KindFix || f.Priority != "P1" || f.Estimate != 3 {
		t.Errorf("assessed = %+v; want fix, P1, 3", f.RequestFields)
	}
	if got, _ := req.Assess(ctx, api.AssessInput{ID: r.ID, Priority: "P0", By: intake}); got.Request.Kind != api.KindFix || got.Request.Estimate != 3 {
		t.Errorf("a second assessment lost what it left empty: %+v", got.Request.RequestFields)
	}
	if _, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Priority: "P5", By: intake}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("priority P5: %v, want refused", err)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: r.ID, Outcome: api.PlanDropped, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Kind: api.KindFix, By: intake}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("assessing a dropped request: %v, want refused", err)
	}
}

// The requests tools: one per verb intake needs, and requests-file (a module
// asks another) and requests-answer (a question answered) — nothing else.
func TestTheIntakeTools(t *testing.T) {
	t.Parallel()
	var names []string
	for _, tool := range planlocal.IntakeToolGroup() {
		names = append(names, tool.ToolDef().Name)
	}
	sort.Strings(names)
	want := "requests-answer,requests-ask,requests-assess,requests-block,requests-depend,requests-file,requests-finish,requests-get,requests-list,requests-split"
	if got := strings.Join(names, ","); got != want {
		t.Errorf("intake tools = %s, want %s", got, want)
	}
}

// The intake tools run headless: they are allowed without a permission
// prompt (no one would answer it).
func TestTheIntakeToolsNeedNoPrompt(t *testing.T) {
	t.Parallel()
	for _, tool := range planlocal.IntakeToolGroup() {
		if got := tool.CheckPermissions(context.Background(), []byte(`{}`)); got.Behavior != toolsapi.PermissionAllow {
			t.Errorf("%s: permission %v, want allowed", tool.ToolDef().Name, got.Behavior)
		}
	}
}
