// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// CORRECTING WHAT A REQUEST PROMISED — a request's deliverables are set when it
// is filed, and a check can be wrong from the start or broken by a later change
// (a flag renamed). Revise corrects them while the request is open, why
// recorded; Check runs the checks now instead of waiting for the recipe's end;
// Verify records one verified by hand — by a person, with what they saw — for a
// check no command can run. Done still needs every deliverable verified.

// Revise sets an open request's deliverables to the list given. A deliverable
// whose title and check are unchanged keeps its verification; any other must
// be verified again. Why it changed is added to the request's details.
func (s *Service) Revise(ctx context.Context, in api.ReviseInput) (api.RequestOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.RequestOutput{}, err
	}
	if strings.TrimSpace(in.Note) == "" {
		return api.RequestOutput{}, invalid("Revise: say why the deliverables change (note)")
	}
	next := make([]api.Deliverable, len(in.Deliverables))
	for i, d := range in.Deliverables {
		next[i] = api.Deliverable{Title: strings.TrimSpace(d.Title), Check: strings.TrimSpace(d.Check), Timeout: strings.TrimSpace(d.Timeout), ByHand: strings.TrimSpace(d.ByHand)}
	}
	o, f, err := openRequest(ctx, "Revise", in.ID)
	if err != nil {
		return api.RequestOutput{}, err
	}
	if in.Deliverables == nil {
		// No list: its note is recorded, its deliverables kept as they are —
		// verified ones included, never checked again as if filed anew, never
		// erased by a revise that only meant to say something.
		next = f.Deliverables
	} else if err := checkAsk("Revise", "", "", next, f.Deliverables); err != nil {
		return api.RequestOutput{}, err
	}
	for i, d := range next {
		for _, was := range f.Deliverables {
			if was.Title == d.Title && was.Check == d.Check {
				// Its verification is kept; how a person tests it and how long
				// its check may run are the revise's (an auto-test marks an
				// untested one by hand; a slow check declares its bound).
				kept := was
				kept.ByHand, kept.Timeout = d.ByHand, d.Timeout
				next[i] = kept
				break
			}
		}
	}
	f.Deliverables = next
	f.Details = strings.TrimRight(f.Details, "\n") + fmt.Sprintf("\n\n_Deliverables revised by %s on %s: %s_", in.By.ID, time.Now().UTC().Format("2006-01-02"), strings.TrimSpace(in.Note))
	out, err := saveRequest(ctx, o, f)
	if err != nil {
		return api.RequestOutput{}, err
	}
	return api.RequestOutput{Request: toRequest(out)}, nil
}

// Check runs an open request's unverified deliverable checks now — every one,
// a failure does not stop the others — and records each that passes. A live
// GUI walk is never run here (walkNotRun): it counts by its criterion's
// recorded verdict, the batch script (scripts/gui-batch.sh) running it.
func (s *Service) Check(ctx context.Context, in api.CheckInput) (api.CheckOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.CheckOutput{}, err
	}
	o, f, err := openRequest(ctx, "Check", in.ID)
	if err != nil {
		return api.CheckOutput{}, err
	}
	var results []api.CheckResult
	changed := false
	approves := map[string]string{} // the deliverables a person's step verifies
	if todos, err := todosOf(ctx, o.ID); err == nil {
		for _, t := range todos {
			for _, title := range approvedBy(t.Packet.Verifies, f.Deliverables) {
				approves[title] = t.Packet.Step
			}
		}
	}
	cwm, _ := cwmOf(ctx)
	for i, d := range f.Deliverables {
		r := api.CheckResult{Title: d.Title, Check: d.Check}
		var byVerdict string
		if !d.Verified && strings.TrimSpace(d.Check) != "" {
			byVerdict = verdictFor(ctx, cwm, d, f.Deliverables, f.FiledAt)
		}
		switch {
		case d.Verified:
			r.Passed = true
		case byVerdict != "":
			r.Passed = true
			verifyByVerdict(f.Deliverables, i, byVerdict)
			changed = true
		case strings.TrimSpace(d.Check) == "" && approves[d.Title] != "":
			r.Error = "verified when a person approves step " + approves[d.Title]
		case strings.TrimSpace(d.Check) == "":
			r.Error = "no check: a person verifies it by hand (Verify)"
		case walkNotRun(d, f.Deliverables) != "":
			r.Error = walkNotRun(d, f.Deliverables)
			if crit := failedVerdictFor(ctx, cwm, d, f.FiledAt); crit != "" && allLiveTests(f.Deliverables) {
				// A release's GUI batch whose script recorded the walk failed:
				// the failure files its fixes (RQ-1153).
				r.Error = "the recorded verdict of " + crit + " failed; " + r.Error
				if ferr := s.fileGUIFailure(ctx, cwm, o.ID, liveScript(d.Check), "the recorded verdict of "+crit+" failed"); ferr != nil {
					r.Error += " (filing its fixes: " + ferr.Error() + ")"
				}
			}
		default:
			if out, err := runCheck(ctx, d.Check, d.Timeout); err != nil {
				r.Error = fmt.Sprintf("%v%s", err, tail(out))
			} else {
				r.Passed = true
				f.Deliverables[i].Verified, f.Deliverables[i].VerifiedAt = true, time.Now().UTC().Format(time.RFC3339)
				changed = true
			}
		}
		results = append(results, r)
	}
	if changed {
		if o, err = saveRequest(ctx, o, f); err != nil {
			return api.CheckOutput{}, err
		}
	}
	// Its end is tried even when nothing new passed: a request refused its end
	// may have had every deliverable verified since.
	if o, err = endIfDone(ctx, o, f, in.By); err != nil {
		return api.CheckOutput{}, err
	}
	return api.CheckOutput{Request: toRequest(o), Results: results}, nil
}

// endIfDone ends a request whose to-dos were all collected once every
// deliverable is verified: its end was refused for an unverified one, and
// nothing else would try it again.
func endIfDone(ctx context.Context, o cwmapi.Object, f api.RequestFields, by cwmapi.Actor) (cwmapi.Object, error) {
	for _, d := range f.Deliverables {
		if !d.Verified {
			return o, nil
		}
	}
	open, err := todosOf(ctx, o.ID)
	if err != nil || len(open) > 0 {
		return o, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return o, err
	}
	if err := endRequest(ctx, cwm, o.ID, by); err != nil {
		return o, err
	}
	return load(ctx, cwm, o.ID)
}

// Verify records one deliverable verified by hand: by a person only — an agent
// does not answer a person's check on their behalf, and a call from an agent's
// session is refused whatever by says — with what they saw.
func (s *Service) Verify(ctx context.Context, in api.VerifyInput) (api.RequestOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.RequestOutput{}, err
	}
	if in.By.Kind != "person" {
		return api.RequestOutput{}, invalid("Verify: a deliverable is verified by hand by a person, not by %s %s — run its check instead (Check)", in.By.Kind, in.By.ID)
	}
	relayed, err := personsAct(ctx, in.By, "verify:"+in.ID+"#"+in.Deliverable, "Verify: a person verifies by hand themselves (the request view, or their own command)")
	if err != nil {
		return api.RequestOutput{}, err
	}
	if strings.TrimSpace(in.Evidence) == "" {
		return api.RequestOutput{}, invalid("Verify: say what was seen that verifies it (evidence)")
	}
	o, f, err := openRequest(ctx, "Verify", in.ID)
	if err != nil {
		return api.RequestOutput{}, err
	}
	found := false
	for i, d := range f.Deliverables {
		if d.Title != in.Deliverable {
			continue
		}
		by := in.By
		f.Deliverables[i].Verified, f.Deliverables[i].VerifiedAt = true, time.Now().UTC().Format(time.RFC3339)
		f.Deliverables[i].VerifiedBy, f.Deliverables[i].Evidence = &by, strings.TrimSpace(in.Evidence)
		f.Deliverables[i].Relayed = relayed
		found = true
		break
	}
	if !found {
		return api.RequestOutput{}, invalid("Verify: %s has no deliverable %q", in.ID, in.Deliverable)
	}
	out, err := saveRequest(ctx, o, f)
	if err != nil {
		return api.RequestOutput{}, err
	}
	if out, err = endIfDone(ctx, out, f, in.By); err != nil {
		return api.RequestOutput{}, err
	}
	return api.RequestOutput{Request: toRequest(out)}, nil
}

// openRequest loads a request that has not ended, and its fields.
func openRequest(ctx context.Context, verb, id string) (cwmapi.Object, api.RequestFields, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return cwmapi.Object{}, api.RequestFields{}, err
	}
	o, err := load(ctx, cwm, id)
	if err != nil {
		return cwmapi.Object{}, api.RequestFields{}, err
	}
	if !hasRequest(o) {
		return cwmapi.Object{}, api.RequestFields{}, invalid("%s: %s carries no request", verb, id)
	}
	if nodeEnded(o) {
		return cwmapi.Object{}, api.RequestFields{}, invalid("%s: %s has ended (%s)", verb, id, stateOf(o))
	}
	return o, fieldsOf(o), nil
}

// saveRequest writes a request's fields.
func saveRequest(ctx context.Context, o cwmapi.Object, f api.RequestFields) (cwmapi.Object, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return cwmapi.Object{}, err
	}
	if err := o.SetCapability(api.CapRequest, f); err != nil {
		return cwmapi.Object{}, err
	}
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	if err != nil {
		return cwmapi.Object{}, err
	}
	return out.Object, nil
}
