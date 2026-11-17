// SPDX-License-Identifier: Apache-2.0

package api

import "time"

// THE RECONCILER - a deterministic sweep over the stored
// state that repairs what is safe and raises an incident for the rest. Dry-run
// first: it says what it would repair; each repair is switched on by itself
// (Planification.yaml, reconciler.repairs) and a kill switch stops them all.
// A repair never undoes a person's or an agent's act, never re-plans started
// work, never starts a session and never calls an LLM; it applies only if what
// it observed is unchanged, and above three repairs of one element it stops
// and raises « flapping ».

// The repairs, each switched on by itself.
const (
	// RepairEndPlan ends the plan of an object whose lifecycle is done, when
	// that move is later than the plan's last step and nothing of it runs.
	RepairEndPlan = "end-plan"
	// RepairPutStep puts the planned work of a ready plan with no to-do: its
	// template's steps when none ran, its end when its last step ran.
	RepairPutStep = "put-step"
	// RepairApplyAnswer applies an answer given in the Inbox that its asking
	// step or request did not take up.
	RepairApplyAnswer = "apply-answer"
	// RepairClearWait clears a to-do's wait on an object that has ended.
	RepairClearWait = "clear-wait"
	// RepairLabels recomputes the labels of a to-do not started.
	RepairLabels = "labels"
	// RepairRepointWait moves a to-do's wait on a superseded request to its
	// last successor, while that one is open.
	RepairRepointWait = "repoint-wait"
	// RepairRetryUnnamed sends a step blocked with no named cause back to the
	// queue once, past its deadline (reconciler.unclassified_hours).
	RepairRetryUnnamed = "retry-unclassified"
	// RepairDecideWait opens ONE prepared decision for the person on a wait
	// no rule resolves: a cycle, an unreadable object, a dependency nobody
	// plans, a superseded request with no readable successor.
	RepairDecideWait = "decide-wait"
)

// Repairs are the repairs in the order a sweep runs them.
var Repairs = []string{RepairEndPlan, RepairPutStep, RepairApplyAnswer, RepairRepointWait, RepairClearWait, RepairRetryUnnamed, RepairLabels, RepairDecideWait}

// DefaultUnclassifiedHours is how long a step blocked with no named cause
// stays with whoever reported it before the reconciler retries it once - or,
// retried already, before it is the person's.
const DefaultUnclassifiedHours = 24

// IncidentFlapping is the check of an element the reconciler repaired too
// often: it stops repairing it.
const IncidentFlapping = "flapping"

// ReconcilerActorID is who the reconciler's repairs are recorded by.
const ReconcilerActorID = "planification-reconciler"

// ReconcilerConfig is the reconciler's part of Planification.yaml.
type ReconcilerConfig struct {
	// Off is the kill switch: no repair applies, whatever is switched on.
	Off bool `json:"off,omitempty" yaml:"off,omitempty" jsonschema:"description=The kill switch: no repair applies"`
	// Repairs are the repairs switched on; the others are only reported.
	Repairs []string `json:"repairs,omitempty" yaml:"repairs,omitempty" jsonschema:"description=The repairs switched on (end-plan or put-step or apply-answer or repoint-wait or clear-wait or retry-unclassified or labels or decide-wait); none: dry-run"`
	// IntervalMinutes is the period of the sweep in the lease holder; 0: 10.
	IntervalMinutes int `json:"interval_minutes,omitempty" yaml:"interval_minutes,omitempty" jsonschema:"description=Minutes between two sweeps; 10 when unset; -1 stops the periodic sweep"`
	// UnclassifiedHours is the deadline of a block with no named cause; 0: 24.
	UnclassifiedHours int `json:"unclassified_hours,omitempty" yaml:"unclassified_hours,omitempty" jsonschema:"description=Hours a step blocked with no named cause stays with who reported it before the reconciler retries it once; 24 when unset"`
}

// UnclassifiedDeadline is the deadline of a block with no named cause.
func (c ReconcilerConfig) UnclassifiedDeadline() time.Duration {
	if c.UnclassifiedHours <= 0 {
		return DefaultUnclassifiedHours * time.Hour
	}
	return time.Duration(c.UnclassifiedHours) * time.Hour
}

// On says whether a repair is switched on.
func (c ReconcilerConfig) On(repair string) bool {
	if c.Off {
		return false
	}
	for _, r := range c.Repairs {
		if r == repair {
			return true
		}
	}
	return false
}

// ReconcileInput runs one sweep.
type ReconcileInput struct {
	Apply   bool     `json:"apply,omitempty" jsonschema:"description=Apply the repairs switched on in Planification.yaml; without it the sweep only says what it would repair"`
	Repairs []string `json:"repairs,omitempty" jsonschema:"description=Only these repairs (of those switched on)"`
}

// Repair is one repair the sweep found: applied, or what it would do.
type Repair struct {
	Repair string `json:"repair"`
	Work   string `json:"work" jsonschema:"description=The piece of work: the object carrying the request"`
	Todo   string `json:"todo,omitempty"`
	// Reason is what is wrong; Action what the repair does.
	Reason string `json:"reason"`
	Action string `json:"action"`
	// Applied says it was done; Skipped why it was not (dry-run, switched
	// off, moved since observed, flapping, failed).
	Applied bool   `json:"applied,omitempty"`
	Skipped string `json:"skipped,omitempty"`
}

// RepairDone is a repair the reconciler applied and when (RFC 3339): the
// incidents' list keeps them a week, so a scope shows what was repaired on
// it without an agent (an increment's Blockers tab).
type RepairDone struct {
	Repair
	At string `json:"at"`
}

// Incident is what the sweep cannot repair safely: one per check and element.
type Incident struct {
	// Key is check:element - the same state raises the same key.
	Key    string `json:"key"`
	Check  string `json:"check"`
	Work   string `json:"work"`
	Todo   string `json:"todo,omitempty"`
	Reason string `json:"reason"`
	// Cause is the incident's cause (Blocker.Cause) when it has one.
	Cause string `json:"cause,omitempty"`
	// Actor is who can clear it.
	Actor string `json:"actor,omitempty"`
}

// ReconcileOutput is one sweep: its repairs and its incidents, each once.
type ReconcileOutput struct {
	// DryRun says nothing was applied; Why says why (no apply asked, the
	// kill switch, the day's limit of put to-dos).
	DryRun    bool       `json:"dry_run"`
	Why       string     `json:"why,omitempty"`
	Repairs   []Repair   `json:"repairs"`
	Incidents []Incident `json:"incidents"`
	// Open is the incidents' list after this sweep: every open incident,
	// with since when and how many sweeps saw it.
	Open []IncidentRecord `json:"open,omitempty"`
	// TookMS is how long the sweep took, read to set its period
	//
	TookMS int64 `json:"took_ms" jsonschema:"description=How long the sweep took in milliseconds"`
}

// THE INCIDENTS' LIST - a system list, never a to-do:
// one record per check and element (its Key), kept in the project's own
// `.katyoti/` (uncommitted, recomputable), never under modules/. A sweep that
// sees an incident again updates its record; one that sees it no more counts
// a clean sweep, and the second clean sweep in a row closes it. The repairs'
// attempts are kept beside, so the flap limit holds across a restart.

// IncidentCleanSweeps is how many clean sweeps in a row close an incident.
const IncidentCleanSweeps = 2

// IncidentRecord is one incident of the list.
type IncidentRecord struct {
	Incident
	// Since is when it was raised (or raised again after it closed); Seen
	// when a sweep last saw it.
	Since string `json:"since"`
	Seen  string `json:"seen"`
	// Sweeps is how many sweeps saw it since it was raised; Clean how many
	// sweeps in a row did not.
	Sweeps int `json:"sweeps"`
	Clean  int `json:"clean,omitempty"`
	// Closed is when its second clean sweep closed it; "" while open.
	Closed string `json:"closed,omitempty"`
}
