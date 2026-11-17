// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
	verificationlocal "github.com/katyoti/katyoti-fwk/internal/extensions/verification/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// DONE MEANS VERIFIED: a workflow that gates `finish` on
// `verify().green && lint().blocking == 0` — the verification service's two
// functions, declared by the shipped `verification` capability (the shipped
// feature reaches done through the ready-to-test gate, whose requirements ask
// the same two functions). A failing or
// unproven criterion refuses, nothing to verify refuses, a lint finding blocks
// at or above the type's gate_blocks_on, and the refusal says what the
// functions answered.

const (
	verifyHook = "x:acceptance/verify"
	lintHook   = "x:rules/lint"
	// The shipped capability names its functions host-relative; cwmapp hosts
	// it under ktw-test, so they resolve to ktw-test.
	verifyFn = "ktw-test:verification/verify-function"
	lintFn   = "ktw-test:verification/lint-function"
)

// actions answers the two hooks with canned replies and routes the two
// functions to the verification service, read-only as their methods declare.
type actions struct {
	mu      sync.Mutex
	replies map[string]string
	calls   map[string]int
}

func (a *actions) Describe(_ context.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	m := api.VerificationServiceDef.Methods // what each function's method declares
	return actionsapi.DescribeOutput{URI: in.URI, ReadOnly: (in.URI == verifyFn && m["VerifyFunction"].ReadOnly) || (in.URI == lintFn && m["LintFunction"].ReadOnly)}, nil
}

func (a *actions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	a.mu.Lock()
	a.calls[in.URI]++
	a.mu.Unlock()
	if in.URI != verifyFn && in.URI != lintFn {
		return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(a.replies[in.URI])}, nil
	}
	var call cwmapi.FunctionCall
	if err := json.Unmarshal(in.Input, &call); err != nil {
		return actionsapi.RunOutput{}, err
	}
	v, err := application.UseService[api.VerificationService](ctx)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	var out any
	if in.URI == verifyFn {
		out, err = v.VerifyFunction(ctx, call)
	} else {
		out, err = v.LintFunction(ctx, call)
	}
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	raw, _ := json.Marshal(out)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

func (a *actions) count(uri string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls[uri]
}

const (
	verifyPass = `{"status":"pass"}`
	verifyFail = `{"status":"fail","evidence":"1 != 2"}`
	lintClean  = `{"status":"ok","findings":[]}`
	lintWarn   = `{"status":"findings","findings":[{"rule":"r","message":"m","severity":"warn"}]}`
)

var person = cwmapi.Actor{ID: "ana", Kind: cwmapi.ActorPerson}

// deliveryWorkflow: approved by a person, worked on, and done only when its
// verification is green and no blocking lint finding remains; anyone may force
// the finish, the forced pass recorded.
const deliveryWorkflow = `name: delivery
initial: idea
states:
  - {name: idea}
  - {name: proposed}
  - {name: in-review}
  - {name: accepted}
  - {name: active}
  - {name: done, terminal: true}
  - {name: dropped, terminal: true}
transitions:
  - {name: propose, from: [idea], to: proposed}
  - {name: review, from: [proposed], to: in-review}
  - {name: accept, from: [in-review], to: accepted, gate: "by.kind == 'person'"}
  - {name: start, from: [accepted], to: active}
  - {name: finish, from: [active], to: done, gate: "verify().green && lint().blocking == 0", force: anyone}
  - {name: drop, from: ["*"], to: dropped}
`

// deliveryWorld: a feature (a lint hook on its type, the given bar) with one
// checkable criterion under it, moved by a person to `active`.
func deliveryWorld(t *testing.T, bar, verify, lint string) (context.Context, cwmapi.CWMService, *actions) {
	t.Helper()
	acts := &actions{replies: map[string]string{verifyHook: verify, lintHook: lint}, calls: map[string]int{}}
	feature := "name: feature\ncapabilities: [container, lifecycle]\nworkflow: delivery\nhooks:\n  lint:\n    - uri: " + lintHook + "\n"
	if bar != "" {
		feature += "verification:\n  gate_blocks_on: " + bar + "\n"
	}
	pack := fstest.MapFS{
		"capabilities/container.yaml":    {Data: []byte("name: container\n")},
		"capabilities/lifecycle.yaml":    {Data: []byte("name: lifecycle\n")},
		"capabilities/checkable.yaml":    {Data: []byte("name: checkable\nhooks:\n  verify:\n    - uri: " + verifyHook + "\n")},
		"capabilities/verification.yaml": {Data: cwmapp.ShippedFile(t, "internal/extensions/verification/contrib/embed/cwm/capabilities/verification.yaml")},
		"workflows/delivery.yaml":        {Data: []byte(deliveryWorkflow)},
		"object-types/module.yaml":       {Data: []byte("name: module\ncapabilities: [container]\n")},
		"object-types/feature.yaml":      {Data: []byte(feature)},
		"object-types/criterion.yaml":    {Data: []byte("name: criterion\ncapabilities: [container, checkable]\n")},
	}
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(pack),
		verificationlocal.Extension(),
		panelocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) { return acts, nil }),
	)
	cwmapp.Module(t, ctx, cwm, "billing")
	for _, o := range []cwmapi.Object{{ID: "billing.f1", Type: "feature"}, {ID: "billing.ac1", Type: "criterion"}} {
		o.Name = o.ID
		o.Parent = map[string]string{"billing.f1": "billing", "billing.ac1": "billing.f1"}[o.ID]
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	for _, to := range []string{"proposed", "in-review", "accepted", "active"} {
		if _, err := move(ctx, cwm, to, false); err != nil {
			t.Fatalf("→ %s: %v", to, err)
		}
	}
	return ctx, cwm, acts
}

func move(ctx context.Context, cwm cwmapi.CWMService, to string, force bool) (cwmapi.ObjectTransitionOutput, error) {
	return cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: "billing.f1", To: to, By: person, Force: force})
}

func TestDoneNeedsAGreenVerification(t *testing.T) {
	for name, reply := range map[string]string{`"failed":1`: verifyFail, `"unproven":1`: `{"verdict":"x"}`} {
		ctx, cwm, _ := deliveryWorld(t, "", reply, lintClean)
		_, err := move(ctx, cwm, "done", false)
		if !errors.Is(err, cwmapi.ErrTransitionRefused) || !strings.Contains(err.Error(), name) {
			t.Errorf("finish = %v, want refused saying %s", err, name)
		}
	}
	ctx, cwm, _ := deliveryWorld(t, "", verifyPass, lintClean)
	if out, err := move(ctx, cwm, "done", false); err != nil || out.Forced {
		t.Fatalf("a verified feature: finish = %+v, %v", out, err)
	}
}

func TestNothingToVerifyIsNotDone(t *testing.T) {
	ctx, cwm, _ := deliveryWorld(t, "", verifyPass, lintClean)
	if _, err := cwm.ObjectDelete(ctx, cwmapi.ObjectDeleteInput{Module: "billing", ID: "billing.ac1"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := move(ctx, cwm, "done", false); !errors.Is(err, cwmapi.ErrTransitionRefused) || !strings.Contains(err.Error(), `"runs":0`) {
		t.Fatalf("finish with nothing verified = %v, want refused saying runs 0", err)
	}
}

func TestGateBlocksOnIsTheLintBar(t *testing.T) {
	ctx, cwm, _ := deliveryWorld(t, "", verifyPass, lintWarn)
	if _, err := move(ctx, cwm, "done", false); err != nil {
		t.Fatalf("a warn under the error bar: %v", err)
	}
	ctx, cwm, _ = deliveryWorld(t, "warn", verifyPass, lintWarn)
	if _, err := move(ctx, cwm, "done", false); !errors.Is(err, cwmapi.ErrTransitionRefused) || !strings.Contains(err.Error(), `"blocking":1`) {
		t.Fatalf("a warn at the warn bar = %v, want refused saying blocking 1", err)
	}
}

func TestAForcedFinishIsRecorded(t *testing.T) {
	ctx, cwm, _ := deliveryWorld(t, "", verifyFail, lintClean)
	out, err := move(ctx, cwm, "done", true)
	if err != nil || !out.Forced {
		t.Fatalf("forced finish = %+v, %v", out, err)
	}
	// The move is recorded as forced on its event — the journal's record;
	// the object stores the state alone.
	if life, _ := out.Object.Lifecycle(); life.State != "done" || out.To != "done" {
		t.Fatalf("lifecycle = %+v, want done (forced)", life)
	}
}

// The earlier moves (no guard, or a guard on `by`) called neither function,
// and the options leave the costly finish guard to the move.
func TestOnlyTheFinishGuardVerifies(t *testing.T) {
	ctx, _, acts := deliveryWorld(t, "", verifyPass, lintClean)
	if n := acts.count(verifyFn) + acts.count(lintFn) + acts.count(verifyHook) + acts.count(lintHook); n != 0 {
		t.Fatalf("%d calls before finish — a guard that names no function calls none", n)
	}
	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	opts, err := pane.TransitionOptions(ctx, paneapi.TransitionOptionsInput{ID: "billing.f1", By: person})
	if err != nil {
		t.Fatal(err)
	}
	for _, lo := range opts.Lifecycles {
		for _, o := range lo.Transitions {
			if o.Name == "finish" && o.GuardStatus != paneapi.GuardAtMove {
				t.Errorf("finish option: %s, want decided at the move (verify() and lint() are costly)", o.GuardStatus)
			}
		}
	}
	if n := acts.count(verifyFn) + acts.count(lintFn); n != 0 {
		t.Errorf("listing the options called the costly functions %d times", n)
	}
}
