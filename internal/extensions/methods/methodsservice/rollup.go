// SPDX-License-Identifier: Apache-2.0

package methodsservice

import "context"

// Rollup is the METHOD's grading vocabulary — deliberately NOT in the substrate.
//
// CWM walks the graph and runs an object's checks (it owns the graph and the
// lock); what the results MEAN is the method's business. This verb takes the raw
// per-check outcomes as INPUT and returns the grade — stateless, data-in /
// verdict-out, exactly like VerifyAcceptance. It touches no CWM.
//
// The rule it encodes (modules/method/concepts.md §3): a composite is green
// ONLY when every check is a pass. Anything unproven blocks green, so "no
// criteria" and "not yet probed" can never masquerade as success.

// CheckStatus is one raw check outcome, as produced by a run.
type CheckStatus struct {
	ID     string `json:"id"`
	Status string `json:"status"` // pass | fail | no-probe | harness-error
}

// RollupInput carries the raw outcomes to grade.
type RollupInput struct {
	Checks []CheckStatus `json:"checks"`
}

// RollupOutput is the grade derived from those outcomes.
//
//	green    — every check passed (and there is at least one)
//	unmet    — at least one check did not pass
//	ungraded — there were no checks at all (never green by omission)
type RollupOutput struct {
	Status   string   `json:"status"`   // green | unmet | ungraded
	Total    int      `json:"total"`    // number of checks graded
	Passed   int      `json:"passed"`   // number that passed
	Progress float64  `json:"progress"` // passed / total (0 when total == 0)
	Unmet    []string `json:"unmet"`    // the blocking frontier, by check id
}

// GRADING A MILESTONE is the milestones service's Grade, built on this one.
//
// A milestone has no state of its own — its grade IS the rollup over what
// judges it (its `judged-by` edges), else over what it includes (`includes`).
// Grade reads those edges, verifies each member and feeds the outcomes here;
// Rollup itself touches no CWM.
//
// ONE RULE THE CALLER MUST HONOUR: a member that resolves to NO object is passed
// as a non-pass outcome (status "missing"), never omitted. Omitting it shrinks
// the denominator, and a rollup over 4 of 5 members reads greener than the
// truth. TestRollup_MissingMemberBlocksGreen pins this.

// Rollup grades a set of raw check outcomes.
func (s *service) Rollup(_ context.Context, in RollupInput) (RollupOutput, error) {
	out := RollupOutput{Status: "ungraded", Unmet: []string{}}
	for _, c := range in.Checks {
		out.Total++
		if c.Status == "pass" {
			out.Passed++
			continue
		}
		// fail, no-probe, harness-error — all block green.
		out.Unmet = append(out.Unmet, c.ID)
	}
	if out.Total > 0 {
		out.Progress = float64(out.Passed) / float64(out.Total)
		if out.Passed == out.Total {
			out.Status = "green"
		} else {
			out.Status = "unmet"
		}
	}
	return out, nil
}
