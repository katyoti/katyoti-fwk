// SPDX-License-Identifier: Apache-2.0

// Package assistant implements the method-core "where are we / what's next"
// derivation (modules/code/concepts.md §4.3): a PURE function
// of workspace state → an overview, the alignment-loop findings
// (GAP / WASTE / ORPHAN), and a ranked list of suggested next actions.
//
// It is ADVISORY — it reads state and ranks, never writes. The wiring layer
// gathers the Snapshot from CWM (goals, features, the acceptance criteria
// under them with the status of their recorded verdicts, the serves edges)
// and calls these functions. The want ladder is AC → feature (recursive) →
// goal: a feature is delivered when every criterion under it passes.
package assistant

import "sort"

// CoD is a feature / goal's Cost of Delay (method-core: the prioritization
// signal; effort is derived, never stored here). It is stated either as one
// number — Declared, a feature's `cod` — or as its three primitives.
type CoD struct {
	Declared        int
	BusinessValue   int
	TimeCriticality int
	RiskOpportunity int
}

// Sum is the Cost of Delay used for ranking.
func (c CoD) Sum() int {
	return c.Declared + c.BusinessValue + c.TimeCriticality + c.RiskOpportunity
}

// Goal is a standing north-star; ServingUS is the set of features that
// `serves` it (derived from serves edges).
type Goal struct {
	ID        string
	Title     string
	Scope     string // method | project
	CoD       CoD
	ServingUS []string
}

// Feature is the method's recursive want node (objects of type `feature`).
// It carries its CoD, the goals it `serves`, and the acceptance criteria that
// prove it: those under it, directly or through its sub-features.
type Feature struct {
	ID       string
	Title    string
	CoD      CoD
	Serves   []string // goal ids
	Criteria []string // acceptance-criterion ids under it, at any depth
}

// Criterion is one acceptance criterion: the status its recorded verdict
// holds and the nearest feature it sits under.
type Criterion struct {
	ID        string
	Title     string
	Status    string // its verdict's status: pass | fail | needs-harness | no-probe | harness-error; "" = never run
	ParentID  string // the object it sits under
	FeatureID string // the nearest feature above it; "" = it sits under none
}

// Snapshot is the gathered workspace state the assistant derives from.
type Snapshot struct {
	Goals    []Goal
	Features []Feature
	Criteria []Criterion
}

// ----------------------------------------------------------------------------
// Status helpers — a criterion's verdict status
// ----------------------------------------------------------------------------

// Passes reports a criterion whose last run passed.
func Passes(status string) bool { return status == "pass" }

// NoTest reports a criterion with nothing run: no verdict, or no probe to run.
func NoTest(status string) bool { return status == "" || status == "no-probe" }

// Failing reports a criterion whose probe ran and failed — delivery work to
// build to green.
func Failing(status string) bool { return status == "fail" }

// ----------------------------------------------------------------------------
// Alignment loop — GAP / WASTE / ORPHAN (modules/code/concepts.md §4.3)
// ----------------------------------------------------------------------------

// Finding is one alignment-loop result.
type Finding struct {
	Kind    string // GAP | WASTE | ORPHAN
	Subject string // the goal / feature / criterion id
	Title   string
	Message string
}

// Findings runs the alignment loop: GAP (a goal no feature serves), WASTE (a
// feature serving no goal), ORPHAN (a criterion under no feature). The three
// are symmetric — purpose-without-want, want-without-purpose,
// have-without-want.
func Findings(s Snapshot) []Finding {
	var out []Finding
	for _, g := range s.Goals {
		if len(g.ServingUS) == 0 {
			out = append(out, Finding{"GAP", g.ID, g.Title, "goal has no serving feature — shape one (purpose without want)"})
		}
	}
	for _, f := range s.Features {
		if len(f.Serves) == 0 {
			out = append(out, Finding{"WASTE", f.ID, f.Title, "feature serves no goal — link it or question it (want without purpose)"})
		}
	}
	for _, c := range s.Criteria {
		if c.FeatureID == "" {
			out = append(out, Finding{"ORPHAN", c.ID, c.Title, "criterion sits under no feature — place it under the feature it proves (have without want)"})
		}
	}
	return out
}

// ----------------------------------------------------------------------------
// Overview — the derived readout
// ----------------------------------------------------------------------------

// GoalGauge is the per-goal advancement readout (open-ended, never "done").
type GoalGauge struct {
	ID          string
	Title       string
	ServingUS   int
	DeliveredUS int // serving features whose criteria all pass
}

// Overview is the derived summary of "where are we".
type Overview struct {
	GoalGauges       []GoalGauge
	USTotal          int
	USDelivered      int // features whose criteria all pass
	CriteriaTotal    int
	CriteriaPassing  int
	CriteriaNoTest   int // no verdict, or no probe
	CriteriaFailing  int
	CriteriaUnproven int // the check could not run (needs-harness, harness-error)
}

func statuses(s Snapshot) map[string]string {
	by := make(map[string]string, len(s.Criteria))
	for _, c := range s.Criteria {
		by[c.ID] = c.Status
	}
	return by
}

// Delivered reports a feature with at least one criterion, all passing;
// status is each criterion's verdict status by id.
func Delivered(f Feature, status map[string]string) bool {
	if len(f.Criteria) == 0 {
		return false
	}
	for _, id := range f.Criteria {
		if !Passes(status[id]) {
			return false
		}
	}
	return true
}

// ComputeOverview derives the where-are-we summary.
func ComputeOverview(s Snapshot) Overview {
	status := statuses(s)
	var ov Overview
	ov.USTotal = len(s.Features)
	delivered := map[string]bool{}
	for _, f := range s.Features {
		if Delivered(f, status) {
			ov.USDelivered++
			delivered[f.ID] = true
		}
	}
	for _, c := range s.Criteria {
		ov.CriteriaTotal++
		switch {
		case Passes(c.Status):
			ov.CriteriaPassing++
		case NoTest(c.Status):
			ov.CriteriaNoTest++
		case Failing(c.Status):
			ov.CriteriaFailing++
		default:
			ov.CriteriaUnproven++
		}
	}
	for _, g := range s.Goals {
		gg := GoalGauge{ID: g.ID, Title: g.Title, ServingUS: len(g.ServingUS)}
		for _, id := range g.ServingUS {
			if delivered[id] {
				gg.DeliveredUS++
			}
		}
		ov.GoalGauges = append(ov.GoalGauges, gg)
	}
	return ov
}

// ----------------------------------------------------------------------------
// Next actions — the ranked advisory (modules/code/concepts.md §4.3)
// ----------------------------------------------------------------------------

// Action is one ranked next-action suggestion. Higher Score ranks first.
type Action struct {
	Kind      string // shape-us | build | write-probe | plan | review-waste | retro-link | capture-want
	Subject   string
	Title     string
	Rationale string
	Score     int
}

// Severity floors keep the ladder stable: GAP, then a failing criterion, then
// no-test, then plan, then WASTE. CoD is added on top so high-value items
// outrank low-value ones within a band.
const (
	scoreGap    = 500
	scoreBuild  = 450
	scoreNoTest = 300
	scorePlan   = 200
	scoreWaste  = 50
	scoreOrphan = 40
	scoreEmpty  = 10
)

// NextActions maps state signals to suggested actions, scored + ranked.
// Pure function; never writes. Deduplicates by (kind, subject) keeping the
// highest score.
func NextActions(s Snapshot) []Action {
	status := statuses(s)
	codOf := map[string]int{}
	for _, f := range s.Features {
		codOf[f.ID] = f.CoD.Sum()
	}
	acc := map[string]Action{}
	add := func(a Action) {
		key := a.Kind + "\x00" + a.Subject
		if cur, ok := acc[key]; !ok || a.Score > cur.Score {
			acc[key] = a
		}
	}

	// 1. GAP — a goal no feature serves. Shape a feature (∝ goal CoD).
	for _, g := range s.Goals {
		if len(g.ServingUS) == 0 {
			add(Action{"shape-us", g.ID, g.Title, "goal has no serving feature (alignment GAP)", scoreGap + g.CoD.Sum()})
		}
	}
	for _, c := range s.Criteria {
		if c.FeatureID == "" {
			continue
		}
		// 2. failing — a feature's criterion ran RED. Build it to green
		// (∝ its feature's CoD). Ranks below GAP, above no-test/plan.
		if Failing(c.Status) {
			add(Action{"build", c.ID, c.Title, "criterion is failing — build it to green", scoreBuild + codOf[c.FeatureID]})
		}
		// 3. no-test — a feature's criterion has no verdict. Write its probe,
		// or run it (∝ its feature's CoD).
		if NoTest(c.Status) {
			add(Action{"write-probe", c.ID, c.Title, "criterion has no verdict — write its probe, or run it", scoreNoTest + codOf[c.FeatureID]})
		}
	}
	// 4. ready-to-plan — a high-CoD feature with criteria not all passing.
	for _, f := range s.Features {
		if f.CoD.Sum() > 0 && len(f.Criteria) > 0 && !Delivered(f, status) {
			add(Action{"plan", f.ID, f.Title, "high-CoD feature ready to work on", scorePlan + f.CoD.Sum()})
		}
	}
	// 5. WASTE — a feature serving no goal. Review.
	for _, f := range s.Features {
		if len(f.Serves) == 0 {
			add(Action{"review-waste", f.ID, f.Title, "feature serves no goal (alignment WASTE)", scoreWaste})
		}
	}
	// 6. ORPHAN — a criterion under no feature. Place it under one.
	for _, c := range s.Criteria {
		if c.FeatureID == "" {
			add(Action{"retro-link", c.ID, c.Title, "criterion sits under no feature (alignment ORPHAN)", scoreOrphan})
		}
	}

	out := make([]Action, 0, len(acc))
	for _, a := range acc {
		out = append(out, a)
	}
	// 7. nothing actionable — entry suggestion.
	if len(out) == 0 {
		out = append(out, Action{"capture-want", "", "", "all green — revisit goals or capture new wants", scoreEmpty})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Subject < out[j].Subject // stable tie-break
	})
	return out
}
