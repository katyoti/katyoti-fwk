// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// ONLY WHAT IS THE PERSON'S WAITS ON THEM - a realized
// feature whose acceptance criteria all pass and which no by-hand deliverable
// built is verified without the person: planification moves it done by its
// `verify` transition (the gate ready-to-test still decides), the passing
// criteria named in the move's note. It is tried when a build records the
// feature realized and when a verdict of one of its criteria is recorded. A
// feature with a by-hand deliverable, a failing or an unproven criterion -
// or none - stays realized: « À faire » asks the person to confirm only
// those.

// verifyBlocker says why a realized feature's verification is the person's;
// "" when an agent may verify it. passed are its criteria, all passing.
func verifyBlocker(ctx context.Context, cwm cwmapi.CWMService, f cwmapi.Object) (why string, passed []string) {
	crits := criteriaServing(ctx, cwm, f.ID)
	if len(crits) == 0 {
		return "it has no acceptance criterion", nil
	}
	for _, c := range crits {
		if !verdictPassed(ctx, cwm, c) {
			return "its criterion " + c + " has not passed", nil
		}
	}
	for _, r := range buildersOf(ctx, cwm, f) {
		for _, d := range fieldsOf(r).Deliverables {
			if strings.TrimSpace(d.ByHand) != "" {
				return r.ID + " has a deliverable a person tests by hand: " + d.Title, nil
			}
		}
	}
	return "", crits
}

// criteriaServing is a want's acceptance criteria: its children of the
// criterion type and the criteria serving it, each once, in id order.
func criteriaServing(ctx context.Context, cwm cwmapi.CWMService, id string) []string {
	var out []string
	if serving, err := incoming(ctx, cwm, id, api.EdgeServes); err == nil {
		for _, s := range serving {
			if c, err := loadObject(ctx, cwm, s); err == nil && c.Type == typeCriterion {
				out = append(out, c.ID)
			}
		}
	}
	module, _ := cwmapi.ModuleOfObjectID(id)
	if list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: module, Type: typeCriterion}); err == nil {
		for _, c := range list.Objects {
			if c.Parent == id && !slices.Contains(out, c.ID) {
				out = append(out, c.ID)
			}
		}
	}
	slices.Sort(out)
	return out
}

// buildersOf is the requests that built a want: the want itself when it
// carries one, and those serving it - a dropped one built nothing.
func buildersOf(ctx context.Context, cwm cwmapi.CWMService, f cwmapi.Object) []cwmapi.Object {
	var out []cwmapi.Object
	if hasRequest(f) {
		out = append(out, f)
	}
	serving, _ := incoming(ctx, cwm, f.ID, api.EdgeServes)
	for _, s := range serving {
		if r, err := loadObject(ctx, cwm, s); err == nil && hasRequest(r) && planState(r) != api.PlanDropped {
			out = append(out, r)
		}
	}
	return out
}

// verifyRealized moves a realized feature done when an agent may verify it;
// it answers whether it moved. A refused move (the gate) leaves it realized:
// the refusal plans what meets the gate.
func verifyRealized(ctx context.Context, cwm cwmapi.CWMService, id string) bool {
	f, err := loadObject(ctx, cwm, id)
	if err != nil || f.Type != featureType || ownState(f) != featureRealized {
		return false
	}
	why, passed := verifyBlocker(ctx, cwm, f)
	if why != "" {
		return false
	}
	note := "verified without the person: its acceptance criteria pass (" + strings.Join(passed, ", ") + ") and no deliverable of its build is tested by hand"
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: api.StateDone, Transition: "verify", By: PlanningActor, Note: note}); err != nil {
		slog.Info("planification: a realized feature whose criteria pass was not verified", "feature", id, "error", err)
		return false
	}
	return true
}

// VerdictRecorded verifies the realized features a criterion's new verdict
// may complete: the criterion's parent and the wants it serves.
func VerdictRecorded(ctx context.Context, verdict string) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return
	}
	v, err := loadObject(ctx, cwm, verdict)
	if err != nil || v.Type != typeVerdict || v.Parent == "" {
		return
	}
	c, err := loadObject(ctx, cwm, v.Parent)
	if err != nil || c.Type != typeCriterion {
		return
	}
	wants := edgesTo(c, api.EdgeServes)
	if c.Parent != "" && !slices.Contains(wants, c.Parent) {
		wants = append(wants, c.Parent)
	}
	for _, w := range wants {
		verifyRealized(ctx, cwm, w)
	}
}
