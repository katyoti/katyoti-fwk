// SPDX-License-Identifier: Apache-2.0

package api

// EVALUATION (modules/decisions/): a choice between alternatives is argued on
// an `evaluation-grid` - weighted criteria, frozen before anything is scored
// so they cannot be tuned to favour an alternative - and each alternative is
// a `solution-evaluation` scored on it, criterion by criterion with a
// justification and its evidence. The total is computed (EvaluationCompute),
// never typed; EvaluationsMatrix is the comparison a decision is prepared
// from.

import cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"

// The types, workflows and edges the ktw-methods pack declares for it.
const (
	GridType               = "evaluation-grid"
	EvaluationType         = "solution-evaluation"
	GridWorkflow           = "evaluation-grid"
	EvaluationWorkflow     = "solution-evaluation"
	EvaluatesEdge          = "evaluates"
	ScoredOnEdge           = "scored-on"
	GridDraft              = "draft"
	GridFrozen             = "frozen"
	EvaluationDraft        = "draft"
	EvaluationReviewed     = "reviewed"
	CriterionScored        = "scored"
	CriterionEliminatory   = "eliminatory"
	defaultScaleMax        = 5
	weightsTotalPercentage = 100
)

// EvaluationGrid are the fields of an evaluation grid.
type EvaluationGrid struct {
	Criteria []Criterion `json:"criteria"        jsonschema:"title=Criteria,description=What an alternative is judged on"`
	Scale    *Scale      `json:"scale,omitempty" jsonschema:"title=Scale,description=How a criterion is scored - 0 to 5 when empty"`
}

// Criterion is one row of a grid.
type Criterion struct {
	ID       string  `json:"id"                 jsonschema:"title=Id,description=A handle unique in the grid in kebab-case"`
	Label    string  `json:"label"              jsonschema:"title=Criterion,description=What is judged"`
	Category string  `json:"category,omitempty" jsonschema:"title=Category,description=The side it judges,enum=functional|technical|security|cost|organisational|strategic|compliance"`
	Kind     string  `json:"kind,omitempty"     jsonschema:"title=Kind,description=scored counts with its weight - eliminatory rules an alternative out when scored 0,enum=scored|eliminatory"`
	Weight   float64 `json:"weight,omitempty"   jsonschema:"title=Weight,description=Its share of the total in percent - scored criteria only"`
	Source   string  `json:"source,omitempty"   jsonschema:"title=Source,description=Where it comes from: a need or a standard"`
	Note     string  `json:"note,omitempty"     jsonschema:"title=Note,description=How to score it"`
}

// Scale says how a criterion is scored.
type Scale struct {
	Max    int          `json:"max"              jsonschema:"title=Maximum,description=The best score - the lowest is 0"`
	Levels []ScaleLevel `json:"levels,omitempty" jsonschema:"title=Levels,description=What each score means"`
}

// ScaleLevel is what one score means.
type ScaleLevel struct {
	Score   int    `json:"score"   jsonschema:"title=Score"`
	Meaning string `json:"meaning" jsonschema:"title=Meaning"`
}

// ScaleMax is the grid's best score.
func (g EvaluationGrid) ScaleMax() int {
	if g.Scale != nil && g.Scale.Max > 0 {
		return g.Scale.Max
	}
	return defaultScaleMax
}

// IsEliminatory reports a criterion that rules an alternative out.
func (c Criterion) IsEliminatory() bool { return c.Kind == CriterionEliminatory }

// SolutionEvaluation are the fields of an evaluation.
type SolutionEvaluation struct {
	Scores       []Score  `json:"scores"                  jsonschema:"title=Scores,description=One per criterion of the grid"`
	Total        float64  `json:"total,omitempty"         jsonschema:"title=Total,description=The weighted total on the grid's scale (computed)"`
	EliminatedBy []string `json:"eliminated_by,omitempty" jsonschema:"title=Eliminated by,description=The eliminatory criteria it fails (computed)"`
	Missing      []string `json:"missing,omitempty"       jsonschema:"title=Missing,description=The criteria not scored - counted 0 (computed)"`
	ComputedAt   string   `json:"computed_at,omitempty"   jsonschema:"title=Computed at,description=When the total was computed"`
}

// Score is one criterion's score.
type Score struct {
	Criterion     string `json:"criterion"               jsonschema:"title=Criterion,description=The criterion's id in the grid"`
	Score         int    `json:"score"                   jsonschema:"title=Score"`
	Justification string `json:"justification,omitempty" jsonschema:"title=Justification,description=Why this score"`
	Evidence      string `json:"evidence,omitempty"      jsonschema:"title=Evidence,description=What shows it: a source or a document"`
}

// GridInput names a grid.
type GridInput struct {
	ID string `json:"id" cli:"id" jsonschema:"The evaluation grid's id; required"`
}

// GridMoveInput names a grid and who moves it.
type GridMoveInput struct {
	ID   string       `json:"id"             cli:"id"   jsonschema:"The evaluation grid's id; required"`
	By   cwmapi.Actor `json:"by"                  jsonschema:"Who moves it; empty = the call's cause"`
	Note string       `json:"note,omitempty" cli:"note" jsonschema:"Why - recorded on the move"`
}

// GridCheckOutput says whether a grid can be frozen.
type GridCheckOutput struct {
	OK           bool     `json:"ok"`
	Message      string   `json:"message"`
	WeightsTotal float64  `json:"weights_total"`
	Problems     []string `json:"problems,omitempty"`
	Reopened     []string `json:"reopened,omitempty" jsonschema:"On a reopen: the evaluations sent back to draft"`
}

// EvaluationInput names an evaluation.
type EvaluationInput struct {
	ID string `json:"id" cli:"id" jsonschema:"The solution evaluation's id; required"`
}

// EvaluationComputeOutput is the computed total.
type EvaluationComputeOutput struct {
	OK           bool     `json:"ok"`
	Message      string   `json:"message"`
	Total        float64  `json:"total"`
	EliminatedBy []string `json:"eliminated_by,omitempty"`
	Missing      []string `json:"missing,omitempty"`
}

// MatrixInput names the grid whose evaluations are compared.
type MatrixInput struct {
	Grid string `json:"grid" cli:"grid" jsonschema:"The evaluation grid's id; required"`
}

// MatrixOutput is the comparison: every alternative scored on the grid,
// ranked - the ones eliminated last.
type MatrixOutput struct {
	Grid     string      `json:"grid"`
	Frozen   bool        `json:"frozen"`
	Criteria []Criterion `json:"criteria"`
	Rows     []MatrixRow `json:"rows"`
}

// MatrixRow is one alternative.
type MatrixRow struct {
	Alternative  string         `json:"alternative"             jsonschema:"The object the evaluation evaluates"`
	Name         string         `json:"name"`
	Evaluation   string         `json:"evaluation"`
	State        string         `json:"state"`
	Scores       map[string]int `json:"scores"`
	Total        float64        `json:"total"`
	EliminatedBy []string       `json:"eliminated_by,omitempty"`
	Rank         int            `json:"rank"`
}
