// SPDX-License-Identifier: Apache-2.0

// Package core implements the decisions service over the CWM Object API. A
// decision's state is its `decision` field-group; its reasoning is the prose
// body. Nothing is stored here.
package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person"
)

// state is the `decision` field-group (cap-decision).
type state struct {
	Options  []string `json:"options,omitempty"`
	Selected []string `json:"selected,omitempty"`
	Raised   string   `json:"raised,omitempty"`
	Resolved string   `json:"resolved,omitempty"`
	RaisedBy string   `json:"raised_by,omitempty"`
	Source   string   `json:"source,omitempty"`
	// Proposal and Challenge: the preparation's recommendation, the points
	// raised against it.
	Proposal  *api.Proposal          `json:"proposal,omitempty"`
	Challenge *api.ProposalChallenge `json:"challenge,omitempty"`
	// Relayed: the person's decision was relayed by an agent's session, and
	// how the person confirmed it.
	Relayed *api.Relayed `json:"relayed,omitempty"`
	// Owner is whose it is (api.OwnerPerson when empty) and OwnerReason why
	// an agent may take it.
	Owner       string `json:"owner,omitempty"`
	OwnerReason string `json:"owner_reason,omitempty"`
}

// ownerOf is whose a decision is: the person's unless it says an agent's.
func ownerOf(st state) string {
	if st.Owner == api.OwnerAgent {
		return api.OwnerAgent
	}
	return api.OwnerPerson
}

// Service implements api.DecisionsService.
type Service struct {
	now func() time.Time
}

// New returns the decisions service.
func New() *Service { return &Service{now: time.Now} }

var _ api.DecisionsService = (*Service)(nil)

func (s *Service) today() string { return s.now().UTC().Format(dateLayout) }

func cwmOf(ctx context.Context) (cwmapi.CWMService, error) {
	return application.UseService[cwmapi.CWMService](ctx)
}

// loaded is a decision read in full (body included): its module, its state
// and the revision read, which makes the write that follows conditional.
type loaded struct {
	obj      cwmapi.Object
	module   string
	st       state
	revision string
}

// load reads a decision in full (body included) and its state.
func load(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, string, state, error) {
	l, err := loadRev(ctx, cwm, id)
	return l.obj, l.module, l.st, err
}

// loadRev is load with the revision read.
func loadRev(ctx context.Context, cwm cwmapi.CWMService, id string) (loaded, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return loaded{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return loaded{}, err
	}
	if got.Object.Type != api.DecisionType {
		return loaded{}, fmt.Errorf("%w: %s is a %q, not a decision", cwmapi.ErrInvalidInput, id, got.Object.Type)
	}
	st, _, err := cwmapi.DecodeCapability[state](got.Object, api.CapDecision)
	if err != nil {
		return loaded{}, err
	}
	return loaded{obj: got.Object, module: module, st: st, revision: got.Revision}, nil
}

// stateOf is the decision's lifecycle state; a decision with none is open (the
// `decision` workflow's initial state).
func stateOf(o cwmapi.Object) string {
	if l, ok := o.Lifecycle(); ok && l.State != "" {
		return l.State
	}
	return api.StatusOpen
}

// isOpen: the lifecycle is the one state.
func isOpen(o cwmapi.Object, _ state) bool { return stateOf(o) == api.StatusOpen }

// transition moves a decision through its workflow — the one way its state
// changes; the workflow's own checks refuse what it does not allow.
func transition(ctx context.Context, cwm cwmapi.CWMService, id, to string, actor cwmapi.Actor, note string) error {
	_, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: to, By: actor, Note: note})
	return err
}

// saveAt writes the decision over the revision read: a write made since is
// refused with ErrConflict, never overwritten.
func saveAt(ctx context.Context, cwm cwmapi.CWMService, module string, o cwmapi.Object, st state, revision string) (api.Decision, error) {
	if err := o.SetCapability(api.CapDecision, st); err != nil {
		return api.Decision{}, err
	}
	up, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o, Revision: revision})
	if err != nil {
		return api.Decision{}, err
	}
	return toDecision(module, up.Object), nil
}

// Open creates an open decision with the question as its first prose section.
func (s *Service) Open(ctx context.Context, in api.OpenInput) (api.OpenOutput, error) {
	if in.Module == "" || strings.TrimSpace(in.Question) == "" {
		return api.OpenOutput{}, fmt.Errorf("%w: decisions.Open: module and question required", cwmapi.ErrInvalidInput)
	}
	switch in.Owner {
	case "", api.OwnerPerson:
		in.Owner, in.OwnerReason = "", ""
	case api.OwnerAgent:
		if strings.TrimSpace(in.OwnerReason) == "" {
			return api.OpenOutput{}, fmt.Errorf("%w: decisions.Open: an agent's decision says why an agent may take it (owner_reason)", cwmapi.ErrInvalidInput)
		}
	default:
		return api.OpenOutput{}, fmt.Errorf("%w: decisions.Open: owner %q: person or agent", cwmapi.ErrInvalidInput, in.Owner)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.OpenOutput{}, err
	}
	// The question is the TITLE. Prose is written only when there is context
	// beyond it: repeating the title in the body puts one datum in two places,
	// which the object rules refuse.
	o := cwmapi.Object{Type: api.DecisionType}
	o.Parent = in.Module
	o.Name = in.Question
	if prose := strings.TrimSpace(in.Context); prose != "" {
		o.Body = "## Question\n\n" + prose + "\n"
	}
	if err := o.SetCapability(api.CapDecision, state{
		Options: dedupe(in.Options), Raised: s.today(), RaisedBy: in.RaisedBy, Source: in.Source,
		Owner: in.Owner, OwnerReason: strings.TrimSpace(in.OwnerReason),
	}); err != nil {
		return api.OpenOutput{}, err
	}
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: in.Module, Object: o})
	if err != nil {
		return api.OpenOutput{}, err
	}
	return api.OpenOutput{Decision: toDecision(in.Module, out.Object), Path: out.Path}, nil
}

// ProposeOption adds an option. Only an open decision takes new options: an
// decided one is changed by superseding it, never by editing its options.
func (s *Service) ProposeOption(ctx context.Context, in api.ProposeOptionInput) (api.ProposeOptionOutput, error) {
	if strings.TrimSpace(in.Option) == "" {
		return api.ProposeOptionOutput{}, fmt.Errorf("%w: decisions.ProposeOption: option required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ProposeOptionOutput{}, err
	}
	l, err := loadRev(ctx, cwm, in.ID)
	if err != nil {
		return api.ProposeOptionOutput{}, err
	}
	o, module, st := l.obj, l.module, l.st
	if !isOpen(o, st) {
		return api.ProposeOptionOutput{}, fmt.Errorf("%w: decisions.ProposeOption: %s is not open; supersede it instead", cwmapi.ErrTypeRuleViolation, in.ID)
	}
	if slices.Contains(st.Options, in.Option) {
		return api.ProposeOptionOutput{}, fmt.Errorf("%w: decisions.ProposeOption: %s already has option %q", cwmapi.ErrAlreadyExists, in.ID, in.Option)
	}
	st.Options = append(st.Options, in.Option)
	d, err := saveAt(ctx, cwm, module, o, st, l.revision)
	if err != nil {
		return api.ProposeOptionOutput{}, err
	}
	return api.ProposeOptionOutput{Decision: d}, nil
}

// Resolve decides an open decision; it then applies. When options were weighed, the choice must
// be among them — a choice nobody proposed is an option to propose first.
func (s *Service) Resolve(ctx context.Context, in api.ResolveInput) (api.ResolveOutput, error) {
	if len(in.Selected) == 0 {
		return api.ResolveOutput{}, fmt.Errorf("%w: decisions.Resolve: selected required", cwmapi.ErrInvalidInput)
	}
	// A person decides themselves: a call whose cause is an agent (an agent
	// run, a command an agent's session runs) naming a person as who decides
	// is refused - the actor is the client's word, the cause is not - unless
	// it relays that person's decision, confirmed, with relaying on (iam's one
	// rule); the decision then records the relay.
	var relayed *api.Relayed
	if in.Actor.Kind == cwmapi.ActorPerson {
		r, err := person.AcceptRelayed(ctx, in.Actor.ID, "decide:"+in.ID, "decisions.Resolve: "+in.ID+" is decided by the person themselves (the Decision panel, or their own command)")
		if err != nil {
			return api.ResolveOutput{}, fmt.Errorf("%w: %v", cwmapi.ErrInvalidInput, err)
		}
		if r != nil {
			c, _ := cause.CauseFrom(ctx)
			relayed = &api.Relayed{By: c.ID, Confirmed: r.Confirmed, At: r.At}
		}
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ResolveOutput{}, err
	}
	l, err := loadRev(ctx, cwm, in.ID)
	if err != nil {
		return api.ResolveOutput{}, err
	}
	o, module, st := l.obj, l.module, l.st
	if !isOpen(o, st) {
		return api.ResolveOutput{}, alreadyDecided(ctx, o, st)
	}
	if len(st.Options) > 0 {
		for _, sel := range in.Selected {
			if !slices.Contains(st.Options, sel) {
				return api.ResolveOutput{}, fmt.Errorf("%w: decisions.Resolve: %q is not an option of %s (%s)", cwmapi.ErrInvalidInput, sel, in.ID, strings.Join(st.Options, ", "))
			}
		}
	}
	if in.Actor.ID == "" {
		return api.ResolveOutput{}, fmt.Errorf("%w: decisions.Resolve: actor required — who decides is recorded", cwmapi.ErrInvalidInput)
	}
	// The person's decision is the person's; an agent takes an agent's, and
	// says why.
	agentTakes := in.Actor.Kind != cwmapi.ActorPerson
	if agentTakes && ownerOf(st) == api.OwnerPerson {
		return api.ResolveOutput{}, fmt.Errorf("%w: decisions.Resolve: %s is the person's decision: the person decides it (an agent takes only a decision marked an agent's)", cwmapi.ErrTypeRuleViolation, in.ID)
	}
	if agentTakes && strings.TrimSpace(in.Rationale) == "" {
		return api.ResolveOutput{}, fmt.Errorf("%w: decisions.Resolve: an agent deciding %s gives its reasons (rationale)", cwmapi.ErrInvalidInput, in.ID)
	}
	st.Selected = dedupe(in.Selected)
	st.Resolved = s.today()
	st.Relayed = relayed
	section := "## Decision\n\n" + strings.Join(st.Selected, ", ") + "."
	if r := strings.TrimSpace(in.Rationale); r != "" {
		section += " " + r
	}
	if relayed != nil {
		section += fmt.Sprintf("\n\nRelayed by %s, confirmed by %s (%s).", relayed.By, in.Actor.ID, relayed.Confirmed)
	}
	if agentTakes {
		section += fmt.Sprintf("\n\nTaken by the agent %s: an agent's decision - why is its owner reason. The person may reopen it.", in.Actor.ID)
	}
	o.Body = appendProse(o.Body, section)
	// The service's own fields first, then the state: a transition first would
	// make this write carry the stale state, which ObjectUpdate refuses. The
	// write is conditional on the revision read: of two resolves racing, the
	// later write is refused and answers who decided.
	if _, err := saveAt(ctx, cwm, module, o, st, l.revision); err != nil {
		if errors.Is(err, cwmapi.ErrConflict) {
			if now, lerr := loadRev(ctx, cwm, in.ID); lerr == nil && (!isOpen(now.obj, now.st) || now.st.Resolved != "") {
				return api.ResolveOutput{}, alreadyDecided(ctx, now.obj, now.st)
			}
		}
		return api.ResolveOutput{}, err
	}
	if err := transition(ctx, cwm, in.ID, api.StatusApplies, in.Actor, strings.Join(st.Selected, ", ")); err != nil {
		return api.ResolveOutput{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.ID})
	if err != nil {
		return api.ResolveOutput{}, err
	}
	return api.ResolveOutput{Decision: toDecision(module, got.Object)}, nil
}

// Reopen opens again a decision that applies: the person's act, saying why.
// Its choice is kept in its history (its prose and its lifecycle moves); its
// selection and resolved date are cleared, so it reads open.
func (s *Service) Reopen(ctx context.Context, in api.ReopenInput) (api.ReopenOutput, error) {
	if strings.TrimSpace(in.Reason) == "" {
		return api.ReopenOutput{}, fmt.Errorf("%w: decisions.Reopen: reason required", cwmapi.ErrInvalidInput)
	}
	if in.Actor.Kind != cwmapi.ActorPerson {
		return api.ReopenOutput{}, fmt.Errorf("%w: decisions.Reopen: reopening %s is a person's act", cwmapi.ErrTypeRuleViolation, in.ID)
	}
	if _, err := person.AcceptRelayed(ctx, in.Actor.ID, "reopen:"+in.ID, "decisions.Reopen: "+in.ID+" is reopened by the person themselves"); err != nil {
		return api.ReopenOutput{}, fmt.Errorf("%w: %v", cwmapi.ErrInvalidInput, err)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReopenOutput{}, err
	}
	l, err := loadRev(ctx, cwm, in.ID)
	if err != nil {
		return api.ReopenOutput{}, err
	}
	o, module, st := l.obj, l.module, l.st
	if stateOf(o) != api.StatusApplies {
		return api.ReopenOutput{}, fmt.Errorf("%w: decisions.Reopen: %s is %s; only a decision that applies is reopened", cwmapi.ErrTypeRuleViolation, in.ID, stateOf(o))
	}
	was := strings.Join(st.Selected, ", ")
	st.Selected, st.Resolved, st.Relayed = nil, "", nil
	o.Body = appendProse(o.Body, fmt.Sprintf("## Reopened\n\n%s reopened it (it was %s): %s", in.Actor.ID, was, strings.TrimSpace(in.Reason)))
	if _, err := saveAt(ctx, cwm, module, o, st, l.revision); err != nil {
		return api.ReopenOutput{}, err
	}
	if err := transition(ctx, cwm, in.ID, api.StatusOpen, in.Actor, "reopened: "+strings.TrimSpace(in.Reason)); err != nil {
		return api.ReopenOutput{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.ID})
	if err != nil {
		return api.ReopenOutput{}, err
	}
	return api.ReopenOutput{Decision: toDecision(module, got.Object)}, nil
}

// Supersede writes the supersedes edge on the newer decision and marks the
// older one superseded. Only a decision that applies is superseded: an open one
// is still being decided, and is resolved instead.
func (s *Service) Supersede(ctx context.Context, in api.SupersedeInput) (api.SupersedeOutput, error) {
	if in.ID == "" || in.By == "" || in.ID == in.By {
		return api.SupersedeOutput{}, fmt.Errorf("%w: decisions.Supersede: two different decisions required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.SupersedeOutput{}, err
	}
	old, module, st, err := load(ctx, cwm, in.ID)
	if err != nil {
		return api.SupersedeOutput{}, err
	}
	if in.Actor.ID == "" {
		return api.SupersedeOutput{}, fmt.Errorf("%w: decisions.Supersede: actor required — who records it is kept", cwmapi.ErrInvalidInput)
	}
	switch stateOf(old) {
	case api.StatusOpen:
		return api.SupersedeOutput{}, fmt.Errorf("%w: decisions.Supersede: %s is still open; resolve it instead", cwmapi.ErrTypeRuleViolation, in.ID)
	case api.StatusSuperseded, api.StatusRetired, api.StatusDropped:
		return api.SupersedeOutput{}, fmt.Errorf("%w: decisions.Supersede: %s is already %s", cwmapi.ErrTypeRuleViolation, in.ID, stateOf(old))
	}
	_ = st
	if _, _, _, err := load(ctx, cwm, in.By); err != nil {
		return api.SupersedeOutput{}, err
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: in.By, To: in.ID, Kind: cwmapi.EdgeType(api.EdgeSupersedes)}); err != nil {
		return api.SupersedeOutput{}, err
	}
	if err := transition(ctx, cwm, in.ID, api.StatusSuperseded, in.Actor, "superseded by "+in.By); err != nil {
		return api.SupersedeOutput{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.ID})
	if err != nil {
		return api.SupersedeOutput{}, err
	}
	return api.SupersedeOutput{Decision: toDecision(module, got.Object)}, nil
}

// Record writes down a decision already taken: a decision created with its
// original dates and choice, then moved open → applies by the actor, so
// its history says who recorded it and its fields say when it was decided. The
// same checks as Open + Resolve hold: a choice is one of the options when
// there are any.
func (s *Service) Record(ctx context.Context, in api.RecordInput) (api.RecordOutput, error) {
	if in.Module == "" || strings.TrimSpace(in.Question) == "" {
		return api.RecordOutput{}, fmt.Errorf("%w: decisions.Record: module and question required", cwmapi.ErrInvalidInput)
	}
	selected := dedupe(in.Selected)
	if len(selected) == 0 {
		return api.RecordOutput{}, fmt.Errorf("%w: decisions.Record: selected required", cwmapi.ErrInvalidInput)
	}
	options := dedupe(in.Options)
	if len(options) > 0 {
		for _, sel := range selected {
			if !slices.Contains(options, sel) {
				return api.RecordOutput{}, fmt.Errorf("%w: decisions.Record: %q is not one of the options (%s)", cwmapi.ErrInvalidInput, sel, strings.Join(options, ", "))
			}
		}
	}
	if in.Actor.ID == "" {
		return api.RecordOutput{}, fmt.Errorf("%w: decisions.Record: actor required — who records it is kept", cwmapi.ErrInvalidInput)
	}
	raised, err := time.Parse(dateLayout, in.Raised)
	if err != nil {
		return api.RecordOutput{}, fmt.Errorf("%w: decisions.Record: raised %q is not a YYYY-MM-DD date", cwmapi.ErrInvalidInput, in.Raised)
	}
	resolvedOn := in.Resolved
	if resolvedOn == "" {
		resolvedOn = in.Raised
	}
	resolved, err := time.Parse(dateLayout, resolvedOn)
	if err != nil {
		return api.RecordOutput{}, fmt.Errorf("%w: decisions.Record: resolved %q is not a YYYY-MM-DD date", cwmapi.ErrInvalidInput, in.Resolved)
	}
	if resolved.Before(raised) {
		return api.RecordOutput{}, fmt.Errorf("%w: decisions.Record: resolved %s is before raised %s", cwmapi.ErrInvalidInput, resolvedOn, in.Raised)
	}
	if resolved.After(s.now()) {
		return api.RecordOutput{}, fmt.Errorf("%w: decisions.Record: resolved %s is in the future", cwmapi.ErrInvalidInput, resolvedOn)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RecordOutput{}, err
	}
	o := cwmapi.Object{Type: api.DecisionType}
	o.Parent = in.Module
	o.Name = in.Question
	var body string
	if prose := strings.TrimSpace(in.Context); prose != "" {
		body = "## Question\n\n" + prose + "\n"
	}
	section := "## Decision\n\n" + strings.Join(selected, ", ") + "."
	if r := strings.TrimSpace(in.Rationale); r != "" {
		section += " " + r
	}
	body = appendProse(body, section)
	if c := strings.TrimSpace(in.Consequences); c != "" {
		body = appendProse(body, "## Consequences\n\n"+c)
	}
	o.Body = body
	if err := o.SetCapability(api.CapDecision, state{
		Options: options, Selected: selected, Raised: in.Raised, Resolved: resolvedOn,
		RaisedBy: in.RaisedBy, Source: in.Source,
	}); err != nil {
		return api.RecordOutput{}, err
	}
	created, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: in.Module, Object: o})
	if err != nil {
		return api.RecordOutput{}, err
	}
	id := created.Object.ID
	if err := transition(ctx, cwm, id, api.StatusApplies, in.Actor, "recorded; decided "+resolvedOn); err != nil {
		return api.RecordOutput{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: in.Module, ID: id})
	if err != nil {
		return api.RecordOutput{}, err
	}
	return api.RecordOutput{Decision: toDecision(in.Module, got.Object), Path: created.Path}, nil
}

// dateLayout is the decision dates' format.
const dateLayout = "2006-01-02"

// ListOpen returns a module's decisions that are still open.
func (s *Service) ListOpen(ctx context.Context, in api.ListOpenInput) (api.ListOpenOutput, error) {
	if in.Module == "" {
		return api.ListOpenOutput{}, fmt.Errorf("%w: decisions.ListOpen: module required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ListOpenOutput{}, err
	}
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: in.Module, Type: api.DecisionType})
	if err != nil {
		return api.ListOpenOutput{}, err
	}
	out := []api.Decision{}
	for _, o := range list.Objects {
		st, _, err := cwmapi.DecodeCapability[state](o, api.CapDecision)
		if err != nil {
			return api.ListOpenOutput{}, err
		}
		if isOpen(o, st) {
			out = append(out, toDecision(in.Module, o))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return api.ListOpenOutput{Decisions: out}, nil
}

func toDecision(module string, o cwmapi.Object) api.Decision {
	st, _, _ := cwmapi.DecodeCapability[state](o, api.CapDecision)
	return api.Decision{
		ID: o.ID, Module: module, Question: o.DisplayTitle(), Status: stateOf(o),
		Options: st.Options, Selected: st.Selected, Raised: st.Raised, Resolved: st.Resolved,
		RaisedBy: st.RaisedBy, Source: st.Source, Proposal: st.Proposal, Challenge: st.Challenge, Relayed: st.Relayed,
		Owner: ownerOf(st), OwnerReason: st.OwnerReason,
	}
}

// appendProse adds a section to a prose body. A structured body is left alone:
// it is some other writer's shape, and the state fields carry the answer.
func appendProse(prose, section string) string {
	if prose != "" {
		prose = strings.TrimRight(prose, "\n") + "\n\n"
	}
	return prose + section + "\n"
}

func dedupe(in []string) []string {
	var out []string
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}
