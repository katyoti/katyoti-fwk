// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	actionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/actions/local"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// fnActions answers two capability functions, counted: score() is 3 and
// heavy() — declared costly — is true.
type fnActions struct {
	mu    sync.Mutex
	calls map[string]int
}

func (f *fnActions) Describe(_ context.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{URI: in.URI, ReadOnly: true}, nil
}

func (f *fnActions) Run(_ context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	f.mu.Lock()
	f.calls[in.URI]++
	f.mu.Unlock()
	if in.URI == "x:heavy/get" {
		return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`true`)}, nil
	}
	return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`3`)}, nil
}

func (f *fnActions) count(uri string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[uri]
}

// Listing an object's actions is a preview: a `when:` calling a costly
// function does not run it — CWM answers it decided at the move, and the
// action is listed disabled with that reason — while a cheap function runs.
func TestListingActionsNeverRunsACostlyFunction(t *testing.T) {
	acts := &fnActions{calls: map[string]int{}}
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(fstest.MapFS{
			"capabilities/container.yaml": {Data: []byte("name: container\n")},
			"capabilities/scores.yaml": {Data: []byte("name: scores\nfunctions:\n" +
				"  - {name: score, action: 'x:score/get', returns: number}\n" +
				"  - {name: heavy, action: 'x:heavy/get', returns: boolean, costly: true}\n")},
			"object-types/module.yaml": {Data: []byte("name: module\ncapabilities: [container]\n")},
			"object-types/thing.yaml": {Data: []byte("name: thing\ncapabilities: [container, scores]\nactions:\n" +
				"  - {uri: 'p:tasks/cheap', when: 'score() > 2'}\n" +
				"  - {uri: 'p:tasks/audit', when: 'heavy()'}\n")},
		}),
		actionslocal.SurfaceExtensionWith(func(context.Context) []actionsapi.TaskAction { return nil }),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) { return acts, nil }),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	o := cwmapi.Object{ID: "m.t", Type: "thing", Name: "t", Parent: "m"}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	s, err := application.UseService[actionsapi.SurfaceService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := list(t, ctx, s, actionsapi.ListInput{Scope: cwmapi.ActionScopeObject, ID: "m.t"})
	if a := got["p:tasks/cheap"]; a.Disabled {
		t.Errorf("cheap: %+v — score() is 3", a)
	}
	if a := got["p:tasks/audit"]; !a.Disabled || !strings.Contains(a.DisabledReason, cwmapi.ErrDecidedAtMove.Error()) {
		t.Errorf("audit: %+v — disabled, its condition left undecided", a)
	}
	if n := acts.count("x:heavy/get"); n != 0 {
		t.Errorf("listing the actions ran the costly heavy() %d times", n)
	}
	if n := acts.count("x:score/get"); n != 1 {
		t.Errorf("the cheap score() ran %d times, want once", n)
	}
}
