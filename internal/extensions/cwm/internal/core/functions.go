// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/el"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// EXPRESSION FUNCTIONS (b6). A capability adds read-only questions its service
// answers about an object — `gate('ready')` — to the expression language. CWM
// keeps the registry and calls them; it never knows what they mean.
//
//   - A function runs its action, a READ-ONLY method, through the ActionService
//     with {function, object, args}; the answer is its value. A function
//     declared costly may name an action that writes (verify() records
//     verdicts): it is never run by a preview.
//   - A preview (ObjectEvaluateInput.NoCostly, a dry run with NoCostly) does
//     not run a costly function: its call answers ErrDecidedAtMove, an error
//     of the expression, marked AtMove — decided at the move. The other
//     functions run, told NoCostly so a function evaluating expressions of its
//     own (gate()) previews them too.
//   - It is called only when an expression calls it, at most once per
//     evaluation for the same arguments, within its timeout.
//   - Guards, gate checks, action `when:` / inputs, panel filters and views
//     may call one. A hook's `when:` / input, a named-file path, an agent-context
//     section and a task `filter:` may not: their vocabularies have no
//     functions, so naming one is a compile error.
//   - A call that fails is an ERROR of the expression — a guard refuses with
//     it, a `when:` disables with it, a view leaves the value empty — and it is
//     logged once per function.

// defaultFunctionTimeout is how long a call may take when the declaration
// says nothing.
const defaultFunctionTimeout = 5 * time.Second

var functionName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// exprFunction is one registered function.
type exprFunction struct {
	decl       api.CapabilityFunction
	capability string
	timeout    time.Duration
}

// functionRegistry is the universe's expression functions and the vocabularies
// compiled with them.
type functionRegistry struct {
	byName          map[string]exprFunction
	names           []string
	objectExprs     *el.Cache
	transitionExprs *el.Cache
	readOnly        sync.Map // action URI → true once it answered read-only
	logged          sync.Map // function name → true once a failure was logged
}

// buildFunctions registers the functions the capabilities declare. A name that
// is not an identifier, shadows a vocabulary name or is declared by two
// capabilities is left out and reported — the catalog itself still loads.
func buildFunctions(caps []api.Capability) (*functionRegistry, []string) {
	reserved := el.TransitionVocabulary()
	byName := map[string][]exprFunction{}
	var problems []string
	for _, c := range caps {
		for _, f := range c.Functions {
			switch {
			case !functionName.MatchString(f.Name):
				problems = append(problems, fmt.Sprintf("capability %q: function %q is not a lowercase identifier", c.Name, f.Name))
				continue
			case reserved[f.Name] != nil:
				problems = append(problems, fmt.Sprintf("capability %q: function %q would hide the vocabulary name %q", c.Name, f.Name, f.Name))
				continue
			case f.Action == "":
				problems = append(problems, fmt.Sprintf("capability %q: function %q names no action", c.Name, f.Name))
				continue
			}
			timeout := defaultFunctionTimeout
			if f.Timeout != "" {
				d, err := time.ParseDuration(f.Timeout)
				if err != nil || d <= 0 {
					problems = append(problems, fmt.Sprintf("capability %q: function %q: timeout %q is not a duration; 5s is used", c.Name, f.Name, f.Timeout))
				} else {
					timeout = d
				}
			}
			byName[f.Name] = append(byName[f.Name], exprFunction{decl: f, capability: c.Name, timeout: timeout})
		}
	}
	r := &functionRegistry{byName: map[string]exprFunction{}}
	for name, decls := range byName {
		if len(decls) > 1 {
			by := make([]string, 0, len(decls))
			for _, d := range decls {
				by = append(by, d.capability)
			}
			sort.Strings(by)
			problems = append(problems, fmt.Sprintf("function %q is declared by %s — left out: one name, one meaning", name, strings.Join(by, " and ")))
			continue
		}
		r.byName[name] = decls[0]
		r.names = append(r.names, name)
	}
	sort.Strings(r.names)
	sort.Strings(problems)
	for _, p := range problems {
		slog.Error("cwm: expression function refused", "problem", p)
	}
	r.objectExprs = el.NewEnvCache(el.WithFunctions(el.ObjectVocabulary(), r.names))
	r.transitionExprs = el.NewEnvCache(el.WithFunctions(el.TransitionVocabulary(), r.names))
	return r, problems
}

// objectCache compiles object expressions that may call functions.
func (s *Service) objectCache() *el.Cache {
	fns := s.fns.Load()
	if fns == nil {
		return objectExprs
	}
	return fns.objectExprs
}

// transitionCache compiles guards and gate checks that may call functions.
func (s *Service) transitionCache() *el.Cache {
	fns := s.fns.Load()
	if fns == nil {
		return transitionExprs
	}
	return fns.transitionExprs
}

// objectEnv is objectActionEnv with the functions bound for this object: what
// every expression compiled by objectCache / transitionCache is evaluated in.
// A preview's env (noCostly) does not run a costly function.
func (s *Service) objectEnv(ctx stdctx.Context, obj api.Object, noCostly bool) map[string]any {
	return s.bindFunctions(ctx, obj.ID, s.objectActionEnv(obj), noCostly)
}

// bindFunctions puts each function into env as a call over (ctx, object). A
// call runs only when the expression makes it, and once per env for the same
// arguments. With noCostly, a costly function is not run: it answers
// ErrDecidedAtMove.
func (s *Service) bindFunctions(ctx stdctx.Context, objectID string, env map[string]any, noCostly bool) map[string]any {
	fns := s.fns.Load()
	if fns == nil || len(fns.names) == 0 {
		return env
	}
	type answer struct {
		v   any
		err error
	}
	var mu sync.Mutex
	memo := map[string]answer{}
	log := &callLog{}
	env[callLogKey] = log
	for _, name := range fns.names {
		f := fns.byName[name]
		env[name] = func(args ...any) (any, error) {
			key := name
			if b, err := json.Marshal(args); err == nil {
				key += string(b)
			}
			mu.Lock()
			if a, ok := memo[key]; ok {
				mu.Unlock()
				return a.v, a.err
			}
			mu.Unlock()
			var v any
			var err error
			if noCostly && f.decl.Costly {
				err = fmt.Errorf("%s(): %w", name, api.ErrDecidedAtMove)
			} else {
				v, err = s.callFunction(ctx, f, objectID, args, env, noCostly)
			}
			if err != nil && !api.IsDecidedAtMove(err) {
				if _, seen := fns.logged.LoadOrStore(f.decl.Name, true); !seen {
					slog.Warn("cwm: expression function failed", "function", f.decl.Name, "object", objectID, "error", err)
				}
			}
			mu.Lock()
			memo[key] = answer{v, err}
			mu.Unlock()
			log.add(name, args, v, err)
			return v, err
		}
	}
	return env
}

// callLogKey holds, in an environment, what its functions answered — for a
// refusal to say why. Not an identifier an expression can name.
const callLogKey = "#calls"

// callLog records each call once, in order.
type callLog struct {
	mu      sync.Mutex
	answers []api.FunctionAnswer
}

func (l *callLog) add(name string, args []any, v any, err error) {
	a := api.FunctionAnswer{Function: name, Args: args, Answer: v}
	if err != nil {
		a.Answer, a.Error, a.AtMove = nil, err.Error(), api.IsDecidedAtMove(err)
	}
	l.mu.Lock()
	l.answers = append(l.answers, a)
	l.mu.Unlock()
}

// callsOf is what the functions an evaluation called answered, in order.
func callsOf(env map[string]any) []api.FunctionAnswer {
	l, ok := env[callLogKey].(*callLog)
	if !ok {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]api.FunctionAnswer(nil), l.answers...)
}

// A refusal's reason is what a person reads (a block note in « À faire »):
// one line, short. The full answers stay on the refusal's Calls.
const (
	reasonCallsMax  = 240 // runes of the whole " (...)" part
	reasonAnswerMax = 120 // runes of one raw answer
	reasonLabelsMax = 3   // unmet items named for a checklist answer
	reasonItemsMax  = 200 // runes the unmet items share
)

// calledFunctions says, in one short line, what the functions an evaluation
// called answered — " (score(): 3; gate("ready"): 42 unmet - a; b; c; …)" —
// or "" when it called none. A checklist answer (items with a status) says
// its unmet count and first labels; any other answer is its compact JSON,
// unescaped and cut.
func calledFunctions(env map[string]any) string {
	calls := callsOf(env)
	if len(calls) == 0 {
		return ""
	}
	lines := make([]string, 0, len(calls))
	for _, c := range calls {
		call := c.Function + "(" + strings.TrimSuffix(strings.TrimPrefix(plainJSON(c.Args), "["), "]") + ")"
		if c.Function != "" && (c.Args == nil || plainJSON(c.Args) == "null") {
			call = c.Function + "()"
		}
		if c.Error != "" {
			lines = append(lines, call+": "+cut(c.Error, reasonAnswerMax))
			continue
		}
		lines = append(lines, call+": "+answerLine(c.Answer))
	}
	return " (" + cut(strings.Join(lines, "; "), reasonCallsMax-3) + ")"
}

// answerLine is one answer in a reason: a checklist's unmet count and first
// labels, else its compact JSON, cut.
func answerLine(v any) string {
	m, ok := v.(map[string]any)
	if items, isList := m["items"].([]any); ok && isList {
		var unmet []string
		for _, it := range items {
			item, _ := it.(map[string]any)
			if s, _ := item["status"].(string); item == nil || s == "met" {
				continue
			}
			// The name stays: readers match on it (gui-tested, module-updated).
			name, _ := item["name"].(string)
			label, _ := item["reason"].(string) // why it is unmet says most
			if label == "" {
				label, _ = item["label"].(string)
			}
			switch {
			case label == "" || label == name:
				label = name
			case name != "":
				label = name + ": " + label
			}
			unmet = append(unmet, label)
		}
		if len(unmet) == 0 {
			return "all met"
		}
		named := append([]string(nil), unmet...)
		if len(named) > reasonLabelsMax {
			named = append(named[:reasonLabelsMax], "…")
		}
		share := reasonItemsMax / min(len(unmet), reasonLabelsMax) // one item may say all its causes
		for i := range named {
			named[i] = cut(named[i], share)
		}
		return fmt.Sprintf("%d unmet - %s", len(unmet), strings.Join(named, "; "))
	}
	return cut(plainJSON(v), reasonAnswerMax)
}

// plainJSON is v's compact JSON, without HTML escaping.
func plainJSON(v any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// cut is s on one line, at most max runes (an ellipsis marks a cut).
func cut(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

// callFunction runs one call: the action must be read-only (a costly
// function's may write); the answer must come within the timeout and be of
// the declared kind.
func (s *Service) callFunction(ctx stdctx.Context, f exprFunction, objectID string, args []any, env map[string]any, noCostly bool) (any, error) {
	name := f.decl.Name
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil || actions == nil {
		return nil, fmt.Errorf("%s(): there is no ActionService to ask", name)
	}
	if !f.decl.Costly {
		if err := s.fns.Load().checkReadOnly(ctx, actions, f.decl.Action); err != nil {
			return nil, fmt.Errorf("%s(): %w", name, err)
		}
	}
	if args == nil {
		args = []any{}
	}
	call := api.FunctionCall{Function: name, Object: objectID, Args: args, NoCostly: noCostly}
	if by, ok := env["by"].(map[string]any); ok {
		id, _ := by["id"].(string)
		kind, _ := by["kind"].(string)
		if id != "" || kind != "" {
			call.By = &api.Actor{ID: id, Kind: kind}
		}
	}
	call.From, _ = env["from"].(string)
	call.To, _ = env["to"].(string)
	in, err := json.Marshal(call)
	if err != nil {
		return nil, fmt.Errorf("%s(): %w", name, err)
	}
	cctx, cancel := stdctx.WithTimeout(ctx, f.timeout)
	defer cancel()
	type result struct {
		out actionsapi.RunOutput
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := actions.Run(cctx, actionsapi.RunInput{URI: f.decl.Action, Input: in})
		done <- result{out, err}
	}()
	var r result
	select {
	case r = <-done:
	case <-cctx.Done():
		return nil, fmt.Errorf("%s(): no answer within %s", name, f.timeout)
	}
	if r.err != nil {
		return nil, fmt.Errorf("%s(): %w", name, r.err)
	}
	var v any
	if len(r.out.Value) > 0 {
		if err := json.Unmarshal(r.out.Value, &v); err != nil {
			return nil, fmt.Errorf("%s(): the answer is not JSON: %w", name, err)
		}
	}
	if err := checkReturns(f.decl.Returns, v); err != nil {
		return nil, fmt.Errorf("%s(): %w", name, err)
	}
	return v, nil
}

// checkReadOnly asks whether an action changes nothing. Only a read-only
// answer is remembered: a refusal or a failure to ask is asked again at the
// next call, since a catalog not yet current (its sidecar still connecting or
// re-indexed) answers « not read-only » for a method it does not know yet
func (r *functionRegistry) checkReadOnly(ctx stdctx.Context, actions actionsapi.ActionService, uri string) error {
	if _, ok := r.readOnly.Load(uri); ok {
		return nil
	}
	d, err := actions.Describe(ctx, actionsapi.DescribeInput{URI: uri})
	if err != nil {
		return fmt.Errorf("cannot describe %s: %w", uri, err)
	}
	if !d.ReadOnly {
		return fmt.Errorf("%s is not a read-only method — an expression may only ask, never change", uri)
	}
	r.readOnly.Store(uri, true)
	return nil
}

// checkReturns holds an answer to the declared kind; empty = any. A null
// answer is an answer of every kind: the service had nothing to say.
func checkReturns(kind string, v any) error {
	if kind == "" || v == nil {
		return nil
	}
	ok := false
	switch kind {
	case api.FunctionReturnsBoolean:
		_, ok = v.(bool)
	case api.FunctionReturnsNumber:
		_, ok = v.(float64)
	case api.FunctionReturnsString:
		_, ok = v.(string)
	case api.FunctionReturnsObject:
		_, ok = v.(map[string]any)
	case api.FunctionReturnsList:
		_, ok = v.([]any)
	default:
		return nil
	}
	if !ok {
		return fmt.Errorf("answered %T where it declares %s", v, kind)
	}
	return nil
}
