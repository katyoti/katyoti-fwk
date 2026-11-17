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
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	gatescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/gates/contrib"
	gateslocal "github.com/katyoti/katyoti-fwk/internal/extensions/gates/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// heavyActions answers heavy() — declared costly — with true, counted.
type heavyActions struct {
	mu sync.Mutex
	n  int
}

func (h *heavyActions) Describe(_ context.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{URI: in.URI, ReadOnly: true}, nil
}

func (h *heavyActions) Run(_ context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	h.mu.Lock()
	h.n++
	h.mu.Unlock()
	return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`true`)}, nil
}

func (h *heavyActions) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.n
}

// A gate in a preview: gate() called by a preview (FunctionCall.NoCostly)
// checks with ObjectEvaluate's NoCostly, so a check calling a costly function
// is not run and reads unproven, decided at the move. A gate kept from
// passing only by such checks is undecided — gate() answers ErrDecidedAtMove;
// one failing on anything else fails. Outside a preview the check runs.
func TestAGateInAPreviewLeavesACostlyCheckToTheMove(t *testing.T) {
	heavy := &heavyActions{}
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(fstest.MapFS{
			"capabilities/container.yaml": {Data: []byte("name: container\n")},
			"capabilities/audits.yaml":    {Data: []byte("name: audits\nfunctions:\n  - {name: heavy, action: 'x:heavy/get', returns: boolean, costly: true}\n")},
			"object-types/module.yaml":    {Data: []byte("name: module\ncapabilities: [container]\n")},
			"object-types/thing.yaml":     {Data: []byte("name: thing\ncapabilities: [container, audits]\n")},
			"gates/audited.yaml":          {Data: []byte("name: audited\nrequirements:\n  - {name: audit, check: 'heavy()'}\n")},
			"gates/mixed.yaml": {Data: []byte("name: mixed\nrequirements:\n  - {name: audit, check: 'heavy()'}\n" +
				"  - {name: described, check: \"object.description != ''\"}\n")},
		}),
		gatescontrib.Pack(""),
		gateslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) { return heavy, nil }),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	o := cwmapi.Object{ID: "m.t", Type: "thing", Name: "t", Parent: "m"}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	g, err := application.UseService[api.GatesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	preview := cwmapi.FunctionCall{Function: "gate", Object: "m.t", NoCostly: true}

	preview.Args = []any{"audited"}
	if _, err := g.GateFunction(ctx, preview); !cwmapi.IsDecidedAtMove(err) {
		t.Errorf("gate('audited') in a preview: %v — undecided, decided at the move", err)
	}
	preview.Args = []any{"mixed"}
	cl, err := g.GateFunction(ctx, preview)
	if err != nil || cl.Passes || names(cl.Items) != "audit=unproven,described=unmet" ||
		!strings.Contains(cl.Items[0].Reason, cwmapi.ErrDecidedAtMove.Error()) {
		t.Errorf("gate('mixed') in a preview: %s, %v — it fails on its description whatever the audit says", names(cl.Items), err)
	}
	if n := heavy.count(); n != 0 {
		t.Errorf("a preview's gate ran the costly heavy() %d times", n)
	}

	cl, err = g.GateFunction(ctx, cwmapi.FunctionCall{Function: "gate", Object: "m.t", Args: []any{"audited"}})
	if err != nil || !cl.Passes || heavy.count() != 1 {
		t.Errorf("gate('audited') at a move: %+v, %v, %d runs — heavy() runs and holds", cl, err, heavy.count())
	}
}
