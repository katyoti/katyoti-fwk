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
	"github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	gatescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/gates/contrib"
	gateslocal "github.com/katyoti/katyoti-fwk/internal/extensions/gates/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// A `uri:` REQUIREMENT is an action answering {accept, reason} over the
// object. Accept is met; a refusal is unmet with the action's reason (or "did
// not accept"); an action that fails, answers nothing or answers no JSON, or
// no ActionService at all, is unproven — a check that did not run has not
// passed.

var uriPack = fstest.MapFS{
	"capabilities/container.yaml": {Data: []byte("name: container\n")},
	"object-types/module.yaml":    {Data: []byte("name: module\ncapabilities: [container]\n")},
	"object-types/doc.yaml":       {Data: []byte("name: doc\ncapabilities: [container]\n")},
	"gates/reviewed.yaml": {Data: []byte(`name: reviewed
requirements:
  - {name: accepts, uri: "x:review/accepts"}
  - {name: refuses, uri: "x:review/refuses"}
  - {name: refuses-silently, uri: "x:review/refuses-silently"}
  - {name: fails, uri: "x:review/fails"}
  - {name: empty, uri: "x:review/empty"}
  - {name: not-json, uri: "x:review/not-json"}
`)},
}

// verdicts answers each review URI its way and records what it was asked.
type verdicts struct {
	mu    sync.Mutex
	asked map[string]json.RawMessage
}

func (*verdicts) Describe(_ context.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{URI: in.URI, ReadOnly: true}, nil
}

func (v *verdicts) Run(_ context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	v.mu.Lock()
	v.asked[in.URI] = in.Input
	v.mu.Unlock()
	out := actionsapi.RunOutput{URI: in.URI}
	switch in.URI {
	case "x:review/accepts":
		out.Value = json.RawMessage(`{"accept": true}`)
	case "x:review/refuses":
		out.Value = json.RawMessage(`{"accept": false, "reason": "no reviewer signed"}`)
	case "x:review/refuses-silently":
		out.Value = json.RawMessage(`{"accept": false}`)
	case "x:review/fails":
		return actionsapi.RunOutput{}, errors.New("review service down")
	case "x:review/empty":
	case "x:review/not-json":
		out.Value = json.RawMessage(`"maybe"`)
	}
	return out, nil
}

func uriWorld(t *testing.T, withActions *verdicts) (context.Context, api.GatesService) {
	t.Helper()
	exts := []application.Extension{cwmapp.Pack(uriPack), gatescontrib.Pack(""), gateslocal.Extension()}
	if withActions != nil {
		exts = append(exts, application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) { return withActions, nil }))
	}
	ctx, cwm := cwmapp.New(t, exts...)
	cwmapp.Module(t, ctx, cwm, "m")
	o := cwmapi.Object{ID: "m.d1", Type: "doc"}
	o.Name, o.Parent = "d1", "m"
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	g, err := application.UseService[api.GatesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, g
}

func TestAURIRequirementTakesItsStatusFromTheActionsVerdict(t *testing.T) {
	v := &verdicts{asked: map[string]json.RawMessage{}}
	ctx, g := uriWorld(t, v)
	out, err := g.Check(ctx, api.CheckInput{ID: "m.d1", Gate: "reviewed"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct{ status, reason string }{
		"accepts":          {api.Met, ""},
		"refuses":          {api.Unmet, "no reviewer signed"},
		"refuses-silently": {api.Unmet, "did not accept"},
		"fails":            {api.Unproven, "review service down"},
		"empty":            {api.Unproven, "x:review/empty answered no verdict"},
		"not-json":         {api.Unproven, "x:review/not-json answered no verdict"},
	}
	if len(out.Checklist.Items) != len(want) {
		t.Fatalf("items: %s", names(out.Checklist.Items))
	}
	for _, it := range out.Checklist.Items {
		w := want[it.Name]
		if it.Kind != "uri" || it.Status != w.status || it.Reason != w.reason {
			t.Errorf("%s: kind=%s status=%s reason=%q, want uri %s %q", it.Name, it.Kind, it.Status, it.Reason, w.status, w.reason)
		}
	}
	if out.Checklist.Passes {
		t.Error("a gate with unmet and unproven items passes")
	}
	var asked struct {
		Point  string        `json:"point"`
		Object cwmapi.Object `json:"object"`
	}
	if err := json.Unmarshal(v.asked["x:review/accepts"], &asked); err != nil || asked.Point != "gate" || asked.Object.ID != "m.d1" {
		t.Errorf("the action was asked %s (%v), want {point: gate, object: m.d1}", v.asked["x:review/accepts"], err)
	}
}

func TestAURIRequirementWithoutAnActionServiceIsUnproven(t *testing.T) {
	ctx, g := uriWorld(t, nil)
	out, err := g.Check(ctx, api.CheckInput{ID: "m.d1", Gate: "reviewed"})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range out.Checklist.Items {
		if it.Status != api.Unproven || !strings.HasPrefix(it.Reason, "no action service to run x:review/") {
			t.Errorf("%s: %s %q, want unproven for want of an action service", it.Name, it.Status, it.Reason)
		}
	}
	if out.Checklist.Passes {
		t.Error("an unrun gate passes")
	}
}
