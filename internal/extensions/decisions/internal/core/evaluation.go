// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
)

// object is a CWM object read with its module, revision and lifecycle state.
type object struct {
	obj      cwmapi.Object
	module   string
	revision string
}

func (o object) state(initial string) string {
	if st, _ := o.obj.Capabilities["lifecycle"]["state"].(string); st != "" {
		return st
	}
	return initial
}

func readObject(ctx context.Context, cwm cwmapi.CWMService, id, typ string) (object, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return object{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return object{}, err
	}
	if got.Object.Type != typ {
		return object{}, fmt.Errorf("%w: %q is a %s, not a %s", cwmapi.ErrInvalidInput, id, got.Object.Type, typ)
	}
	return object{obj: got.Object, module: module, revision: got.Revision}, nil
}

func fieldsOf[T any](o object) (T, error) {
	var v T
	if raw := o.obj.FieldsJSON(); len(raw) > 0 {
		if err := json.Unmarshal(raw, &v); err != nil {
			return v, fmt.Errorf("%s fields: %w", o.obj.ID, err)
		}
	}
	return v, nil
}

// actorOf is who moves a grid: the actor given, else the call's cause (an
// action run from the pane carries none).
func actorOf(ctx context.Context, by cwmapi.Actor) (cwmapi.Actor, error) {
	if by.ID != "" {
		return by, nil
	}
	if c, ok := cause.CauseFrom(ctx); ok && c.ID != "" {
		if c.Kind == cause.KindPerson {
			return cwmapi.Actor{ID: c.ID, Kind: cwmapi.ActorPerson}, nil
		}
		return cwmapi.Actor{ID: c.ID, Kind: cwmapi.ActorAgent}, nil
	}
	return cwmapi.Actor{}, fmt.Errorf("%w: by required (who moves the grid)", cwmapi.ErrInvalidInput)
}

// checkGrid names what keeps a grid from being frozen.
func checkGrid(g api.EvaluationGrid) (float64, []string) {
	var problems []string
	seen := map[string]bool{}
	total, scored := 0.0, 0
	for i, c := range g.Criteria {
		if strings.TrimSpace(c.ID) == "" {
			problems = append(problems, fmt.Sprintf("le critère %d n'a pas d'identifiant", i+1))
		} else if seen[c.ID] {
			problems = append(problems, fmt.Sprintf("l'identifiant %q est donné deux fois", c.ID))
		}
		seen[c.ID] = true
		if c.IsEliminatory() {
			continue
		}
		scored++
		if c.Weight <= 0 {
			problems = append(problems, fmt.Sprintf("le critère %q n'a pas de poids", c.Label))
		}
		total += c.Weight
	}
	if scored == 0 {
		problems = append(problems, "la grille n'a aucun critère noté")
	}
	if math.Abs(total-100) > 0.001 {
		problems = append(problems, fmt.Sprintf("les poids font %g %%, pas 100 %%", total))
	}
	return total, problems
}

// GridCheck says whether a grid can be frozen.
func (s *Service) GridCheck(ctx context.Context, in api.GridInput) (api.GridCheckOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.GridCheckOutput{}, err
	}
	o, err := readObject(ctx, cwm, in.ID, api.GridType)
	if err != nil {
		return api.GridCheckOutput{}, err
	}
	g, err := fieldsOf[api.EvaluationGrid](o)
	if err != nil {
		return api.GridCheckOutput{}, err
	}
	total, problems := checkGrid(g)
	out := api.GridCheckOutput{OK: len(problems) == 0, WeightsTotal: total, Problems: problems}
	if out.OK {
		out.Message = fmt.Sprintf("La grille peut être figée : %d critère(s), poids à 100 %%.", len(g.Criteria))
	} else {
		out.Message = "La grille ne peut pas être figée : " + strings.Join(problems, " ; ")
	}
	return out, nil
}

// GridFreeze freezes a grid whose check passes.
func (s *Service) GridFreeze(ctx context.Context, in api.GridMoveInput) (api.GridCheckOutput, error) {
	out, err := s.GridCheck(ctx, api.GridInput{ID: in.ID})
	if err != nil || !out.OK {
		return out, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return out, err
	}
	by, err := actorOf(ctx, in.By)
	if err != nil {
		return out, err
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: in.ID, To: api.GridFrozen, By: by, Note: in.Note}); err != nil {
		return out, err
	}
	out.Message = "Grille figée : les solutions peuvent être évaluées dessus."
	return out, nil
}

// GridReopen sends a frozen grid back to draft, and every evaluation scored
// on it with it: their scores were given on criteria that may now change.
func (s *Service) GridReopen(ctx context.Context, in api.GridMoveInput) (api.GridCheckOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.GridCheckOutput{}, err
	}
	by, err := actorOf(ctx, in.By)
	if err != nil {
		return api.GridCheckOutput{}, err
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: in.ID, To: api.GridDraft, By: by, Note: in.Note}); err != nil {
		return api.GridCheckOutput{}, err
	}
	evals, err := scoredOn(ctx, cwm, in.ID)
	if err != nil {
		return api.GridCheckOutput{}, err
	}
	out := api.GridCheckOutput{OK: true}
	note := "La grille " + in.ID + " a été rouverte"
	if in.Note != "" {
		note += " : " + in.Note
	}
	for _, id := range evals {
		e, err := readObject(ctx, cwm, id, api.EvaluationType)
		if err != nil {
			return out, err
		}
		if e.state(api.EvaluationDraft) == api.EvaluationDraft {
			out.Reopened = append(out.Reopened, id)
			continue
		}
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: api.EvaluationDraft, By: by, Note: note}); err != nil {
			return out, err
		}
		out.Reopened = append(out.Reopened, id)
	}
	out.Message = fmt.Sprintf("Grille rouverte ; %d évaluation(s) à revoir.", len(out.Reopened))
	return out, nil
}

// scoredOn is the evaluations scored on a grid.
func scoredOn(ctx context.Context, cwm cwmapi.CWMService, grid string) ([]string, error) {
	edges, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: grid, Kind: api.ScoredOnEdge, Direction: cwmapi.DirectionIn})
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range edges.Edges {
		out = append(out, e.From)
	}
	sort.Strings(out)
	return out, nil
}

// edgeTarget is the target of an object's only edge of a kind.
func edgeTarget(o cwmapi.Object, kind string) string {
	for _, e := range o.Edges {
		if string(e.Kind) == kind {
			return e.To
		}
	}
	return ""
}

// compute is an evaluation's total on its grid.
func compute(g api.EvaluationGrid, e api.SolutionEvaluation) (total float64, eliminated, missing []string) {
	scores := map[string]int{}
	for _, sc := range e.Scores {
		scores[sc.Criterion] = sc.Score
	}
	weights := 0.0
	for _, c := range g.Criteria {
		v, ok := scores[c.ID]
		if !ok {
			missing = append(missing, c.ID)
		}
		if c.IsEliminatory() {
			if v <= 0 {
				eliminated = append(eliminated, c.ID)
			}
			continue
		}
		weights += c.Weight
		total += c.Weight * float64(v)
	}
	if weights > 0 {
		total = math.Round(total/weights*100) / 100
	}
	return total, eliminated, missing
}

// EvaluationCompute writes an evaluation's total, computed on its frozen grid.
func (s *Service) EvaluationCompute(ctx context.Context, in api.EvaluationInput) (api.EvaluationComputeOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.EvaluationComputeOutput{}, err
	}
	e, err := readObject(ctx, cwm, in.ID, api.EvaluationType)
	if err != nil {
		return api.EvaluationComputeOutput{}, err
	}
	gridID := edgeTarget(e.obj, api.ScoredOnEdge)
	if gridID == "" {
		return api.EvaluationComputeOutput{Message: "L'évaluation n'est rattachée à aucune grille (lien scored-on)."}, nil
	}
	g, err := readObject(ctx, cwm, gridID, api.GridType)
	if err != nil {
		return api.EvaluationComputeOutput{}, err
	}
	if g.state(api.GridDraft) != api.GridFrozen {
		return api.EvaluationComputeOutput{Message: "La grille " + gridID + " n'est pas figée : figez-la avant de calculer."}, nil
	}
	grid, err := fieldsOf[api.EvaluationGrid](g)
	if err != nil {
		return api.EvaluationComputeOutput{}, err
	}
	ev, err := fieldsOf[api.SolutionEvaluation](e)
	if err != nil {
		return api.EvaluationComputeOutput{}, err
	}
	ev.Total, ev.EliminatedBy, ev.Missing = compute(grid, ev)
	ev.ComputedAt = s.now().UTC().Format(time.RFC3339)
	raw, err := json.Marshal(ev)
	if err != nil {
		return api.EvaluationComputeOutput{}, err
	}
	e.obj.SetFieldsJSON(raw)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: e.module, Object: e.obj, Revision: e.revision}); err != nil {
		return api.EvaluationComputeOutput{}, err
	}
	out := api.EvaluationComputeOutput{OK: true, Total: ev.Total, EliminatedBy: ev.EliminatedBy, Missing: ev.Missing}
	out.Message = fmt.Sprintf("Total : %g / %d.", ev.Total, grid.ScaleMax())
	if len(ev.EliminatedBy) > 0 {
		out.Message += " Éliminée par : " + strings.Join(ev.EliminatedBy, ", ") + "."
	}
	if len(ev.Missing) > 0 {
		out.Message += " Non notés (comptés 0) : " + strings.Join(ev.Missing, ", ") + "."
	}
	return out, nil
}

// EvaluationsMatrix is every alternative scored on a grid, ranked.
func (s *Service) EvaluationsMatrix(ctx context.Context, in api.MatrixInput) (api.MatrixOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.MatrixOutput{}, err
	}
	g, err := readObject(ctx, cwm, in.Grid, api.GridType)
	if err != nil {
		return api.MatrixOutput{}, err
	}
	grid, err := fieldsOf[api.EvaluationGrid](g)
	if err != nil {
		return api.MatrixOutput{}, err
	}
	out := api.MatrixOutput{Grid: in.Grid, Frozen: g.state(api.GridDraft) == api.GridFrozen, Criteria: grid.Criteria, Rows: []api.MatrixRow{}}
	ids, err := scoredOn(ctx, cwm, in.Grid)
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		e, err := readObject(ctx, cwm, id, api.EvaluationType)
		if err != nil {
			return out, err
		}
		ev, err := fieldsOf[api.SolutionEvaluation](e)
		if err != nil {
			return out, err
		}
		total, eliminated, _ := compute(grid, ev)
		row := api.MatrixRow{Alternative: edgeTarget(e.obj, api.EvaluatesEdge), Evaluation: id, Name: e.obj.Name,
			State: e.state(api.EvaluationDraft), Scores: map[string]int{}, Total: total, EliminatedBy: eliminated}
		for _, sc := range ev.Scores {
			row.Scores[sc.Criterion] = sc.Score
		}
		if row.Alternative != "" {
			if alt, err := readAny(ctx, cwm, row.Alternative); err == nil && alt.Name != "" {
				row.Name = alt.Name
			}
		}
		out.Rows = append(out.Rows, row)
	}
	sort.SliceStable(out.Rows, func(i, j int) bool {
		ei, ej := len(out.Rows[i].EliminatedBy) > 0, len(out.Rows[j].EliminatedBy) > 0
		if ei != ej {
			return !ei
		}
		return out.Rows[i].Total > out.Rows[j].Total
	})
	for i := range out.Rows {
		out.Rows[i].Rank = i + 1
	}
	return out, nil
}

func readAny(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return cwmapi.Object{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	return got.Object, err
}
