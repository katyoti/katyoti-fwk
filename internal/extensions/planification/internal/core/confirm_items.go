// SPDX-License-Identifier: Apache-2.0

package core

import (
	"cmp"
	"context"
	"errors"

	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// WANTS THE WORK RECORDED DONE — a feature realized, a goal assumed — wait on
// the person's confirmation: only a person moves them on (their workflow's
// confirm). « À faire » shows each as a validate item: what it is judged on
// (its acceptance criteria with their last result, the requests that built
// it, its increment) and two buttons — Accept runs confirm, Reject sends it
// back saying why.

// confirmable is, per type, the state a want waits in for the person and its
// two moves.
var confirmable = map[string]struct {
	state string
	move  api.ConfirmMove
}{
	"feature": {"realized", api.ConfirmMove{Accept: "confirm", AcceptTo: "done", Reject: "reopen", RejectTo: "proposed"}},
	"goal":    {"assumed", api.ConfirmMove{Accept: "confirm", AcceptTo: "active", Reject: "rethink", RejectTo: "idea"}},
}

// The criteria and their verdicts, as the method pack declares them.
const (
	typeCriterion   = "acceptance-criterion"
	typeVerdict     = "verdict"
	verdictUnproven = "unproven"
)

// confirmItems is one validate item per want waiting on the person's
// confirmation, the oldest id first.
func confirmItems(ctx context.Context, cwm cwmapi.CWMService) []api.PersonItem {
	var waiting []cwmapi.Object
	why := map[string]string{}
	for typ, c := range confirmable {
		list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: typ})
		if err != nil {
			continue
		}
		for i, o := range list.Objects {
			if ownState(o) != c.state {
				continue
			}
			// Several projects served: named in the project holding it
			//
			if len(list.Projects) == len(list.Objects) {
				o = inKey(o, list.Projects[i])
			}
			// A realized feature an agent may verify is not the person's
			// : only those whose verification needs
			// them wait here - and those the verify gate refuses an agent
			// (: its GUI test batched, gui-tested
			// unmet), else nobody is asked: the person's « deliver then
			// test », its GUI test kept in its batch.
			if o.Type == featureType {
				if b, _ := verifyBlocker(ctx, cwm, o); b == "" {
					if b = agentVerifyRefused(ctx, o.ID); b == "" {
						continue
					}
					why[o.ID] = b
				}
			}
			waiting = append(waiting, o)
		}
	}
	if len(waiting) == 0 {
		return nil
	}
	sort.Slice(waiting, func(i, j int) bool { return waiting[i].ID < waiting[j].ID })
	wants := make(map[string]bool, len(waiting))
	for _, o := range waiting {
		wants[o.ID] = true
	}
	criteria := criteriaByWant(ctx, cwm, wants)
	built := requestsByWant(ctx, cwm)
	out := make([]api.PersonItem, 0, len(waiting))
	for _, o := range waiting {
		c := confirmable[o.Type]
		move := c.move
		module, _ := cwmapi.ModuleOfObjectID(bareID(o.ID))
		it := api.PersonItem{Key: api.ActionValidate + ":" + o.ID, Action: api.ActionValidate, Object: o.ID, Module: module,
			Project: keyOf(o.ID), Title: o.DisplayTitle(), Criteria: criteria[o.ID], Confirm: &move,
			Asks: "Its work is recorded " + c.state + ": accept it, or reject it saying why."}
		if b := why[o.ID]; b != "" {
			it.Asks = "Its work is recorded " + c.state + " and an agent cannot verify it (" + b + "): accept it - what is left to test stays planned - or reject it saying why."
		}
		for _, r := range built[o.ID] {
			it.Refs = append(it.Refs, api.Ref{ID: r, Label: "Built by"})
		}
		out = append(out, it)
	}
	return out
}

// agentVerifyRefused is why an agent's verify of a realized feature is
// refused: the requirements its gate (ready-to-test) has unmet for the
// planner - a costly check is not run: decided at the move, it is not counted - empty
// when an agent may verify it.
func agentVerifyRefused(ctx context.Context, id string) string {
	gates, err := application.UseService[gatesapi.GatesService](ctx)
	if err != nil {
		return ""
	}
	by := PlanningActor
	chk, err := gates.Check(ctx, gatesapi.CheckInput{ID: id, Gate: api.GateReadyToTest, By: &by,
		From: featureRealized, To: api.StateDone, NoCostly: true})
	if err != nil {
		return ""
	}
	var unmet []string
	for _, it := range chk.Checklist.Unmet() {
		if cwmapi.IsDecidedAtMove(errors.New(it.Reason)) {
			continue // a costly check, decided at the move
		}
		unmet = append(unmet, cmp.Or(it.Reason, it.Label, it.Name))
	}
	return strings.Join(unmet, "; ")
}

// criteriaByWant is the acceptance criteria of each want asked (its
// children of the criterion type), each with its verdict's status and when it
// ran. Only the wants asked: the verdicts of every other criterion are never
// read whole (: reading them all weighed on each read of
// « À faire »).
func criteriaByWant(ctx context.Context, cwm cwmapi.CWMService, wants map[string]bool) map[string][]api.Criterion {
	out := map[string][]api.Criterion{}
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: typeCriterion})
	if err != nil {
		return out
	}
	type found struct {
		c      api.Criterion
		bare   string
		parent string
	}
	var mine []found
	needed := map[string]bool{}
	for i, o := range list.Objects {
		if o.Parent == "" {
			continue
		}
		c := api.Criterion{ID: o.ID, Title: o.DisplayTitle(), Status: verdictUnproven}
		// Several projects served: each want's own, by its qualified id.
		parent := o.Parent
		if len(list.Projects) == len(list.Objects) && list.Projects[i] != "" {
			key := list.Projects[i]
			c.ID, parent = cwmapi.QualifiedID(key, o.ID), cwmapi.QualifiedID(key, parent)
		}
		if !wants[parent] {
			continue
		}
		mine = append(mine, found{c, o.ID, parent})
		needed[o.ID] = true
	}
	last := verdictsByCriterion(ctx, cwm, needed)
	for _, f := range mine {
		if v, ok := last[f.bare]; ok {
			f.c.Status, f.c.RanAt = v.Status, v.RanAt
		}
		out[f.parent] = append(out[f.parent], f.c)
	}
	return out
}

// verdictsByCriterion is the verdict of each criterion asked: its status and
// when it ran.
func verdictsByCriterion(ctx context.Context, cwm cwmapi.CWMService, criteria map[string]bool) map[string]api.Criterion {
	out := map[string]api.Criterion{}
	if len(criteria) == 0 {
		return out
	}
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: typeVerdict})
	if err != nil {
		return out
	}
	whole := cwmapi.NewWholeReader(ctx, cwm, typeVerdict)
	for _, o := range list.Objects {
		if o.Parent != "" && !criteria[o.Parent] {
			continue // another criterion's: not read whole
		}
		w, err := whole.One(o)
		if err != nil || w.Parent == "" || !criteria[w.Parent] {
			continue
		}
		v := w.Capabilities[typeVerdict]
		status, _ := v["status"].(string)
		ranAt, _ := v["ran_at"].(string)
		if status == "" {
			continue
		}
		out[w.Parent] = api.Criterion{Status: status, RanAt: ranAt}
	}
	return out
}

// requestsByWant is the requests that built each want: those that serve it
// (one filed on the want itself is on its own page).
func requestsByWant(ctx context.Context, cwm cwmapi.CWMService) map[string][]string {
	out := map[string][]string{}
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: api.CapRequest})
	if err != nil {
		return out
	}
	for _, o := range qualifyListed(list.Objects, list.Projects) {
		for _, e := range o.Edges {
			if string(e.Kind) == api.EdgeServes {
				out[e.To] = append(out[e.To], o.ID)
			}
		}
	}
	return out
}
