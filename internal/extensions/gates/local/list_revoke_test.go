// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
)

func init() {
	// A gate whose only requirement is for stories: it applies to a story,
	// never to a feature.
	pack["gates/ready-to-ship.yaml"] = &fstest.MapFile{Data: []byte("name: ready-to-ship\nrequirements:\n  - {name: shipped, applies: {types: [story]}, check: \"object.description != ''\"}\n")}
}

func listed(out api.ListOutput) string {
	var parts []string
	for _, g := range out.Gates {
		parts = append(parts, fmt.Sprintf("%s:%d/%d/%d:%v", g.Gate, g.Met, g.Unmet, g.Unproven, g.Passes))
	}
	return strings.Join(parts, " ")
}

// LIST: the gates that apply to an object are the ones with a requirement
// for its type, as Check resolves them, each counted.
func TestListAnswersTheGatesOfAnObjectsType(t *testing.T) {
	ctx, _, g, _ := world(t)
	f, err := g.List(ctx, api.ListInput{ID: "m.f1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := listed(f); got != "ready-to-build:1/3/0:false" {
		t.Errorf("feature: %s — ready-to-ship has no requirement for a feature", got)
	}
	if len(f.Gates[0].Items) != 4 {
		t.Errorf("the summary lost the requirements: %+v", f.Gates[0].Items)
	}
	s, _ := g.List(ctx, api.ListInput{ID: "m.s1"})
	if got := listed(s); got != "ready-to-build:1/2/0:false ready-to-ship:0/1/0:false" {
		t.Errorf("story: %s", got)
	}
	if _, err := g.List(ctx, api.ListInput{}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("no id: %v", err)
	}
}

// REVOKE: a person withdraws a confirmation; the requirement is unmet again
// and the withdrawal is kept, with what was confirmed and who withdrew it.
func TestAPersonRevokesAConfirmationAndTheTraceIsKept(t *testing.T) {
	ctx, cwm, g, _ := world(t)
	in := api.RevokeInput{ID: "m.f1", Gate: "ready-to-build", Requirement: "owner-agrees", By: ana, Note: "scope changed"}
	if _, err := g.Revoke(ctx, in); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Errorf("revoking what was never confirmed: %v", err)
	}
	if _, err := g.Confirm(ctx, api.ConfirmInput{ID: "m.f1", Gate: "ready-to-build", Requirement: "owner-agrees", By: ana, Note: "ok"}); err != nil {
		t.Fatal(err)
	}
	agent := in
	agent.By = bot
	if _, err := g.Revoke(ctx, agent); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("an agent revoked: %v", err)
	}
	out, err := g.Revoke(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if it := out.Checklist.Items[1]; it.Name != "owner-agrees" || it.Status != api.Unmet || it.ConfirmedBy != nil {
		t.Errorf("still confirmed after the withdrawal: %+v", it)
	}
	if r := out.Revoked; r.By != ana || r.At == "" || r.Note != "scope changed" || r.Confirmed.By != ana || r.Confirmed.Note != "ok" {
		t.Errorf("the withdrawal: %+v", r)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.f1"})
	if err != nil {
		t.Fatal(err)
	}
	group := got.Object.Capabilities[api.CapGates]
	if _, ok := group[api.GateKey("ready-to-build", "owner-agrees")]; ok {
		t.Error("the confirmation is still in the group")
	}
	if trace, ok := group[api.RevokedKey].([]any); !ok || len(trace) != 1 {
		t.Errorf("revoked trace: %#v, want one entry", group[api.RevokedKey])
	}
	// The trace never reads as a confirmation: the gate still checks.
	if cl, err := g.Check(ctx, api.CheckInput{ID: "m.f1", Gate: "ready-to-build"}); err != nil || names(cl.Checklist.Items) != "described=unmet,owner-agrees=unmet,scored=met,has-criteria=unmet" {
		t.Errorf("after the withdrawal: %s (%v)", names(cl.Checklist.Items), err)
	}
	// Confirmed again, withdrawn again: the trace grows, it is not replaced.
	if _, err := g.Confirm(ctx, api.ConfirmInput{ID: "m.f1", Gate: "ready-to-build", Requirement: "owner-agrees", By: ana}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Revoke(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, _ = cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.f1"})
	if trace, _ := got.Object.Capabilities[api.CapGates][api.RevokedKey].([]any); len(trace) != 2 {
		t.Errorf("revoked trace after two withdrawals: %d entries", len(trace))
	}
}

// A withdrawal and another write of the same object at once both land, as a
// confirmation's do.
func TestARevocationAndAConcurrentWriteBothLand(t *testing.T) {
	ctx, cwm, g, _ := world(t)
	const n = 20
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("m.v%d", i)
		o := cwmapi.Object{ID: id, Type: "feature", Name: "v", Parent: "m"}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatal(err)
		}
		if _, err := g.Confirm(ctx, api.ConfirmInput{ID: id, Gate: "ready-to-build", Requirement: "owner-agrees", By: ana}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("m.v%d", i)
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := g.Revoke(ctx, api.RevokeInput{ID: id, Gate: "ready-to-build", Requirement: "owner-agrees", By: ana}); err != nil {
				t.Errorf("revoke %s: %v", id, err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: id, Capability: "notes"}); err != nil {
				t.Errorf("add notes to %s: %v", id, err)
			}
		}()
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: fmt.Sprintf("m.v%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		_, notes := got.Object.Capabilities["notes"]
		_, confirmed := got.Object.Capabilities[api.CapGates][api.GateKey("ready-to-build", "owner-agrees")]
		if !notes || confirmed {
			t.Errorf("%s: notes %v, still confirmed %v — one write erased the other", got.Object.ID, notes, confirmed)
		}
	}
}
