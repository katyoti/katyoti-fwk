// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A MODULE OFF IN ONE PROJECT ONLY: two served
// projects hold a billing module, the scheduler's Modes names each
// <key>:billing, and alpha's is off. The hold reader names alpha's ready
// to-do held (module off) and not beta's; a person's step of alpha is not
// asked (moduleOff), beta's is. One project served keeps the bare lookup.
func TestAModuleOffInOneProjectHoldsThatProjectsWorkOnly(t *testing.T) {
	const alphaRepo, betaRepo = "0193a0d3-a5ed-71e3-9f71-b810b0466a01", "0193a0d3-a5ed-71e3-9f71-c921c1577b02"
	ctx := context.Background()
	todo := func(id, repo string) api.WorkTodo {
		return api.WorkTodo{Todo: schedapi.Todo{ID: id, Module: "billing", Request: "billing.RQ-001", Repository: repo, For: schedapi.ForAgent}}
	}
	alpha, beta := todo("billing.TD-a", alphaRepo), todo("billing.TD-b", betaRepo)

	modes, several := modesByRef([]schedapi.ModeOutput{{Module: "alpha:billing", Mode: schedapi.ModeOff}, {Module: "beta:billing", Mode: "manual"}})
	if !several {
		t.Fatal("qualified modes must say several projects are served")
	}
	// Each project's key resolved once already (what ObjectResolve answers).
	refs := &projectRefs{ctx: ctx, several: true, keys: map[string]string{alphaRepo: "alpha", betaRepo: "beta"}}
	if !offFor(modes, refs, alpha) || offFor(modes, refs, beta) {
		t.Errorf("off: alpha %v, beta %v - want alpha's billing off, beta's not", offFor(modes, refs, alpha), offFor(modes, refs, beta))
	}

	r := &holdReader{modes: modes, refs: refs, queue: map[string]schedapi.QueueEntry{
		alpha.ID: {Todo: alpha.Todo, Takeable: true}, beta.ID: {Todo: beta.Todo, Takeable: true}}}
	held := func(td api.WorkTodo) bool {
		b := r.ready(ctx, td, api.Blocker{Work: td.Request}, "")
		return b.Kind == api.BlockerHeld && strings.Contains(b.Reason, "is off")
	}
	if !held(alpha) {
		t.Error("alpha's ready to-do must be held: its project's billing is off")
	}
	if held(beta) {
		t.Error("beta's ready to-do must not be held: only alpha's billing is off")
	}

	// One project served: Modes names the module bare, read bare.
	one, several := modesByRef([]schedapi.ModeOutput{{Module: "billing", Mode: schedapi.ModeOff}})
	if several || !offFor(one, &projectRefs{ctx: ctx, keys: map[string]string{}}, alpha) {
		t.Error("one project served: its bare billing off must hold its to-do")
	}
}
