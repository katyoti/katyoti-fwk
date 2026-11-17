// SPDX-License-Identifier: Apache-2.0

// Package core checks named gates. The definitions come from the catalog
// (`gate` / `gate-requirement` contributions, any pack); a requirement is
// checked on its own — an expression by CWM (ObjectEvaluate, so it reads
// what a guard reads, functions included), an action by the ActionService, a
// person requirement by the confirmation kept in the object's `gates` group,
// which this service alone writes.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/el"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// Service implements api.GatesService over the CWMService and the catalog in ctx.
type Service struct {
	mu   sync.Mutex
	defs *definitions // nil until read; Invalidate drops it
}

// New returns the service.
func New() *Service { return &Service{} }

// definitions are the gates and the contributed requirements.
type definitions struct {
	gates        []api.Gate
	requirements []api.GateRequirement
}

// Invalidate drops the definitions, read again on the next check (the
// catalog changed).
func (s *Service) Invalidate() {
	s.mu.Lock()
	s.defs = nil
	s.mu.Unlock()
}

func (s *Service) definitions(ctx context.Context) definitions {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.defs == nil {
		d := loadDefinitions(ctx)
		s.defs = &d
	}
	return *s.defs
}

// loadDefinitions reads every gate and gate requirement in the catalog,
// through the one typed reader (QueryTyped), each decoded on its own.
func loadDefinitions(ctx context.Context) definitions {
	var d definitions
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil || cat == nil {
		slog.Warn("gates: no catalog: no gate is declared", "error", err)
		return d
	}
	// Each declaration decodes on its own: one that does not (a key no field
	// takes, a content field in a requirement's `applies`) is skipped with the
	// reason, never all of them.
	gates, err := mbapi.QueryTyped[json.RawMessage](ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: api.GateKind, Catalog: cat})
	if err != nil {
		slog.Warn("gates: gate declarations not read: no gate is declared", "error", err)
	}
	for _, g := range gates {
		var x api.Gate
		if len(g.Body) > 0 {
			if err := json.Unmarshal(g.Body, &x); err != nil {
				slog.Warn("gates: gate skipped", "uri", g.URI, "error", err)
				continue
			}
		}
		if x.Name != "" {
			x.Provider = g.Provider
			d.gates = append(d.gates, x)
		}
	}
	reqs, err := mbapi.QueryTyped[json.RawMessage](ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: api.GateRequirementKind, Catalog: cat})
	if err != nil {
		slog.Warn("gates: gate-requirement declarations not read", "error", err)
	}
	for _, r := range reqs {
		var x api.GateRequirement
		if len(r.Body) > 0 {
			if err := json.Unmarshal(r.Body, &x); err != nil {
				slog.Warn("gates: gate-requirement skipped", "uri", r.URI, "error", err)
				continue
			}
		}
		if x.Name != "" && x.Gate != "" {
			x.Provider = r.Provider
			d.requirements = append(d.requirements, x)
		}
	}
	return d
}

// gateFor is the gate `name` as it applies to the subject (an object's type
// and effective capabilities): its own requirements and the contributed ones,
// each kept when its `applies` admits the subject.
func (d definitions) gateFor(name string, subj cwmapi.FilterSubject) (api.Gate, bool) {
	var g api.Gate
	found := false
	for _, x := range d.gates {
		if x.Name == name {
			g, found = x, true
			break
		}
	}
	if !found {
		return api.Gate{}, false
	}
	var reqs []api.GateRequirement
	for _, r := range g.Requirements {
		if ok, _ := r.AppliesTo(subj); ok {
			reqs = append(reqs, r)
		}
	}
	for _, r := range d.requirements {
		if ok, _ := r.AppliesTo(subj); ok && r.Gate == name {
			reqs = append(reqs, r)
		}
	}
	g.Requirements = reqs
	return g, true
}

func cwmOf(ctx context.Context) (cwmapi.CWMService, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil || cwm == nil {
		return nil, fmt.Errorf("gates: no CWMService: %v", err)
	}
	return cwm, nil
}

func load(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, error) {
	module, _ := cwmapi.ModuleOfObjectID(id)
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return cwmapi.Object{}, err
	}
	return got.Object, nil
}

// subjectOf is what a requirement's `applies` is asked about: the object's
// type and its effective capabilities (type, aspects, groups, attached).
func subjectOf(ctx context.Context, cwm cwmapi.CWMService, obj cwmapi.Object) (cwmapi.FilterSubject, error) {
	eff, err := cwm.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: obj.ID})
	if err != nil {
		return cwmapi.FilterSubject{}, err
	}
	s := cwmapi.FilterSubject{Type: obj.Type}
	for _, c := range eff.Capabilities {
		s.Capabilities = append(s.Capabilities, c.ID)
	}
	return s, nil
}

// gateOn is the gate `name` as it applies to obj.
func (s *Service) gateOn(ctx context.Context, cwm cwmapi.CWMService, obj cwmapi.Object, name string) (api.Gate, error) {
	subj, err := subjectOf(ctx, cwm, obj)
	if err != nil {
		return api.Gate{}, err
	}
	g, ok := s.definitions(ctx).gateFor(name, subj)
	if !ok {
		return api.Gate{}, fmt.Errorf("%w: gate %q is not declared", cwmapi.ErrNotFound, name)
	}
	return g, nil
}

// Check answers an object's checklist for a gate.
func (s *Service) Check(ctx context.Context, in api.CheckInput) (api.CheckOutput, error) {
	if in.ID == "" || in.Gate == "" {
		return api.CheckOutput{}, fmt.Errorf("%w: id and gate required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.CheckOutput{}, err
	}
	obj, err := load(ctx, cwm, in.ID)
	if err != nil {
		return api.CheckOutput{}, err
	}
	g, err := s.gateOn(ctx, cwm, obj, in.Gate)
	if err != nil {
		return api.CheckOutput{}, err
	}
	return api.CheckOutput{Checklist: checklist(ctx, cwm, obj, g, in)}, nil
}

// checklist checks every requirement of g on obj. The expressions go to CWM
// in one evaluation, so a function two of them call is asked once.
func checklist(ctx context.Context, cwm cwmapi.CWMService, obj cwmapi.Object, g api.Gate, in api.CheckInput) api.GateChecklist {
	out := api.GateChecklist{Gate: g.Name, Label: g.Label, Passes: true, Items: make([]api.GateItem, len(g.Requirements))}
	confirmed := confirmations(obj)
	var exprs []string
	var exprAt []int
	for i, r := range g.Requirements {
		it := api.GateItem{Name: r.Name, Label: r.Label, Resolve: r.Resolve}
		switch {
		case r.Validate() != nil:
			it.Kind, it.Status, it.Reason = "check", api.Unproven, r.Validate().Error()
		case r.Person:
			it.Kind = "person"
			if c, ok := confirmed[api.GateKey(g.Name, r.Name)]; ok {
				c := c
				it.Status, it.ConfirmedBy = api.Met, &c
			} else {
				it.Status, it.Reason = api.Unmet, "waits for a person's confirmation"
			}
		case r.URI != "":
			it.Kind = "uri"
			it.Status, it.Reason = askAction(ctx, r.URI, obj)
		default:
			it.Kind = "check"
			if names, err := el.Names(r.Check); err == nil && names[api.FunctionName] {
				it.Status, it.Reason = api.Unproven, "a gate's check may not call gate()"
				break
			}
			exprs, exprAt = append(exprs, r.Check), append(exprAt, i)
		}
		out.Items[i] = it
	}
	if len(exprs) > 0 {
		ev := cwmapi.ObjectEvaluateInput{ID: obj.ID, Expressions: exprs, From: in.From, To: in.To, NoCostly: in.NoCostly}
		if in.By != nil {
			ev.By = *in.By
		}
		res, err := cwm.ObjectEvaluate(ctx, ev)
		for k, i := range exprAt {
			it := &out.Items[i]
			switch {
			case err != nil:
				it.Status, it.Reason = api.Unproven, err.Error()
			case k >= len(res.Results):
				it.Status, it.Reason = api.Unproven, "no result"
			case res.Results[k].Error != "":
				it.Status, it.Reason = api.Unproven, res.Results[k].Error
			case res.Results[k].Value == true:
				it.Status = api.Met
			case res.Results[k].Value == false || res.Results[k].Value == nil:
				it.Status, it.Reason = api.Unmet, answered(res.Results[k].Calls)
			default:
				it.Status, it.Reason = api.Unproven, fmt.Sprintf("answered %v, not true or false", res.Results[k].Value)
			}
		}
	}
	for _, it := range out.Items {
		if it.Status != api.Met {
			out.Passes = false
		}
	}
	return out
}

// answered says what the functions a check called answered: `verify():
// {"failed":1,...}`; "" when it called none.
func answered(calls []cwmapi.FunctionAnswer) string {
	lines := make([]string, 0, len(calls))
	for _, c := range calls {
		args, _ := json.Marshal(c.Args)
		a := strings.TrimSuffix(strings.TrimPrefix(string(args), "["), "]")
		if a == "null" {
			a = ""
		}
		v := c.Error
		if v == "" {
			b, _ := json.Marshal(c.Answer)
			v = string(b)
		}
		lines = append(lines, c.Function+"("+a+"): "+v)
	}
	return strings.Join(lines, "; ")
}

// askAction asks an action {accept, reason} over the object. No action
// service, or an action that fails, is unproven — a check that did not run
// has not passed.
func askAction(ctx context.Context, uri string, obj cwmapi.Object) (string, string) {
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil || actions == nil {
		return api.Unproven, "no action service to run " + uri
	}
	// A hook's default payload at the point "gate", answered as a before_* hook
	// is: a `uri:` requirement may be served by the same method as a hook.
	input, err := json.Marshal(cwmapi.HookCall{Point: "gate", Object: &obj})
	if err != nil {
		return api.Unproven, err.Error()
	}
	out, err := actions.Run(ctx, actionsapi.RunInput{URI: uri, Input: input})
	if err != nil {
		return api.Unproven, err.Error()
	}
	var v cwmapi.HookAccept
	if len(out.Value) == 0 || json.Unmarshal(out.Value, &v) != nil {
		return api.Unproven, uri + " answered no verdict"
	}
	if !v.Accept {
		if v.Reason == "" {
			v.Reason = "did not accept"
		}
		return api.Unmet, v.Reason
	}
	return api.Met, ""
}

// confirmations reads the person confirmations kept in the `gates` group —
// the `<gate>/<requirement>` entries; the revoked trace is not one.
func confirmations(obj cwmapi.Object) map[string]api.GateConfirmation {
	out := map[string]api.GateConfirmation{}
	for k, v := range obj.Capabilities[api.CapGates] {
		if !strings.Contains(k, "/") {
			continue
		}
		var c api.GateConfirmation
		if remarshal(v, &c) == nil {
			out[k] = c
		}
	}
	return out
}

// remarshal decodes a value read as any into v.
func remarshal(from, v any) error {
	b, err := json.Marshal(from)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// Confirm records a person's confirmation of one requirement.
func (s *Service) Confirm(ctx context.Context, in api.ConfirmInput) (api.ConfirmOutput, error) {
	if in.ID == "" || in.Gate == "" || in.Requirement == "" {
		return api.ConfirmOutput{}, fmt.Errorf("%w: id, gate and requirement required", cwmapi.ErrInvalidInput)
	}
	if in.By.ID == "" || in.By.Kind != cwmapi.ActorPerson {
		return api.ConfirmOutput{}, fmt.Errorf("%w: a requirement is confirmed by a person — by {id, kind: person}", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ConfirmOutput{}, err
	}
	obj, err := load(ctx, cwm, in.ID)
	if err != nil {
		return api.ConfirmOutput{}, err
	}
	g, err := s.gateOn(ctx, cwm, obj, in.Gate)
	if err != nil {
		return api.ConfirmOutput{}, err
	}
	person := false
	for _, r := range g.Requirements {
		if r.Name == in.Requirement {
			if !r.Person {
				return api.ConfirmOutput{}, fmt.Errorf("%w: %s/%s is checked, not confirmed by a person", cwmapi.ErrInvalidInput, in.Gate, in.Requirement)
			}
			person = true
		}
	}
	if !person {
		return api.ConfirmOutput{}, fmt.Errorf("%w: gate %s has no requirement %q for a %s", cwmapi.ErrNotFound, in.Gate, in.Requirement, obj.Type)
	}
	confirmation := map[string]any{
		"by": map[string]any{"id": in.By.ID, "kind": in.By.Kind}, "at": time.Now().UTC().Format(time.RFC3339), "note": in.Note,
	}
	out, err := writeGates(ctx, cwm, in.ID, func(group map[string]any) error {
		group[api.GateKey(in.Gate, in.Requirement)] = confirmation
		return nil
	})
	if err != nil {
		return api.ConfirmOutput{}, err
	}
	return api.ConfirmOutput{Checklist: checklist(ctx, cwm, out, g, api.CheckInput{ID: in.ID, Gate: in.Gate})}, nil
}

// writeGates changes an object's `gates` group with change, written on the
// version read and redone when another write landed in between (a planner,
// another person): a confirmation or a withdrawal never erases a concurrent
// write, and none is lost to one.
func writeGates(ctx context.Context, cwm cwmapi.CWMService, id string, change func(group map[string]any) error) (cwmapi.Object, error) {
	module, _ := cwmapi.ModuleOfObjectID(id)
	for attempt := 0; ; attempt++ {
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
		if err != nil {
			return cwmapi.Object{}, err
		}
		obj := got.Object
		group := map[string]any{}
		for k, v := range obj.Capabilities[api.CapGates] {
			group[k] = v
		}
		if err := change(group); err != nil {
			return cwmapi.Object{}, err
		}
		if obj.Capabilities == nil {
			obj.Capabilities = map[string]map[string]any{}
		}
		obj.Capabilities[api.CapGates] = group
		out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: obj, Revision: got.Revision})
		if err == nil {
			return out.Object, nil
		}
		if !errors.Is(err, cwmapi.ErrConflict) || attempt >= confirmRetries {
			return cwmapi.Object{}, err
		}
		time.Sleep(time.Duration(attempt+1) * time.Millisecond)
	}
}

// confirmRetries bounds how often a confirmation or a withdrawal is redone
// after losing a race to another write of the object.
const confirmRetries = 32

// Revoke withdraws a person's confirmation of one requirement: the entry
// leaves the `gates` group and a trace of it — what was confirmed, by whom
// and when, and who withdrew it, when and why — is appended to the group's
// `revoked` list. Only a person withdraws, as only a person confirms.
func (s *Service) Revoke(ctx context.Context, in api.RevokeInput) (api.RevokeOutput, error) {
	if in.ID == "" || in.Gate == "" || in.Requirement == "" {
		return api.RevokeOutput{}, fmt.Errorf("%w: id, gate and requirement required", cwmapi.ErrInvalidInput)
	}
	if in.By.ID == "" || in.By.Kind != cwmapi.ActorPerson {
		return api.RevokeOutput{}, fmt.Errorf("%w: a confirmation is withdrawn by a person — by {id, kind: person}", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RevokeOutput{}, err
	}
	obj, err := load(ctx, cwm, in.ID)
	if err != nil {
		return api.RevokeOutput{}, err
	}
	g, err := s.gateOn(ctx, cwm, obj, in.Gate)
	if err != nil {
		return api.RevokeOutput{}, err
	}
	key := api.GateKey(in.Gate, in.Requirement)
	var rev api.GateRevocation
	out, err := writeGates(ctx, cwm, in.ID, func(group map[string]any) error {
		raw, ok := group[key]
		if !ok {
			return fmt.Errorf("%w: %s has no confirmation of %s", cwmapi.ErrNotFound, in.ID, key)
		}
		rev = api.GateRevocation{
			Gate: in.Gate, Requirement: in.Requirement,
			By: cwmapi.Actor{ID: in.By.ID, Kind: in.By.Kind}, At: time.Now().UTC().Format(time.RFC3339), Note: in.Note,
		}
		_ = remarshal(raw, &rev.Confirmed)
		var entry map[string]any
		if err := remarshal(rev, &entry); err != nil {
			return err
		}
		var trace []any
		if prev, ok := group[api.RevokedKey].([]any); ok {
			trace = append(trace, prev...)
		}
		group[api.RevokedKey] = append(trace, entry)
		delete(group, key)
		return nil
	})
	if err != nil {
		return api.RevokeOutput{}, err
	}
	return api.RevokeOutput{Revoked: rev, Checklist: checklist(ctx, cwm, out, g, api.CheckInput{ID: in.ID, Gate: in.Gate})}, nil
}

// List answers every named gate that applies to an object — one with at
// least one requirement for the object's type, as Check resolves it — each
// with its checklist counted, in gate-name order.
func (s *Service) List(ctx context.Context, in api.ListInput) (api.ListOutput, error) {
	if in.ID == "" {
		return api.ListOutput{}, fmt.Errorf("%w: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ListOutput{}, err
	}
	obj, err := load(ctx, cwm, in.ID)
	if err != nil {
		return api.ListOutput{}, err
	}
	subj, err := subjectOf(ctx, cwm, obj)
	if err != nil {
		return api.ListOutput{}, err
	}
	d := s.definitions(ctx)
	out := api.ListOutput{Gates: []api.GateSummary{}}
	for _, name := range d.names() {
		g, _ := d.gateFor(name, subj)
		if len(g.Requirements) == 0 {
			continue
		}
		cl := checklist(ctx, cwm, obj, g, api.CheckInput{ID: in.ID, Gate: name})
		sum := api.GateSummary{Gate: cl.Gate, Label: cl.Label, Passes: cl.Passes, Items: cl.Items}
		for _, it := range cl.Items {
			switch it.Status {
			case api.Met:
				sum.Met++
			case api.Unmet:
				sum.Unmet++
			default:
				sum.Unproven++
			}
		}
		out.Gates = append(out.Gates, sum)
	}
	return out, nil
}

// Steps answers the steps the gate's requirements that apply to an object
// add (resolve.step), by priority then requirement name. No check is run: it
// reads declarations only. A requirement whose step is malformed fails it —
// the same requirement reads unproven in the checklist.
func (s *Service) Steps(ctx context.Context, in api.StepsInput) (api.StepsOutput, error) {
	if in.ID == "" || in.Gate == "" {
		return api.StepsOutput{}, fmt.Errorf("%w: id and gate required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.StepsOutput{}, err
	}
	obj, err := load(ctx, cwm, in.ID)
	if err != nil {
		return api.StepsOutput{}, err
	}
	g, err := s.gateOn(ctx, cwm, obj, in.Gate)
	if err != nil {
		return api.StepsOutput{}, err
	}
	out := api.StepsOutput{Gate: g.Name, Steps: []api.AddedStep{}}
	for _, r := range g.Requirements {
		st, ok, err := r.Step()
		if err != nil {
			return api.StepsOutput{}, err
		}
		if !ok {
			continue
		}
		provider := r.Provider
		if provider == "" {
			provider = g.Provider // the gate's own requirement
		}
		out.Steps = append(out.Steps, api.AddedStep{Requirement: r.Name, Provider: provider, Step: st})
	}
	sort.SliceStable(out.Steps, func(i, j int) bool {
		a, b := out.Steps[i], out.Steps[j]
		if a.Step.Priority != b.Step.Priority {
			return a.Step.Priority < b.Step.Priority
		}
		return a.Requirement < b.Requirement
	})
	return out, nil
}

// names is every declared gate's name, once each, sorted.
func (d definitions) names() []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range d.gates {
		if !seen[g.Name] {
			seen[g.Name] = true
			out = append(out, g.Name)
		}
	}
	sort.Strings(out)
	return out
}

// GateFunction is `gate(name)`: the object's checklist for the gate, for the
// move the expression is judging. In a preview (NoCostly) a check calling a
// costly function is not run; when only such checks keep the gate from
// passing, the gate is undecided and gate() answers ErrDecidedAtMove — the
// move decides it. A gate failing on anything else fails.
func (s *Service) GateFunction(ctx context.Context, in cwmapi.FunctionCall) (api.GateChecklist, error) {
	if len(in.Args) != 1 {
		return api.GateChecklist{}, fmt.Errorf("%w: gate() takes the gate's name", cwmapi.ErrInvalidInput)
	}
	name, ok := in.Args[0].(string)
	if !ok || name == "" {
		return api.GateChecklist{}, fmt.Errorf("%w: gate(%v): the name is a string", cwmapi.ErrInvalidInput, in.Args[0])
	}
	out, err := s.Check(ctx, api.CheckInput{ID: in.Object, Gate: name, By: in.By, From: in.From, To: in.To, NoCostly: in.NoCostly})
	if err != nil {
		return api.GateChecklist{}, err
	}
	if undecided(out.Checklist) {
		return api.GateChecklist{}, fmt.Errorf("gate(%s): %w", name, cwmapi.ErrDecidedAtMove)
	}
	return out.Checklist, nil
}

// undecided reports a checklist that does not pass only for checks left to
// the move.
func undecided(cl api.GateChecklist) bool {
	if cl.Passes {
		return false
	}
	atMove := false
	for _, it := range cl.Items {
		switch {
		case it.Status == api.Met:
		case it.Status == api.Unproven && strings.Contains(it.Reason, cwmapi.ErrDecidedAtMove.Error()):
			atMove = true
		default:
			return false
		}
	}
	return atMove
}
