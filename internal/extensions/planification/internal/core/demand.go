// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A DEMAND RECONCILES — its reconcile step answers a delta (more.delta), a
// closed set of operations: planification checks it before anything is
// recorded (each op known, each object and module there) and keeps it on the
// demand; the person's approval of the step after is the one approval; the
// apply step answers which object each operation changed or created
// (more.applied): planification checks that every operation was applied,
// records a realized-by edge to each, and tells a person requester what the
// demand became.

// deltaOf reads the delta a result reports, or nil.
func deltaOf(r *schedapi.Result) []api.DeltaOp {
	if r == nil || r.More == nil || r.More[api.DeltaKey] == nil {
		return nil
	}
	var d []api.DeltaOp
	fromMore(r, api.DeltaKey, &d)
	return d
}

// appliedOf reads what an apply step reports, or nil.
func appliedOf(r *schedapi.Result) []api.Applied {
	if r == nil || r.More == nil || r.More[api.AppliedKey] == nil {
		return nil
	}
	var a []api.Applied
	fromMore(r, api.AppliedKey, &a)
	return a
}

// checkDelta refuses a delta that cannot be applied: an empty one, an unknown
// operation, an object or a module that is not there, an operation missing
// what it acts on. Every problem is named at once, one per line, each with
// its operation, so the reconcile fixes them in one go.
func checkDelta(ctx context.Context, cwm cwmapi.CWMService, d []api.DeltaOp) error {
	if len(d) == 0 {
		return invalid("a reconcile answers its delta (more.delta): at least one operation - or, for a small fix, the intake makes it direct work")
	}
	exists := func(id string) bool {
		_, err := loadObject(ctx, cwm, id)
		return err == nil
	}
	var problems []string
	say := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	for i, op := range d {
		at := fmt.Sprintf("delta[%d] (%s)", i, op.Op)
		if err := checkRefs(d, i, at); err != nil {
			problems = append(problems, strings.TrimPrefix(strings.TrimPrefix(err.Error(), cwmapi.ErrInvalidInput.Error()+": "), "requests: "))
		}
		exists := func(id string) bool { return isRef(id) || exists(id) }
		if !slices.Contains(api.DeltaOps, op.Op) {
			say("%s: not one of %s", at, strings.Join(api.DeltaOps, ", "))
			continue
		}
		if strings.TrimSpace(op.Why) == "" {
			say("%s: says why (why)", at)
		}
		switch op.Op {
		case api.OpGoalAdd, api.OpFeatureAdd, api.OpIncrementCreate, api.OpDirect:
			if op.Module == "" || !exists(op.Module) || strings.TrimSpace(op.Title) == "" {
				say("%s: names a module that exists and a title", at)
			}
			if op.Under != "" && !exists(op.Under) {
				say("%s: %s, what it goes under, is not there", at, op.Under)
			}
		default:
			if op.Object == "" || !exists(op.Object) {
				say("%s: names the object it acts on, and %q is not there", at, op.Object)
			}
		}
		switch op.Op {
		case api.OpIncrementCreate:
			// An increment is always created in its stream.
			if op.Stream == "" {
				say("%s: names the stream the increment is created in (stream)", at)
			} else if t, _, err := loadAny(ctx, cwm, op.Stream); err != nil || t.Type != api.StreamType {
				say("%s: %s, the stream it is created in, is not a stream", at, op.Stream)
			}
		case api.OpIncrementTarget:
			if op.Increment == "" || !exists(op.Increment) {
				say("%s: names the increment it aims at", at)
			}
		case api.OpGoalAdd, api.OpGoalChange, api.OpFeatureAdd, api.OpFeatureChange:
			if op.Increment != "" && !isRef(op.Increment) {
				if t, _, err := loadAny(ctx, cwm, op.Increment); err != nil || t.Type != api.IncrementType {
					say("%s: %s, the increment it aims what it makes at, is not an increment", at, op.Increment)
				}
			}
		case api.OpRequestMerge, api.OpRequestSupersede:
			if op.Into == "" || !exists(op.Into) {
				say("%s: names the request it goes into", at)
			}
		}
	}
	switch len(problems) {
	case 0:
		return nil
	case 1:
		return invalid("%s", problems[0])
	}
	return invalid("the delta has %d problems:\n- %s", len(problems), strings.Join(problems, "\n- "))
}

// isRef is a field naming an earlier operation of the same delta (#N).
func isRef(s string) bool { return strings.HasPrefix(s, api.DeltaRef) }

// refOps are the operations a reference in each field may name: what the
// named operation makes must be what the field wants. An object is named by
// reference only where the operation acts on what a delta makes.
var refOps = map[string][]string{
	"under":     {api.OpGoalAdd, api.OpFeatureAdd},
	"increment": {api.OpIncrementCreate},
}

// objectRefOps are, by operation, the operations its object may name.
var objectRefOps = map[string][]string{
	api.OpIncrementTarget: {api.OpGoalAdd, api.OpFeatureAdd},
	api.OpIncrementDone:   {api.OpIncrementCreate},
	api.OpGoalChange:      {api.OpGoalAdd},
	api.OpFeatureChange:   {api.OpFeatureAdd},
}

// checkRefs refuses a reference (#N) of operation i that names no earlier
// operation, or one that makes the wrong kind of object.
func checkRefs(d []api.DeltaOp, i int, at string) error {
	op := d[i]
	for _, f := range []struct{ name, value string }{{"under", op.Under}, {"object", op.Object}, {"increment", op.Increment}} {
		if !isRef(f.value) {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(f.value, api.DeltaRef))
		if err != nil || n < 0 || n >= i {
			return invalid("%s: %s %q names no earlier operation (#0 to #%d)", at, f.name, f.value, i-1)
		}
		allowed := refOps[f.name]
		if f.name == "object" {
			allowed = objectRefOps[op.Op]
		}
		if len(allowed) == 0 {
			return invalid("%s: its %s names an existing object, never an operation (%q)", at, f.name, f.value)
		}
		if !slices.Contains(allowed, d[n].Op) {
			return invalid("%s: %s %q is a %s, which makes no %s (%s)", at, f.name, f.value, d[n].Op, f.name, strings.Join(allowed, " or "))
		}
	}
	return nil
}

// resolveRef is the id a field names: an earlier operation's (#N) as applied,
// or the field itself.
func resolveRef(s string, applied []api.Applied) string {
	if !isRef(s) {
		return s
	}
	n, err := strconv.Atoi(strings.TrimPrefix(s, api.DeltaRef))
	if err != nil {
		return s
	}
	for _, x := range applied {
		if x.Op == n {
			return x.ID
		}
	}
	return s
}

// aimsOf is a goal or a feature operation: what it adds or changes can be
// aimed at an increment.
func aimsOf(op api.DeltaOp) bool {
	switch op.Op {
	case api.OpGoalAdd, api.OpGoalChange, api.OpFeatureAdd, api.OpFeatureChange:
		return true
	}
	return false
}

// withAboutIncrement gives the goal and feature operations of a demand about
// an increment that increment, where they name none: what the demand makes
// lands in the increment it is about.
func withAboutIncrement(ctx context.Context, cwm cwmapi.CWMService, request string, d []api.DeltaOp) []api.DeltaOp {
	o, err := load(ctx, cwm, request)
	if err != nil {
		return d
	}
	about := edgeTo(o, api.EdgeAbout)
	if about == "" {
		return d
	}
	if t, _, err := loadAny(ctx, cwm, about); err != nil || t.Type != api.IncrementType {
		return d
	}
	for i := range d {
		if aimsOf(d[i]) && d[i].Increment == "" {
			d[i].Increment = about
		}
	}
	return d
}

// keepDelta records a checked delta on its demand.
func keepDelta(ctx context.Context, request string, d []api.DeltaOp) error {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	o, err := load(ctx, cwm, request)
	if err != nil {
		return err
	}
	f := fieldsOf(o)
	f.Delta, f.Applied = mergeDelta(f.Delta, f.Applied, d)
	_, err = saveRequest(ctx, o, f)
	return err
}

// mergeDelta is a demand's delta after a reconcile answered d: the operations
// already applied stay, and so do those for the delivery goals d does not
// answer (a follow-up reconciles only the goals that changed); the others -
// what d answers again - give way to d, appended, its references (#N) moved
// past what was kept. A first reconcile, with nothing kept, is d itself.
func mergeDelta(old []api.DeltaOp, applied []api.Applied, d []api.DeltaOp) ([]api.DeltaOp, []api.Applied) {
	done := map[int]bool{}
	for _, x := range applied {
		done[x.Op] = true
	}
	answers := map[string]bool{}
	for _, op := range d {
		answers[op.For] = true
	}
	var kept []api.DeltaOp
	var keptApplied []api.Applied
	moved := map[int]int{}
	for i, op := range old {
		if !done[i] && answers[op.For] {
			continue
		}
		moved[i] = len(kept)
		kept = append(kept, op)
	}
	for _, x := range applied {
		if n, ok := moved[x.Op]; ok {
			keptApplied = append(keptApplied, api.Applied{Op: n, ID: x.ID})
		}
	}
	base := len(kept)
	shift := func(s string) string {
		if n, err := strconv.Atoi(strings.TrimPrefix(s, api.DeltaRef)); isRef(s) && err == nil {
			return api.DeltaRef + strconv.Itoa(n+base)
		}
		return s
	}
	for _, op := range d {
		op.Under, op.Object, op.Increment = shift(op.Under), shift(op.Object), shift(op.Increment)
		kept = append(kept, op)
	}
	return kept, keptApplied
}

// pendingOps is how many operations of a delta wait to be applied.
func pendingOps(d []api.DeltaOp, applied []api.Applied) int {
	done := map[int]bool{}
	for _, x := range applied {
		done[x.Op] = true
	}
	n := 0
	for i := range d {
		if !done[i] {
			n++
		}
	}
	return n
}

// checkApplied refuses an apply that left an operation unapplied or answered
// an object that is not there.
func checkApplied(ctx context.Context, cwm cwmapi.CWMService, d []api.DeltaOp, a []api.Applied) error {
	done := map[int]bool{}
	for _, x := range a {
		if x.Op < 0 || x.Op >= len(d) {
			return invalid("applied: operation %d is not in the delta (0 to %d)", x.Op, len(d)-1)
		}
		if _, err := loadObject(ctx, cwm, x.ID); err != nil {
			return invalid("applied: %s (operation %d) is not there", x.ID, x.Op)
		}
		done[x.Op] = true
	}
	var missing []string
	for i, op := range d {
		if !done[i] {
			missing = append(missing, fmt.Sprintf("%d (%s)", i, op.Op))
		}
	}
	if len(missing) > 0 {
		return invalid("applied (more.applied) does not cover operation(s) %s: name the object each changed or created", strings.Join(missing, ", "))
	}
	return nil
}

// realize links a demand to what its delta changed or created and tells a
// person requester what it became.
func realize(ctx context.Context, cwm cwmapi.CWMService, request string, a []api.Applied) error {
	var ids []string
	for _, x := range a {
		if !slices.Contains(ids, x.ID) && x.ID != request {
			ids = append(ids, x.ID)
		}
	}
	for _, id := range ids {
		if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: request, To: id, Kind: cwmapi.EdgeType(api.EdgeRealizedBy)}); err != nil {
			return fmt.Errorf("realized-by %s: %w", id, err)
		}
	}
	o, err := load(ctx, cwm, request)
	if err != nil {
		return err
	}
	f := fieldsOf(o)
	if f.Requester.Kind != "person" {
		return nil
	}
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil || inbox == nil {
		return nil
	}
	if _, err := inbox.File(ctx, inboxapi.FileInput{Title: fmt.Sprintf("Your demand %s became: %s", request, strings.Join(ids, ", ")),
		Description: fmt.Sprintf("«%s» was reconciled and applied: %s.", titleOf(o), strings.Join(ids, ", ")),
		RequestedBy: "planification:" + request, About: request, Expects: inboxapi.ExpectsNotice}); err != nil {
		slog.Warn("planification: a demand's requester was not told what it became", "request", request, "error", err)
	}
	return nil
}
