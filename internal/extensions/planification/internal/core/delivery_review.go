// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN INCREMENT IS DELIVERED ONLY WHEN THE USER CAN USE WHAT IT DELIVERS
// : its delivery approval
// (increment-accept) starts with a delivery review - an agent's, a person's
// where asked - before the person's approve-delivery. The review is recorded
// on the increment (delivery_review), and the gate usable-delivered on the
// increment's done accepts it only while that review passed and its checks
// still hold now - a stale review is never trusted:
//   - every goal and feature it holds is done;
//   - every screen feature it holds has passed its GUI test (gui-tested);
//   - every display switch it holds is retired (the review says so) or
//     turned on by an increment it names, shipping in the same release (a
//     test delivery: increments set-test-delivery);
//   - the review wrote a demo path;
//   - every finding has its follow-up request, in an increment of the same
//     release;
//   - nothing its open work waits on (depends_on) is dropped or ships in a
//     later release (dependencyChecks).
// A person forces the gate with a note (increments done --force).

// The delivery review's step, the key its report carries the review under,
// and the gate on an increment's done.
const (
	StepDeliveryReview  = "delivery-review"
	deliveryReviewKey   = "review"
	GateUsableDelivered = "usable-delivered"
)

// reviewReport is what a delivery review reports under more.review.
type reviewReport struct {
	Demo            string                `json:"demo"`
	Findings        []api.DeliveryFinding `json:"findings"`
	SwitchesRetired []string              `json:"switches_retired"`
}

// deliveryChecks says what keeps an increment from being usable-delivered,
// read with the review given (nil: only what needs none); empty when nothing
// does.
func deliveryChecks(ctx context.Context, cwm cwmapi.CWMService, inc cwmapi.Object, rv *api.DeliveryReview) []string {
	why, _ := heldBack(ctx, cwm, inc)
	rel := releaseOfIncrement(ctx, cwm, inc.ID)
	f := releaseFields(inc)
	why = append(why, dependencyChecks(ctx, cwm, inc, rel)...)
	scope, _ := scopeOf(ctx, cwm, inc)
	for _, t := range scope.Targets {
		if uncounted(t.State) {
			continue
		}
		o, err := loadObject(ctx, cwm, t.ID)
		if err != nil || o.Type != featureType {
			continue
		}
		if _, sw := o.Capabilities[api.CapDisplaySwitch]; sw {
			if rv != nil && slices.Contains(rv.SwitchesRetired, o.ID) {
				continue
			}
			if f.TestDelivery == nil || f.TestDelivery.TurnedOnBy == "" {
				why = append(why, fmt.Sprintf("the display switch %s is left with no follow-up: retire it (the review names it in switches_retired) or name the increment of the same release that turns it on (increments set-test-delivery)", o.ID))
			} else if w := turnedOnWhy(ctx, cwm, inc.ID, f.TestDelivery.TurnedOnBy, rel); w != "" {
				why = append(why, fmt.Sprintf("the display switch %s: %s", o.ID, w))
			}
			continue
		}
		if drawsAScreen(ctx, cwm, o) {
			if out, err := NewPlanning().GuiTested(ctx, api.HookInput{Object: &o}); err == nil && !out.Accept {
				why = append(why, out.Reason)
			}
		}
	}
	if rv == nil {
		return why
	}
	if strings.TrimSpace(rv.Demo) == "" {
		why = append(why, "no demo path: the review says how a user sees what "+inc.ID+" delivers")
	}
	for _, fd := range rv.Findings {
		if fd.FollowUp == "" {
			why = append(why, fmt.Sprintf("« %s » has no follow-up request", fd.What))
		} else if w := followUpWhy(ctx, cwm, fd.FollowUp, rel); w != "" {
			why = append(why, fmt.Sprintf("« %s »: %s", fd.What, w))
		}
	}
	return why
}

// dependencyChecks names what an increment's open work - itself, and the
// requests incrementWork adds (those serving a target, still open after it
// is done) - waits on and can never have in its release: a depends_on whose
// object is dropped, or held by an increment shipping in a release later
// than rel. Empty when nothing does.
func dependencyChecks(ctx context.Context, cwm cwmapi.CWMService, inc cwmapi.Object, rel string) []string {
	var why []string
	work := append([]cwmapi.Object{inc}, incrementWork(ctx, cwm, inc)...)
	seen := map[string]bool{}
	for _, w := range work {
		if seen[w.ID] {
			continue
		}
		seen[w.ID] = true
		if w.ID != inc.ID {
			if st := workStatus(ctx, cwm, w); st == "done" || st == "dropped" {
				continue
			}
		}
		for _, d := range edgesTo(w, api.EdgeDependsOn) {
			do, _, err := loadAny(ctx, cwm, d)
			if err != nil {
				continue
			}
			if droppedObject(do) {
				why = append(why, fmt.Sprintf("%s waits on %s, which is dropped", w.ID, d))
				continue
			}
			if rel == "" || isDone(ctx, cwm, do) {
				continue
			}
			h := holderIncrement(ctx, cwm, do)
			if h == nil || h.ID == inc.ID {
				continue
			}
			if other := releaseOfIncrement(ctx, cwm, h.ID); other != "" && other != rel && shipsLater(ctx, cwm, rel, other) {
				why = append(why, fmt.Sprintf("%s waits on %s, which ships in %s, later than %s", w.ID, d, other, rel))
			}
		}
	}
	return why
}

// droppedObject is whether a piece of work is dropped: an increment, an
// object with a lifecycle by that lifecycle - a feature's dropped round of
// work (its request's plan) never drops the feature - else a request's plan.
func droppedObject(o cwmapi.Object) bool {
	if o.Type == api.IncrementType {
		return releaseState(o) == api.IncrementDropped
	}
	if l, ok := o.Lifecycle(); ok {
		return l.State == api.StateDropped
	}
	return hasRequest(o) && planState(o) == api.PlanDropped
}

// shipsLater is whether release b is planned to ship after release a: by
// date, a dated one first, else the one made later.
func shipsLater(ctx context.Context, cwm cwmapi.CWMService, a, b string) bool {
	ao, err1 := loadObject(ctx, cwm, a)
	bo, err2 := loadObject(ctx, cwm, b)
	if err1 != nil || err2 != nil {
		return false
	}
	da, db := releaseFields(ao).Date, releaseFields(bo).Date
	switch {
	case da == db:
		return laterID(b, a)
	case db == "":
		return true
	case da == "":
		return false
	}
	return db > da
}

// turnedOnWhy says why the increment named does not turn an increment's
// switch on: it is not an increment, it is dropped, or it does not ship in
// the same release; "" when it does.
func turnedOnWhy(ctx context.Context, cwm cwmapi.CWMService, inc, by, rel string) string {
	if by == inc {
		return "an increment does not turn its own switch on"
	}
	o, err := loadObject(ctx, cwm, by)
	if err != nil || o.Type != api.IncrementType {
		return by + " is not an increment"
	}
	if releaseState(o) == api.IncrementDropped {
		return "the increment that turns it on, " + by + ", is dropped"
	}
	if rel == "" {
		return inc + " ships in no release: the increment that turns its switch on cannot be planned in the same release"
	}
	if other := releaseOfIncrement(ctx, cwm, by); other != rel {
		return fmt.Sprintf("the increment that turns it on, %s, is not planned in %s", by, rel)
	}
	return ""
}

// followUpWhy says why a follow-up request does not follow an increment up
// in its release; "" when it is a request of an increment of that release.
func followUpWhy(ctx context.Context, cwm cwmapi.CWMService, id, rel string) string {
	r, err := loadObject(ctx, cwm, id)
	if err != nil || !hasRequest(r) || r.Type == api.IncrementType {
		return "its follow-up " + id + " is not a request"
	}
	if planState(r) == api.PlanDropped {
		return "its follow-up " + id + " is dropped"
	}
	inc, _ := releaseOfWork(ctx, cwm, r)
	if inc == "" {
		return "its follow-up " + id + " is in no increment"
	}
	if rel == "" || releaseOfIncrement(ctx, cwm, inc) != rel {
		return fmt.Sprintf("its follow-up %s is in %s, not in an increment of the same release", id, inc)
	}
	return ""
}

// CheckUsableDelivered is the usable-delivered gate's requirement: accepted
// when the increment's delivery review passed and its checks hold now,
// refused naming what is missing. Read-only.
func (i *Increments) CheckUsableDelivered(ctx context.Context, in api.UsableDeliveredInput) (api.CheckReadyOutput, error) {
	id := in.ID
	if id == "" && in.Object != nil {
		id = in.Object.ID
	}
	if id == "" {
		return api.CheckReadyOutput{}, invalid("CheckUsableDelivered: id or object required")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.CheckReadyOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, id)
	if err != nil {
		return api.CheckReadyOutput{}, err
	}
	return usableDelivered(ctx, cwm, o), nil
}

// usableDelivered is the gate's answer on an increment.
func usableDelivered(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) api.CheckReadyOutput {
	rv := releaseFields(o).DeliveryReview
	var why []string
	switch {
	case rv == nil:
		why = append(why, "its delivery review has not run: it runs once everything "+o.ID+" holds is done, before the person approves its delivery")
	case rv.Verdict != api.VerdictApproved:
		why = append(why, "its delivery review did not pass")
	}
	why = append(why, deliveryChecks(ctx, cwm, o, rv)...)
	if len(why) > 0 {
		return api.CheckReadyOutput{Reason: strings.Join(why, "; ")}
	}
	return api.CheckReadyOutput{Accept: true}
}

// SetTestDelivery makes an increment a test delivery: the increment named,
// shipping in the same release and not dropped, turns its switch on; an
// empty one clears it.
func (i *Increments) SetTestDelivery(ctx context.Context, in api.SetTestDeliveryInput) (api.IncrementOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.IncrementOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	f := releaseFields(o)
	by := strings.TrimSpace(in.TurnedOnBy)
	if by == "" {
		f.TestDelivery = nil
	} else {
		if w := turnedOnWhy(ctx, cwm, o.ID, by, releaseOfIncrement(ctx, cwm, o.ID)); w != "" {
			return api.IncrementOutput{}, invalid("SetTestDelivery: %s", w)
		}
		f.TestDelivery = &api.TestDelivery{TurnedOnBy: by, By: in.By, At: time.Now().UTC().Format(time.RFC3339)}
	}
	if o, err = setFields(ctx, cwm, o, f); err != nil {
		return api.IncrementOutput{}, err
	}
	return api.IncrementOutput{Increment: toIncrement(ctx, o)}, nil
}

// reviewOf reads the delivery review a step reports: its more.review, the
// demo path falling back to a person's details.
func reviewOf(r *schedapi.Result, person bool, by cwmapi.Actor) api.DeliveryReview {
	var rr reviewReport
	fromMore(r, deliveryReviewKey, &rr)
	rv := api.DeliveryReview{Verdict: r.Verdict, Demo: strings.TrimSpace(rr.Demo), Findings: rr.Findings,
		SwitchesRetired: rr.SwitchesRetired, By: by, At: time.Now().UTC().Format(time.RFC3339)}
	if rv.Demo == "" && person {
		rv.Demo = strings.TrimSpace(r.Details)
	}
	return rv
}

// checkDeliveryReview refuses an approved delivery review whose checks do
// not hold: it goes back to the queue naming what is missing - the review
// rejects it, or the work is done first.
func checkDeliveryReview(ctx context.Context, cwm cwmapi.CWMService, wt api.WorkTodo, rv api.DeliveryReview) error {
	if rv.Verdict != api.VerdictApproved {
		return nil
	}
	o, err := loadIncrement(ctx, cwm, wt.Request)
	if err != nil {
		return err
	}
	if why := deliveryChecks(ctx, cwm, o, &rv); len(why) > 0 {
		return invalid("the delivery review approves %s, but it is not usable-delivered: %s - reject it naming each of them (more.review.findings), or report it once they are", o.ID, strings.Join(why, "; "))
	}
	return nil
}

// recordDeliveryReview writes the review on the increment.
func recordDeliveryReview(ctx context.Context, cwm cwmapi.CWMService, inc string, rv *api.DeliveryReview) error {
	o, err := loadIncrement(ctx, cwm, inc)
	if err != nil {
		return err
	}
	f := releaseFields(o)
	f.DeliveryReview = rv
	_, err = setFields(ctx, cwm, o, f)
	return err
}

// rejectDelivery files what a rejected delivery review found missing - its
// findings and every check that fails - as fixes inside the increment, and
// ends the approval: it is asked again once that work ends.
func rejectDelivery(ctx context.Context, cwm cwmapi.CWMService, inc cwmapi.Object, rv api.DeliveryReview, summary string, by cwmapi.Actor) error {
	// A finding with no follow-up is filed in its own words; the checks run
	// on the rest of the review (its demo path, the follow-ups it names).
	var missing []string
	checked := rv
	checked.Findings = nil
	for _, fd := range rv.Findings {
		if fd.FollowUp == "" {
			if w := strings.TrimSpace(fd.What); w != "" {
				missing = append(missing, w)
			}
			continue
		}
		checked.Findings = append(checked.Findings, fd)
	}
	for _, w := range deliveryChecks(ctx, cwm, inc, &checked) {
		if !slices.Contains(missing, w) {
			missing = append(missing, w)
		}
	}
	if len(missing) == 0 {
		missing = []string{firstLine(summary)}
	}
	module, _ := cwmapi.ModuleOfObjectID(inc.ID)
	for _, m := range missing {
		if m == "" {
			continue
		}
		if _, err := New().File(ctx, api.FileInput{
			Title: m, Details: "The delivery review of " + inc.ID + " found it missing:\n\n" + summary,
			Kind: api.KindFix, Module: module, Increment: inc.ID, By: by,
		}); err != nil {
			return fmt.Errorf("file what the delivery review of %s found missing: %w", inc.ID, err)
		}
	}
	_, err := NewPlans().Finish(ctx, api.FinishInput{ID: inc.ID, Outcome: api.PlanDropped, By: by,
		Note: fmt.Sprintf("its delivery review found %d thing(s) missing: asked again once they are done", len(missing))})
	return err
}

// reviewToApproval carries a passed review to the person's approve-delivery:
// its verdict and its demo path, in the to-do's params.
func reviewToApproval(x *api.WorkTodo, rv api.DeliveryReview) {
	x.Packet.Params = maps.Clone(x.Packet.Params)
	if x.Packet.Params == nil {
		x.Packet.Params = map[string]any{}
	}
	review := "the delivery review by " + rv.By.ID + " passed"
	if len(rv.Findings) > 0 {
		var follow []string
		for _, fd := range rv.Findings {
			follow = append(follow, fd.What+" ("+fd.FollowUp+")")
		}
		review += "; followed up in the same release: " + strings.Join(follow, ", ")
	}
	x.Packet.Params["review"] = review
	x.Packet.Params["demo"] = rv.Demo
}
