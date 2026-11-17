// SPDX-License-Identifier: Apache-2.0

// Package core is the object-pane service over CWM's reads.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// Service implements api.ObjectPaneService; CWM and the catalog are resolved
// from the context.
type Service struct{}

// New returns the service.
func New() *Service { return &Service{} }

// TransitionOptions lists every lifecycle of the object — its own, then each
// carried capability that follows a workflow — and the transitions from each
// current state (the id bare or qualified, each move judged on the object of
// the project holding it), each judged for `by` by CWM's dry-run of the move: what it
// calls allowed is what the move would pass. The dry-run is a preview
// (NoCostly): CWM does not run a costly function, and a guard calling one
// reads `at-move`, decided when the move is made. It writes nothing.
func (s *Service) TransitionOptions(ctx context.Context, in api.TransitionOptionsInput) (api.TransitionOptionsOutput, error) {
	if in.ID == "" {
		return api.TransitionOptionsOutput{}, fmt.Errorf("%w: id required", cwmapi.ErrInvalidInput)
	}
	by := in.By
	if by.Kind == "" {
		by.Kind = cwmapi.ActorPerson
	}
	if by.ID == "" {
		by.ID = "user"
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.TransitionOptionsOutput{}, fmt.Errorf("object-pane: resolve CWMService: %w", err)
	}
	module, err := cwmapi.ModuleOfObjectID(in.ID)
	if err != nil {
		return api.TransitionOptionsOutput{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.ID})
	if err != nil {
		return api.TransitionOptionsOutput{}, err
	}
	obj := got.Object
	decl := s.declarations(ctx)

	out := api.TransitionOptionsOutput{Lifecycles: []api.LifecycleOptions{}}
	for _, slot := range slotsOf(ctx, cwm, obj, decl) {
		wf, ok := decl.workflow(slot.workflow)
		if !ok {
			continue
		}
		// The dry-run and the journal name the object as Detail does
		// (qualified with its project when several are served): two projects
		// holding one bare id keep their own history.
		ref := refIn(got.Project, obj.ID)
		lo := options(ctx, cwm, ref, slot, wf, by)
		lo.History = historyOf(ctx, ref, slot.capability)
		lo.StateColors = decl.stateColors(slot)
		out.Lifecycles = append(out.Lifecycles, lo)
	}
	return out, nil
}

// workflow is the workflow `name`, as CWM finds it: the first declared.
func (d decl) workflow(name string) (cwmapi.Workflow, bool) {
	for _, w := range d.Workflows {
		if w.Name == name {
			return w, true
		}
	}
	return cwmapi.Workflow{}, false
}

// slot is one state machine of an object: its own lifecycle (capability ""),
// or a carried capability's own workflow, with its state and the field that
// holds it.
type slot struct {
	capability string
	workflow   string
	field      string
	state      string
}

// slotsOf reads the object's lifecycles the way CWM does: its own — its
// type's workflow — then each carried capability declaring a workflow, by
// name.
func slotsOf(ctx context.Context, cwm cwmapi.CWMService, obj cwmapi.Object, d decl) []slot {
	var out []slot
	life, _ := obj.Lifecycle()
	own := slot{field: lifecycleStateField, state: life.State}
	if t, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: obj.Type}); err == nil {
		own.workflow = t.ObjectType.Workflow
	}
	if own.workflow != "" {
		out = append(out, own)
	}
	var names []string
	caps := d.capabilities()
	for id := range obj.Capabilities {
		if c, ok := caps[id]; ok && c.Workflow != nil && c.Workflow.Name != "" && c.Workflow.State != "" {
			names = append(names, id)
		}
	}
	sort.Strings(names)
	for _, id := range names {
		w := caps[id].Workflow
		group := obj.Capabilities[id]
		sl := slot{capability: id, workflow: w.Name, field: w.State}
		sl.state, _ = group[w.State].(string)
		out = append(out, sl)
	}
	return out
}

func options(ctx context.Context, cwm cwmapi.CWMService, id string, sl slot, wf cwmapi.Workflow, by cwmapi.Actor) api.LifecycleOptions {
	from := sl.state
	if from == "" {
		from = wf.Initial
	}
	lo := api.LifecycleOptions{
		Capability: sl.capability, Workflow: wf.Name, WorkflowLabel: wf.Label,
		State: from, States: wf.States, Transitions: []api.TransitionOption{},
	}
	if st, ok := wf.StateNamed(from); ok && st.Terminal {
		lo.Terminal = true
		return lo
	}
	for _, tr := range wf.Transitions {
		if !slices.Contains(tr.From, from) && !slices.Contains(tr.From, "*") {
			continue
		}
		opt := api.TransitionOption{Name: tr.Name, To: tr.To, Guard: tr.Gate, Force: tr.Force, GuardStatus: api.GuardNone}
		judge(ctx, cwm, id, sl.capability, tr, by, &opt)
		lo.Transitions = append(lo.Transitions, opt)
	}
	return lo
}

// historyOf is a lifecycle's moves, oldest first, as the journal derives them
// (the object stores only its state). Best effort: no journal, no history.
func historyOf(ctx context.Context, id, capability string) []cwmapi.LifecycleEntry {
	journal, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil {
		return nil
	}
	got, err := journal.Transitions(ctx, journalapi.TransitionsInput{ID: id, Capability: capability})
	if err != nil {
		slog.Debug("object-pane: lifecycle history unavailable", "object", id, "capability", capability, "error", err)
		return nil
	}
	return got.Entries
}

// timesOf is when the object was created and last changed, as the journal
// derives them. Best effort: no journal, no times.
func timesOf(ctx context.Context, id string) (created, updated time.Time) {
	journal, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil {
		return time.Time{}, time.Time{}
	}
	got, err := journal.Times(ctx, journalapi.TimesInput{ID: id})
	if err != nil {
		slog.Debug("object-pane: times unavailable", "object", id, "error", err)
		return time.Time{}, time.Time{}
	}
	return got.CreatedAt, got.UpdatedAt
}

// stateColors is how the slot's capability colours its states — the badge it
// declares on the state field (the lifecycle capability's for the object's
// own), so a state reads the same in the pane as in the header and the lists.
func (d decl) stateColors(sl slot) map[string]string {
	name := sl.capability
	if name == "" {
		name = cwmapi.CapLifecycle
	}
	c, ok := d.capabilities()[name]
	if !ok {
		return nil
	}
	for _, b := range c.Badges {
		if b.Field == sl.field && len(b.Colors) > 0 {
			return b.Colors
		}
	}
	return nil
}

// judge asks CWM whether the move would pass, without making it — a preview,
// so a costly function is not run.
func judge(ctx context.Context, cwm cwmapi.CWMService, id, capability string, tr cwmapi.WorkflowTransition, by cwmapi.Actor, opt *api.TransitionOption) {
	_, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{
		ID: id, To: tr.To, Transition: tr.Name, By: by, Capability: capability, DryRun: true, NoCostly: true,
	})
	if err == nil {
		if tr.Gate != "" {
			opt.GuardStatus = api.GuardHolds
		}
		opt.Allowed = true
		return
	}
	var refused *cwmapi.TransitionRefusedError
	if errors.As(err, &refused) {
		opt.Calls = refused.Calls
		opt.Reason = refusalReason(refused, humanReason(refused.Reason))
		// A costly function not run for the preview (AtMove), or one that
		// could not answer now, leaves the guard to the move: the option is
		// not refused on a call that may succeed then.
		if anyErrored(refused.Calls) {
			opt.GuardStatus, opt.Allowed = api.GuardAtMove, true
			return
		}
	} else {
		opt.Reason = humanReason(err.Error())
	}
	if opt.Reason != err.Error() {
		// The pane shows the human part; the whole error stays in the log.
		slog.Debug("object-pane: transition preview refused", "object", id, "transition", tr.Name, "error", err)
	}
	opt.GuardStatus = api.GuardFails
}

// exprPosition is the "(line:column)" an expression error ends its first line with.
var exprPosition = regexp.MustCompile(`\s*\(\d+:\d+\)$`)

// guardPrefix is the `the guard "<el>": ` an unevaluable guard's reason opens with.
var guardPrefix = regexp.MustCompile(`^the guard "(?:[^"\\]|\\.)*": `)

// dispatchPlumbing is what a guard function's error gains crossing the action
// dispatcher: "ActionService.Run <uri> (in-process): ".
var dispatchPlumbing = regexp.MustCompile(`ActionService\.Run \S+(?: \([^)]*\))?: `)

// humanReason is the part of a refusal a person reads: the innermost message,
// without the dispatch plumbing a guard function's error crossed, the guard's
// own prefix (the option shows the guard), and the expression's position,
// source excerpt and caret.
func humanReason(reason string) string {
	line, _, _ := strings.Cut(reason, "\n")
	line = exprPosition.ReplaceAllString(strings.TrimSpace(line), "")
	line = guardPrefix.ReplaceAllString(line, "")
	line = dispatchPlumbing.ReplaceAllString(line, "")
	if line == "" {
		return reason
	}
	return line
}

// refusalReason is the reason a refused guard shows beside its calls. The
// pane draws a gate('…') call's checklist item by item, so a reason that only
// repeats it says nothing more: a guard whose every call is such a checklist
// shows none, and one that called other functions names only those, each with
// its error or plain answer (a structured answer stays in Calls, never inlined).
func refusalReason(refused *cwmapi.TransitionRefusedError, reason string) string {
	if len(refused.Calls) == 0 || !strings.Contains(refused.Reason, " does not hold") {
		return reason
	}
	var others []string
	for _, c := range refused.Calls {
		if drawnChecklist(c) {
			continue
		}
		others = append(others, callLine(c))
	}
	if len(others) == len(refused.Calls) {
		return reason // nothing the pane draws: cwm's own line stays
	}
	if len(others) == 0 {
		return ""
	}
	return fmt.Sprintf("the guard %q does not hold (%s)", refused.Guard, strings.Join(others, "; "))
}

// drawnChecklist reports the call the pane draws as a checklist: a gate('…')
// that answered a list of items (the GUI's gateChecklistOf reads the same).
func drawnChecklist(c cwmapi.FunctionAnswer) bool {
	if c.Function != "gate" || c.Error != "" || c.Answer == nil {
		return false
	}
	var answer struct {
		Items []any `json:"items"`
	}
	b, err := json.Marshal(c.Answer)
	if err != nil || json.Unmarshal(b, &answer) != nil {
		return false
	}
	return answer.Items != nil
}

// callLine is one undrawn call in a reason: the function and its arguments,
// then its error (without the dispatch plumbing) or its plain answer.
func callLine(c cwmapi.FunctionAnswer) string {
	args := make([]string, 0, len(c.Args))
	for _, a := range c.Args {
		b, _ := json.Marshal(a)
		args = append(args, string(b))
	}
	call := c.Function + "(" + strings.Join(args, ", ") + ")"
	if c.Error != "" {
		return call + ": " + humanReason(c.Error)
	}
	switch v := c.Answer.(type) {
	case bool, string, float64, float32, int, int64, int32, uint, uint64, uint32:
		return fmt.Sprintf("%s: %v", call, v)
	}
	return call
}

func anyErrored(calls []cwmapi.FunctionAnswer) bool {
	for _, c := range calls {
		if c.Error != "" {
			return true
		}
	}
	return false
}
