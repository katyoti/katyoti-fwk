// SPDX-License-Identifier: Apache-2.0

package api

// EVALUATION. CWM owns the expression vocabulary over an object and the
// functions the capabilities add to it; ObjectEvaluate lends both, so a
// service that keeps expressions of its own (a gate's checks) has them
// evaluated exactly as a guard would be, without a second evaluator.

// ObjectEvaluateInput evaluates expressions over one object, in the
// transition vocabulary: the object's, plus `by`, `from` and `to`.
type ObjectEvaluateInput struct {
	ID          string   `json:"id"             jsonschema:"The object; required"`
	Expressions []string `json:"expressions"    jsonschema:"el expressions over the object; required"`
	By          Actor    `json:"by,omitempty"   jsonschema:"Who would act; empty = nobody"`
	From        string   `json:"from,omitempty" jsonschema:"The state a move would leave"`
	To          string   `json:"to,omitempty"   jsonschema:"The state a move would reach"`
	// NoFunctions evaluates in the plain object vocabulary: no by / from / to
	// and no capability function, so naming one is an error. Where a function
	// may not run (an agent context, a named-file path) the caller says so.
	NoFunctions bool `json:"no_functions,omitempty" jsonschema:"The plain object vocabulary: no by / from / to and no capability function"`
	// NoCostly is a preview's evaluation (a pane, an action list, a list of
	// possible moves): a function declared costly is not run — its call
	// answers ErrDecidedAtMove, an error of the expression, and its answer is
	// marked AtMove. Every other function runs as usual.
	NoCostly bool `json:"no_costly,omitempty" jsonschema:"A preview: a costly function is not run - its call answers decided at the move"`
}

// ObjectEvaluateOutput answers each expression, in order. The expressions
// share one evaluation: a function called by two of them is asked once.
type ObjectEvaluateOutput struct {
	Results []EvaluateResult `json:"results"`
}

// EvaluateResult is one expression's value, or why it has none, with what
// the functions it called answered.
type EvaluateResult struct {
	Expression string           `json:"expression"`
	Value      any              `json:"value,omitempty"`
	Error      string           `json:"error,omitempty"`
	Calls      []FunctionAnswer `json:"calls,omitempty"`
}
