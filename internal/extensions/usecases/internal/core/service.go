// SPDX-License-Identifier: Apache-2.0

// Package core implements the use-cases service over the CWM Object API. A
// criterion is an acceptance-criterion object under its use-case; the goals a
// use-case serves are its serves edges. Nothing is stored here.
package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/usecases/api"
)

// Service implements api.UseCasesService.
type Service struct{}

// New returns the use-cases service.
func New() *Service { return &Service{} }

var _ api.UseCasesService = (*Service)(nil)

func cwmOf(ctx context.Context) (cwmapi.CWMService, error) {
	return application.UseService[cwmapi.CWMService](ctx)
}

// get loads an object in full by id and refuses one of another type.
func get(ctx context.Context, cwm cwmapi.CWMService, id, typ string) (cwmapi.Object, string, error) {
	o, module, _, err := getRev(ctx, cwm, id, typ)
	return o, module, err
}

// getRev is get with the revision read: an update hands it back, so it never
// overwrites an object written since.
func getRev(ctx context.Context, cwm cwmapi.CWMService, id, typ string) (cwmapi.Object, string, string, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return cwmapi.Object{}, "", "", err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return cwmapi.Object{}, "", "", err
	}
	if typ != "" && got.Object.Type != typ {
		return cwmapi.Object{}, "", "", fmt.Errorf("%w: %s is a %q, not a %s", cwmapi.ErrInvalidInput, id, got.Object.Type, typ)
	}
	return got.Object, module, got.Revision, nil
}

// stateOf is the use-case's lifecycle state (the `manual` workflow); none is
// the workflow's initial state.
func stateOf(o cwmapi.Object) string {
	if l, ok := o.Lifecycle(); ok && l.State != "" {
		return l.State
	}
	return api.StateInitial
}

// live: not dropped — retired and split use-cases are dropped, the reason in
// their lifecycle history.
func live(o cwmapi.Object) bool { return stateOf(o) != api.StateDropped }

// drop moves a use-case to dropped through its workflow, the note saying why.
func drop(ctx context.Context, cwm cwmapi.CWMService, id string, actor cwmapi.Actor, note string) (cwmapi.Object, error) {
	if actor.ID == "" {
		return cwmapi.Object{}, fmt.Errorf("%w: use-cases: actor required — who retires or splits is recorded", cwmapi.ErrInvalidInput)
	}
	out, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: api.StateDropped, By: actor, Note: note})
	if err != nil {
		return cwmapi.Object{}, err
	}
	return out.Object, nil
}

// Add creates a use-case serving the goals it names.
func (s *Service) Add(ctx context.Context, in api.AddInput) (api.AddOutput, error) {
	if in.Module == "" || strings.TrimSpace(in.Title) == "" {
		return api.AddOutput{}, fmt.Errorf("%w: use-cases.Add: module and title required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.AddOutput{}, err
	}
	for _, g := range in.Serves {
		if _, _, err := get(ctx, cwm, g, "goal"); err != nil {
			return api.AddOutput{}, fmt.Errorf("use-cases.Add: serves %s: %w", g, err)
		}
	}
	parent := in.Parent
	if parent == "" {
		parent = in.Module
	}
	o := cwmapi.Object{Type: api.UseCaseType}
	o.Parent = parent
	setWant(&o, in.Title)
	for _, g := range in.Serves {
		o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeServes, To: g})
	}
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: in.Module, Object: o})
	if err != nil {
		return api.AddOutput{}, err
	}
	return api.AddOutput{UseCase: toUseCase(in.Module, out.Object), Path: out.Path}, nil
}

// setWant writes the title a use-case is read by. Its state is its lifecycle,
// which starts in the workflow's initial state.
func setWant(o *cwmapi.Object, title string) {
	o.Name = title
	_ = o.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "closable"})
}

// Split replaces a use-case with several through ObjectSplit, which writes the
// trail. Each successor is made a use-case in full — a want, pending — and
// serves every goal the source served, so no goal loses coverage by the split.
// The source stays, marked split, as the record of what was replaced. The
// count of successors and the reason are ObjectSplit's own checks.
func (s *Service) Split(ctx context.Context, in api.SplitInput) (api.SplitOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.SplitOutput{}, err
	}
	src, module, err := get(ctx, cwm, in.ID, api.UseCaseType)
	if err != nil {
		return api.SplitOutput{}, err
	}
	if !live(src) {
		return api.SplitOutput{}, fmt.Errorf("%w: use-cases.Split: %s is no longer live", cwmapi.ErrTypeRuleViolation, in.ID)
	}
	targets := make([]cwmapi.ObjectSplitTarget, len(in.Successors))
	for i, succ := range in.Successors {
		targets[i] = cwmapi.ObjectSplitTarget{Type: api.UseCaseType, Title: succ.Title, Children: succ.Criteria}
	}
	split, err := cwm.ObjectSplit(ctx, cwmapi.ObjectSplitInput{ID: in.ID, Successors: targets, Reason: in.Reason})
	if err != nil {
		return api.SplitOutput{}, err
	}
	var serves []cwmapi.ObjectEdge
	for _, e := range src.Edges {
		if e.Kind == api.EdgeServes {
			serves = append(serves, e)
		}
	}
	out := api.SplitOutput{Orphans: split.Orphans}
	for i, id := range split.Successors {
		o, m, rev, err := getRev(ctx, cwm, id, "")
		if err != nil {
			return api.SplitOutput{}, err
		}
		setWant(&o, in.Successors[i].Title)
		o.Edges = append(o.Edges, serves...)
		up, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: m, Object: o, Revision: rev})
		if err != nil {
			return api.SplitOutput{}, err
		}
		out.Successors = append(out.Successors, toUseCase(m, up.Object))
	}
	_ = module
	if _, err := drop(ctx, cwm, in.ID, in.Actor, "split: "+in.Reason); err != nil {
		return api.SplitOutput{}, err
	}
	return out, nil
}

// Retire marks a use-case retired. It stays, so what it once claimed and why it
// stopped is still readable; it leaves Uncovered and takes no new criterion.
func (s *Service) Retire(ctx context.Context, in api.RetireInput) (api.RetireOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RetireOutput{}, err
	}
	o, module, rev, err := getRev(ctx, cwm, in.ID, api.UseCaseType)
	if err != nil {
		return api.RetireOutput{}, err
	}
	if !live(o) {
		return api.RetireOutput{}, fmt.Errorf("%w: use-cases.Retire: %s is already retired or split", cwmapi.ErrTypeRuleViolation, in.ID)
	}
	if in.Actor.ID == "" {
		return api.RetireOutput{}, fmt.Errorf("%w: use-cases.Retire: actor required — who retires is recorded", cwmapi.ErrInvalidInput)
	}
	if r := strings.TrimSpace(in.Reason); r != "" {
		o.Body = appendProse(o.Body, "## Retired\n\n"+r)
		// The prose first, then the state: the other order would make this
		// write carry the stale state.
		if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o, Revision: rev}); err != nil {
			return api.RetireOutput{}, err
		}
	}
	dropped, err := drop(ctx, cwm, in.ID, in.Actor, "retired: "+strings.TrimSpace(in.Reason))
	if err != nil {
		return api.RetireOutput{}, err
	}
	return api.RetireOutput{UseCase: toUseCase(module, dropped)}, nil
}

// AttachCriterion creates an acceptance-criterion under a live use-case. With
// no probe the criterion is declared unmeasurable (`checkable.kind: none`)
// rather than given a probe that could only pass vacuously.
func (s *Service) AttachCriterion(ctx context.Context, in api.AttachCriterionInput) (api.AttachCriterionOutput, error) {
	if strings.TrimSpace(in.Title) == "" {
		return api.AttachCriterionOutput{}, fmt.Errorf("%w: use-cases.AttachCriterion: title required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.AttachCriterionOutput{}, err
	}
	uc, module, err := get(ctx, cwm, in.UseCase, api.UseCaseType)
	if err != nil {
		return api.AttachCriterionOutput{}, err
	}
	if !live(uc) {
		return api.AttachCriterionOutput{}, fmt.Errorf("%w: use-cases.AttachCriterion: %s is no longer live", cwmapi.ErrTypeRuleViolation, in.UseCase)
	}
	check := map[string]any{"kind": "none"}
	if strings.TrimSpace(in.Probe) != "" {
		sev := in.Severity
		if sev == "" {
			sev = "core"
		}
		check = map[string]any{"kind": "command", "probe": in.Probe, "severity": sev}
	}
	o := cwmapi.Object{Type: api.CriterionType}
	o.Parent = in.UseCase
	o.Name = in.Title
	_ = o.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "closable"})
	_ = o.SetCapability(cwmapi.CapCheckable, check)
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o})
	if err != nil {
		return api.AttachCriterionOutput{}, err
	}
	return api.AttachCriterionOutput{Criterion: toCriterion(out.Object), Path: out.Path}, nil
}

// Uncovered lists the live use-cases with no acceptance criterion under them.
func (s *Service) Uncovered(ctx context.Context, in api.UncoveredInput) (api.UncoveredOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.UncoveredOutput{}, err
	}
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: in.Module, Type: api.UseCaseType})
	if err != nil {
		return api.UncoveredOutput{}, err
	}
	out := []api.UseCase{}
	for _, o := range list.Objects {
		if !live(o) {
			continue
		}
		kids, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: o.ID, Type: api.CriterionType})
		if err != nil {
			return api.UncoveredOutput{}, err
		}
		if len(kids.Objects) == 0 {
			m, _ := cwmapi.ModuleOfObjectID(o.ID)
			out = append(out, toUseCase(m, o))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return api.UncoveredOutput{UseCases: out}, nil
}

// Goals lists the objects a use-case serves, read from its serves edges.
func (s *Service) Goals(ctx context.Context, in api.GoalsInput) (api.GoalsOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.GoalsOutput{}, err
	}
	if _, _, err := get(ctx, cwm, in.ID, api.UseCaseType); err != nil {
		return api.GoalsOutput{}, err
	}
	edges, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: in.ID, Kind: cwmapi.EdgeType(api.EdgeServes), Direction: cwmapi.DirectionOut})
	if err != nil {
		return api.GoalsOutput{}, err
	}
	out := api.GoalsOutput{Goals: []api.Served{}}
	for _, e := range edges.Edges {
		if e.From != in.ID {
			continue
		}
		served := api.Served{ID: e.To}
		if o, _, err := get(ctx, cwm, e.To, ""); err == nil {
			served.Type, served.Title = o.Type, o.DisplayTitle()
		}
		out.Goals = append(out.Goals, served)
	}
	return out, nil
}

func toUseCase(module string, o cwmapi.Object) api.UseCase {
	uc := api.UseCase{ID: o.ID, Module: module, Title: o.DisplayTitle(), Parent: o.ParentID(), Status: stateOf(o)}
	for _, e := range o.Edges {
		if e.Kind == api.EdgeServes {
			uc.Serves = append(uc.Serves, e.To)
		}
	}
	return uc
}

func toCriterion(o cwmapi.Object) api.Criterion {
	cr := api.Criterion{ID: o.ID, UseCase: o.ParentID(), Title: o.DisplayTitle()}
	if p, ok := o.Capabilities[cwmapi.CapCheckable]["probe"].(string); ok {
		cr.Probe = p
	}
	if s, ok := o.Capabilities[cwmapi.CapCheckable]["severity"].(string); ok {
		cr.Severity = s
	}
	return cr
}

// appendProse adds a section to a prose body. A structured body is refused
// rather than silently losing the section.
func appendProse(prose, section string) string {
	if prose != "" {
		prose = strings.TrimRight(prose, "\n") + "\n\n"
	}
	return prose + section + "\n"
}
