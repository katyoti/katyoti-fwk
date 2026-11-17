// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/projectcmd"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	manifestapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// VERIFYING A REQUEST'S DELIVERABLES — what "done" means for work that
// promised something: each deliverable's check passes. At its recipe's end
// the planner runs the checks not yet verified — an action URI through the
// action service, else a command in the project the universe serves — and
// records each one that passes. One that fails keeps the request from being
// done, and says why.

// checkTimeout bounds one deliverable's check that declares no bound of its
// own (its `timeout`, read by projectcmd.ParseTimeout).
var checkTimeout = 10 * time.Minute // a var: a test shortens it

// verifyDeliverables runs o's unverified deliverable checks, records those
// that pass, and answers o as saved; an error names the first that failed,
// else the first live walk left to the GUI batch (walkNotRun, never run).
func verifyDeliverables(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) (cwmapi.Object, error) {
	if !hasRequest(o) {
		return o, nil
	}
	f := fieldsOf(o)
	var failed, batch error
	changed := false
	for i, d := range f.Deliverables {
		if d.Verified || strings.TrimSpace(d.Check) == "" {
			continue
		}
		if c := verdictFor(ctx, cwm, d, f.Deliverables, f.FiledAt); c != "" {
			verifyByVerdict(f.Deliverables, i, c)
			changed = true
			continue
		}
		if why := walkNotRun(d, f.Deliverables); why != "" {
			if batch == nil {
				batch = fmt.Errorf("deliverable %q: %s", d.Title, why)
			}
			continue
		}
		if out, err := runCheck(ctx, d.Check, d.Timeout); err != nil {
			failed = fmt.Errorf("deliverable %q: %s failed: %v%s", d.Title, d.Check, err, tail(out))
			break
		}
		f.Deliverables[i].Verified, f.Deliverables[i].VerifiedAt = true, time.Now().UTC().Format(time.RFC3339)
		changed = true
	}
	if changed {
		if err := o.SetCapability(api.CapRequest, f); err != nil {
			return o, err
		}
		module, _ := cwmapi.ModuleOfObjectID(o.ID)
		out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
		if err != nil {
			return o, err
		}
		o = out.Object
	}
	if failed == nil {
		failed = batch // the end waits on the GUI batch
	}
	return o, failed
}

// verifiedBy is what a step verifies when a person approves it: a step that
// verifies (RecipeStep.Verifies), on a request with deliverables no check
// verifies — every one of them (api.VerifiesUnchecked), however many; ""
// when there is none.
func verifiedBy(st api.RecipeStep, ds []api.Deliverable) string {
	if !st.Verifies || len(approvedBy(api.VerifiesUnchecked, ds)) == 0 {
		return ""
	}
	return api.VerifiesUnchecked
}

// approvedBy is the deliverables a step's approval verifies: the one it names,
// or (api.VerifiesUnchecked) every one not verified that no check verifies.
func approvedBy(verifies string, ds []api.Deliverable) []string {
	if verifies != api.VerifiesUnchecked {
		if verifies == "" {
			return nil
		}
		return []string{verifies}
	}
	var titles []string
	for _, d := range ds {
		// One a person tests by hand (by_hand) is its own test, never an approval's.
		if !d.Verified && strings.TrimSpace(d.Check) == "" && strings.TrimSpace(d.ByHand) == "" {
			titles = append(titles, d.Title)
		}
	}
	return titles
}

// verifyByApproval records the deliverable a person's step verifies
// (WorkPacket.Verifies) verified by the person who approved it — when, who,
// and the approval as evidence — and answers the request as saved. A
// deliverable already verified, or gone, is left as it is.
func verifyByApproval(ctx context.Context, o cwmapi.Object, wt api.WorkTodo) (cwmapi.Object, error) {
	r := wt.Result
	f := fieldsOf(o)
	titles := approvedBy(wt.Packet.Verifies, f.Deliverables)
	changed := false
	for i, d := range f.Deliverables {
		if !slices.Contains(titles, d.Title) || d.Verified {
			continue
		}
		at := r.At
		if at == "" {
			at = time.Now().UTC().Format(time.RFC3339)
		}
		by := r.By
		f.Deliverables[i].Verified, f.Deliverables[i].VerifiedAt = true, at
		f.Deliverables[i].VerifiedBy, f.Deliverables[i].Evidence = &by, approvalEvidence(wt.Packet.Step, r.Summary, wt.Packet.AskedIn)
		changed = true
	}
	if !changed {
		return o, nil
	}
	return saveRequest(ctx, o, f)
}

// approvalPrefix opens the evidence of a deliverable a step's approval
// verified: approvalEvidence writes it, approvedStep reads the step back.
const approvalPrefix = "approved step "

// approvalEvidence is what verifies a deliverable a person approved at a step:
// "approved step <step>: <summary> (asked in <inbox entry>)".
func approvalEvidence(step, summary, askedIn string) string {
	e := approvalPrefix + step + ": " + firstLine(summary)
	if askedIn != "" {
		e += " (asked in " + askedIn + ")"
	}
	return e
}

// approvedStep is the step whose approval verified a deliverable, read from
// its evidence; "" when a person verified it by hand.
func approvedStep(evidence string) string {
	rest, ok := strings.CutPrefix(evidence, approvalPrefix)
	if !ok {
		return ""
	}
	step, _, _ := strings.Cut(rest, ": ")
	return step
}

// checkRunnable refuses a check no command can run: an action URI, or a
// command whose first word is a program, a project script or a shell word.
// Empty is no check: a person verifies it by hand.
func checkRunnable(check string) error {
	check = strings.TrimSpace(check)
	if check == "" {
		return nil
	}
	if !strings.ContainsAny(check, " \t") {
		if _, err := manifestapi.ParseToolURI(check); err == nil {
			return nil
		}
	}
	if err := projectcmd.Runnable(check); err != nil {
		return fmt.Errorf("not a runnable check (%v): a person verifies it by hand - leave the check empty", err)
	}
	return nil
}

// runCheck runs one check: an action URI (one word that parses as one) through
// the action service — a method, or a script (its exit code counts); otherwise
// a shell command in the project root, bounded by the timeout it declares, else
// by checkTimeout.
func runCheck(ctx context.Context, check, timeout string) (string, error) {
	check = strings.TrimSpace(check)
	if err := checkRunnable(check); err != nil {
		return "", err // never run: a document opened by the shell proves nothing
	}
	bound, ok, err := projectcmd.ParseTimeout(timeout)
	if err != nil {
		return "", err // never run on a bound nobody declared
	}
	if !ok {
		bound = checkTimeout
	}
	if !strings.ContainsAny(check, " \t") {
		if _, err := manifestapi.ParseToolURI(check); err == nil {
			actions, err := application.UseService[actionsapi.ActionService](ctx)
			if err != nil {
				return "", fmt.Errorf("no action service: %w", err)
			}
			res, err := actions.Run(ctx, actionsapi.RunInput{URI: check, Input: json.RawMessage(`{}`)})
			if err != nil {
				return "", err
			}
			if res.TaskID != "" {
				// A task runs apart: submitting it proves nothing.
				return "", fmt.Errorf("a task is not a check (it runs apart, task %s) — name a method or a script", res.TaskID)
			}
			var script struct {
				ExitCode *int `json:"exit_code"`
			}
			if json.Unmarshal(res.Value, &script) == nil && script.ExitCode != nil && *script.ExitCode != 0 {
				return string(res.Value), fmt.Errorf("exit status %d", *script.ExitCode)
			}
			return string(res.Value), nil
		}
	}
	out, err := projectcmd.Run(ctx, check, bound)
	if err == nil {
		if why := projectcmd.RanNoTest(check, out); why != "" {
			return out, fmt.Errorf("%s", why)
		}
	}
	return out, err
}

// tail is the end of a check's output, for the note that says why it failed.
func tail(out string) string {
	if t := projectcmd.Tail(out, 600); t != "" {
		return ": " + t
	}
	return ""
}
