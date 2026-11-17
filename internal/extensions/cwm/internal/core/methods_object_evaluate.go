// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ObjectEvaluate evaluates expressions over an object in the transition
// vocabulary, the capabilities' functions bound — what a guard sees. The
// expressions share one evaluation, so a function two of them call is asked
// once; each result carries the answers of the functions it names.
func (s *Service) ObjectEvaluate(ctx stdctx.Context, in api.ObjectEvaluateInput) (api.ObjectEvaluateOutput, error) {
	if in.ID == "" || len(in.Expressions) == 0 {
		return api.ObjectEvaluateOutput{}, fmt.Errorf("%w: id and expressions required", api.ErrInvalidInput)
	}
	obj, err := s.gateSubject(in.ID)
	if err != nil {
		return api.ObjectEvaluateOutput{}, err
	}
	env, cache := s.gateEnv(ctx, obj, in.By, in.From, in.To, in.NoCostly), s.transitionCache()
	if in.NoFunctions {
		env, cache = s.objectActionEnv(obj), objectExprs
	}
	out := api.ObjectEvaluateOutput{Results: make([]api.EvaluateResult, 0, len(in.Expressions))}
	for _, expr := range in.Expressions {
		r := api.EvaluateResult{Expression: expr}
		v, err := cache.Eval(expr, env)
		if err != nil {
			r.Error = err.Error()
		} else {
			r.Value = v
		}
		r.Calls = callsNamed(env, expr)
		out.Results = append(out.Results, r)
	}
	return out, nil
}

// callsNamed is the answers, among those of env, of the functions expr names —
// asked by it or, memoised, by an expression before it.
func callsNamed(env map[string]any, expr string) []api.FunctionAnswer {
	names, err := el.Names(expr)
	if err != nil {
		return nil
	}
	var out []api.FunctionAnswer
	for _, c := range callsOf(env) {
		if names[c.Function] {
			out = append(out, c)
		}
	}
	return out
}
