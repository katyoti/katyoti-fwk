// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	decisionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// evaluationWorld boots a real CWM with the grid and evaluation declarations
// AS SHIPPED by ktw-methods, a type for the alternatives, and the service.
func evaluationWorld(t *testing.T) (context.Context, cwmapi.CWMService, api.DecisionsService) {
	t.Helper()
	shipped := func(rel string) *fstest.MapFile {
		return &fstest.MapFile{Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/"+rel)}
	}
	ctx, cwm := cwmapp.NewStrict(t, cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml":              {Data: []byte("name: module\ntree: node\n")},
		"object-types/solution.yaml":            {Data: []byte("name: solution\ntree: node\naccept_parents: [\"*\"]\n")},
		"capabilities/lifecycle.yaml":           {Data: []byte("name: lifecycle\n")},
		"capabilities/pane.yaml":                {Data: cwmapp.ShippedFile(t, "internal/extensions/objectpane/contrib/embed/cwm/capabilities/pane.yaml")},
		"object-types/evaluation-grid.yaml":     shipped("object-types/evaluation-grid.yaml"),
		"object-types/solution-evaluation.yaml": shipped("object-types/solution-evaluation.yaml"),
		"workflows/evaluation-grid.yaml":        shipped("workflows/evaluation-grid.yaml"),
		"workflows/solution-evaluation.yaml":    shipped("workflows/solution-evaluation.yaml"),
		"edge-types/evaluates.yaml":             shipped("edge-types/evaluates.yaml"),
		"edge-types/scored-on.yaml":             shipped("edge-types/scored-on.yaml"),
	}), decisionslocal.Extension())
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.DecisionsService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc
}

func createWith(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, typ, name string, fields any, edges ...cwmapi.ObjectEdge) string {
	t.Helper()
	obj := cwmapi.Object{Type: typ, Name: name, Parent: "m", Edges: edges}
	if fields != nil {
		raw, _ := json.Marshal(fields)
		obj.SetFieldsJSON(raw)
	}
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: obj})
	if err != nil {
		t.Fatalf("create %s %q: %v", typ, name, err)
	}
	return out.Object.ID
}

func stateOf(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) string {
	t.Helper()
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: id})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := got.Object.Capabilities["lifecycle"]["state"].(string)
	return st
}

var architect = cwmapi.Actor{ID: "architect", Kind: cwmapi.ActorPerson}

func grid(weights ...float64) api.EvaluationGrid {
	g := api.EvaluationGrid{Criteria: []api.Criterion{
		{ID: "hosting-fr", Label: "Données hébergées en France", Category: "compliance", Kind: api.CriterionEliminatory},
	}}
	for i, w := range weights {
		g.Criteria = append(g.Criteria, api.Criterion{ID: []string{"reliability", "tco", "skills"}[i], Label: "c", Category: "technical", Kind: api.CriterionScored, Weight: w})
	}
	return g
}

// A grid is frozen only when its weights add up to 100; reopening a frozen
// grid sends every evaluation scored on it back to draft.
func TestEvaluationGrid(t *testing.T) {
	ctx, cwm, svc := evaluationWorld(t)
	g := createWith(t, ctx, cwm, api.GridType, "Comité technique", grid(50, 40))

	out, err := svc.GridFreeze(ctx, api.GridMoveInput{ID: g, By: architect})
	if err != nil || out.OK || out.WeightsTotal != 90 || !strings.Contains(out.Message, "90") {
		t.Fatalf("a grid at 90 %% was frozen, or did not say why: %v %+v", err, out)
	}
	if st := stateOf(t, ctx, cwm, g); st == api.GridFrozen {
		t.Fatalf("a refused freeze moved the grid")
	}

	// Corrected to 100: frozen.
	got, _ := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: g})
	raw, _ := json.Marshal(grid(50, 30, 20))
	o := got.Object
	o.SetFieldsJSON(raw)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o, Revision: got.Revision}); err != nil {
		t.Fatal(err)
	}
	if out, err := svc.GridFreeze(ctx, api.GridMoveInput{ID: g, By: architect}); err != nil || !out.OK {
		t.Fatalf("a grid at 100 %% was not frozen: %v %+v", err, out)
	}
	if st := stateOf(t, ctx, cwm, g); st != api.GridFrozen {
		t.Fatalf("state %q, want frozen", st)
	}

	// An evaluation reviewed on it goes back to draft when the grid reopens.
	alt := createWith(t, ctx, cwm, "solution", "Status quo", nil)
	ev := createWith(t, ctx, cwm, api.EvaluationType, "Status quo", api.SolutionEvaluation{Scores: []api.Score{{Criterion: "hosting-fr", Score: 1}}},
		cwmapi.ObjectEdge{Kind: api.EvaluatesEdge, To: alt}, cwmapi.ObjectEdge{Kind: api.ScoredOnEdge, To: g})
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: ev, To: api.EvaluationReviewed, By: architect}); err != nil {
		t.Fatal(err)
	}
	re, err := svc.GridReopen(ctx, api.GridMoveInput{ID: g, By: architect, Note: "un critère de sécurité manquait"})
	if err != nil || len(re.Reopened) != 1 || re.Reopened[0] != ev {
		t.Fatalf("reopen: %v %+v", err, re)
	}
	if st := stateOf(t, ctx, cwm, g); st != api.GridDraft {
		t.Errorf("grid state %q after reopen, want draft", st)
	}
	if st := stateOf(t, ctx, cwm, ev); st != api.EvaluationDraft {
		t.Errorf("evaluation state %q after the grid reopened, want draft", st)
	}
}

// An evaluation's total is computed on its frozen grid: weighted, an
// eliminatory criterion scored 0 eliminates, a criterion not scored counts 0;
// on a draft grid nothing is computed; the matrix ranks every alternative,
// the eliminated last.
func TestSolutionEvaluation(t *testing.T) {
	ctx, cwm, svc := evaluationWorld(t)
	g := createWith(t, ctx, cwm, api.GridType, "Comité technique", grid(50, 30, 20))
	evaluate := func(name string, scores map[string]int) string {
		alt := createWith(t, ctx, cwm, "solution", name, nil)
		var sc []api.Score
		for _, c := range []string{"hosting-fr", "reliability", "tco", "skills"} {
			if v, ok := scores[c]; ok {
				sc = append(sc, api.Score{Criterion: c, Score: v, Justification: "vu"})
			}
		}
		return createWith(t, ctx, cwm, api.EvaluationType, name, api.SolutionEvaluation{Scores: sc},
			cwmapi.ObjectEdge{Kind: api.EvaluatesEdge, To: alt}, cwmapi.ObjectEdge{Kind: api.ScoredOnEdge, To: g})
	}
	upgrade := evaluate("Montée de version", map[string]int{"hosting-fr": 1, "reliability": 4, "tco": 3, "skills": 4})
	saas := evaluate("SaaS", map[string]int{"hosting-fr": 0, "reliability": 5, "tco": 4, "skills": 2})
	partial := evaluate("Développement", map[string]int{"hosting-fr": 1, "reliability": 3})

	// On a draft grid: refused, nothing written.
	if out, err := svc.EvaluationCompute(ctx, api.EvaluationInput{ID: upgrade}); err != nil || out.OK {
		t.Fatalf("computed on a draft grid: %v %+v", err, out)
	}
	if _, err := svc.GridFreeze(ctx, api.GridMoveInput{ID: g, By: architect}); err != nil {
		t.Fatal(err)
	}

	out, err := svc.EvaluationCompute(ctx, api.EvaluationInput{ID: upgrade})
	if err != nil || !out.OK || out.Total != 3.7 || len(out.EliminatedBy) != 0 {
		t.Fatalf("upgrade total: %v %+v, want 3.7 (4x50 + 3x30 + 4x20) / 100", err, out)
	}
	got, _ := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: upgrade})
	var stored api.SolutionEvaluation
	_ = json.Unmarshal(got.Object.FieldsJSON(), &stored)
	if stored.Total != 3.7 || stored.ComputedAt == "" {
		t.Errorf("the total is not written on the evaluation: %+v", stored)
	}
	if out, _ := svc.EvaluationCompute(ctx, api.EvaluationInput{ID: saas}); strings.Join(out.EliminatedBy, ",") != "hosting-fr" {
		t.Errorf("an eliminatory 0 did not eliminate: %+v", out)
	}
	if out, _ := svc.EvaluationCompute(ctx, api.EvaluationInput{ID: partial}); strings.Join(out.Missing, ",") != "tco,skills" || out.Total != 1.5 {
		t.Errorf("unscored criteria not counted 0 and named: %+v", out)
	}

	m, err := svc.EvaluationsMatrix(ctx, api.MatrixInput{Grid: g})
	if err != nil || !m.Frozen || len(m.Rows) != 3 || len(m.Criteria) != 4 {
		t.Fatalf("matrix: %v %+v", err, m)
	}
	order := []string{m.Rows[0].Name, m.Rows[1].Name, m.Rows[2].Name}
	if strings.Join(order, " > ") != "Montée de version > Développement > SaaS" {
		t.Errorf("rank %v, want the best first and the eliminated last", order)
	}
	if m.Rows[0].Scores["reliability"] != 4 || m.Rows[0].Rank != 1 || m.Rows[0].Alternative == "" {
		t.Errorf("row: %+v", m.Rows[0])
	}
}
