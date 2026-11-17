// SPDX-License-Identifier: Apache-2.0

package report

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// QueryFunc runs one named query of a source with its variables and answers
// flat rows (column id → string value) and the columns that describe them.
// It is the one seam between the runner and a source: a connector adapts its
// own query engine and field specs to it.
type QueryFunc func(ctx context.Context, query string, vars map[string]string) ([]map[string]string, []Column, error)

// Transformer turns a step result into another, driven by the step's params.
type Transformer func(input *StepResult, params map[string]any) (*StepResult, error)

// Transformers is a registry of transformers by name.
type Transformers map[string]Transformer

// BuiltinTransformers answers a registry holding the built-in transformers:
// filter-rows, derive-column and group-by-relation. A connector registers its
// own on the value it gets.
func BuiltinTransformers() Transformers {
	return Transformers{
		"filter-rows":       filterRows,
		"derive-column":     deriveColumn,
		"group-by-relation": groupByRelation,
	}
}

// Register adds (or replaces) a transformer.
func (t Transformers) Register(name string, fn Transformer) {
	t[name] = fn
}

// Get returns a transformer by name.
func (t Transformers) Get(name string) (Transformer, error) {
	fn, ok := t[name]
	if !ok {
		return nil, fmt.Errorf("unknown transformer: %q", name)
	}
	return fn, nil
}

// Names lists the registered transformer names, sorted.
func (t Transformers) Names() []string {
	names := make([]string, 0, len(t))
	for name := range t {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// StepResult holds the output of a report pipeline step.
type StepResult struct {
	// Rows contains the flat data rows (column id → string value).
	Rows []map[string]string

	// Columns describes the columns of the rows, in order.
	Columns []Column

	// SubOutputs holds named sub-results — for transformers that split their
	// output into several named buckets (e.g. "valid" / "errors").
	SubOutputs map[string]*StepResult
}

// Runner executes report pipelines over a source's query function.
type Runner struct {
	query        QueryFunc
	transformers Transformers
}

// NewRunner creates a report runner with the given query function and
// transformer registry (nil: the built-in transformers).
func NewRunner(query QueryFunc, transformers Transformers) *Runner {
	if transformers == nil {
		transformers = BuiltinTransformers()
	}
	return &Runner{query: query, transformers: transformers}
}

// Run executes a report definition and returns the named step results.
func (r *Runner) Run(ctx context.Context, def *ReportDef, vars map[string]string) (map[string]*StepResult, error) {
	merged, err := ResolveVariables(def, vars)
	if err != nil {
		return nil, err
	}

	results := make(map[string]*StepResult)

	for _, step := range def.Steps {
		switch step.Type {
		case "query":
			result, err := r.runQueryStep(ctx, step, merged)
			if err != nil {
				return nil, fmt.Errorf("step %q: %w", step.Name, err)
			}
			results[step.Name] = result

		case "transform":
			input, err := resolveStepInput(step.Input, results)
			if err != nil {
				return nil, fmt.Errorf("step %q: %w", step.Name, err)
			}
			tfn, err := r.transformers.Get(step.Transformer)
			if err != nil {
				return nil, fmt.Errorf("step %q: %w", step.Name, err)
			}
			result, err := tfn(input, step.Params)
			if err != nil {
				return nil, fmt.Errorf("step %q: %w", step.Name, err)
			}
			results[step.Name] = result

		default:
			return nil, fmt.Errorf("step %q: unknown type %q", step.Name, step.Type)
		}
	}

	return results, nil
}

// runQueryStep runs a query step through the query function.
func (r *Runner) runQueryStep(ctx context.Context, step StepDef, vars map[string]string) (*StepResult, error) {
	if r.query == nil {
		return nil, fmt.Errorf("query %q: no query function", step.Query)
	}
	stepVars := make(map[string]string)
	for k, v := range step.Vars {
		stepVars[k] = resolveVarString(v, vars)
	}

	rows, cols, err := r.query(ctx, step.Query, stepVars)
	if err != nil {
		return nil, fmt.Errorf("query %q: %w", step.Query, err)
	}
	return &StepResult{Rows: rows, Columns: cols}, nil
}

// resolveStepInput finds a step result by name, supporting dot notation for sub-outputs.
func resolveStepInput(ref string, results map[string]*StepResult) (*StepResult, error) {
	parts := strings.SplitN(ref, ".", 2)
	stepName := parts[0]

	result, ok := results[stepName]
	if !ok {
		return nil, fmt.Errorf("input %q not found (step %q has not run or does not exist)", ref, stepName)
	}

	if len(parts) == 1 {
		return result, nil
	}

	subName := parts[1]
	if result.SubOutputs == nil {
		return nil, fmt.Errorf("input %q: step %q has no sub-outputs", ref, stepName)
	}
	sub, ok := result.SubOutputs[subName]
	if !ok {
		return nil, fmt.Errorf("input %q: sub-output %q not found in step %q", ref, subName, stepName)
	}
	return sub, nil
}

// resolveVarString replaces ${var} placeholders in a string.
func resolveVarString(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	return s
}

// ResolveOutputData resolves a sheet data reference from step results.
// Supports dot notation: "step.subOutput" → named sub-output of a step.
func ResolveOutputData(ref string, results map[string]*StepResult) (*StepResult, error) {
	return resolveStepInput(ref, results)
}

// HeaderColumns answers the columns to write for a step result: its columns,
// each with its label (the id when it has none); with no columns, the keys of
// its first row, sorted.
func HeaderColumns(sr *StepResult) []Column {
	if len(sr.Columns) > 0 {
		cols := make([]Column, len(sr.Columns))
		for i, c := range sr.Columns {
			cols[i] = c
			if cols[i].Label == "" {
				cols[i].Label = c.ID
			}
		}
		return cols
	}
	if len(sr.Rows) == 0 {
		return nil
	}
	keys := make([]string, 0, len(sr.Rows[0]))
	for k := range sr.Rows[0] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	cols := make([]Column, len(keys))
	for i, k := range keys {
		cols[i] = Column{ID: k, Label: k}
	}
	return cols
}
