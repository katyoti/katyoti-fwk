// SPDX-License-Identifier: Apache-2.0

package api

// A DEMAND — what a person (or an outside system) asks, which does no work
// itself: it reconciles against the modules' releases, increments, goals,
// features and open requests, and proposes a delta - a closed set of
// operations planification checks. The person approves it once; an agent
// applies it through the verbs; the demand ends linked to what it changed
// (realized-by), its requester told.

// KindDemand is a demand's kind: its template reconciles.
const KindDemand = "demand"

// The demand's steps and the tasks that do them.
const (
	StepReconcile  = "reconcile"
	StepApplyDelta = "apply"
	ReconcileTask  = "reconcile"
	ApplyDeltaTask = "apply-delta"
	// ProposeDeliveryTask reconciles an increment's delivery goals.
	ProposeDeliveryTask = "propose-delivery"
)

// The result keys a reconcile and an apply step report under.
const (
	DeltaKey   = "delta"
	AppliedKey = "applied"
)

// EdgeRealizedBy points a demand at each object its delta changed or created.
const EdgeRealizedBy = "realized-by"

// The operations of a delta: the closed set.
const (
	OpGoalAdd          = "goal-add"
	OpGoalChange       = "goal-change"
	OpFeatureAdd       = "feature-add"
	OpFeatureChange    = "feature-change"
	OpIncrementCreate  = "increment-create"
	OpIncrementTarget  = "increment-target"
	OpIncrementDone    = "increment-done"
	OpRequestRescope   = "request-rescope"
	OpRequestMerge     = "request-merge"
	OpRequestSupersede = "request-supersede"
	OpRequestDrop      = "request-drop"
	OpDirect           = "direct"
)

// DeltaOps is the closed set, in the order a delta is read.
var DeltaOps = []string{OpGoalAdd, OpGoalChange, OpFeatureAdd, OpFeatureChange, OpIncrementCreate, OpIncrementTarget,
	OpIncrementDone, OpRequestRescope, OpRequestMerge, OpRequestSupersede, OpRequestDrop, OpDirect}

// DeltaOp is one operation of a demand's delta.
type DeltaOp struct {
	Op        string `json:"op"                  jsonschema:"description=goal-add or goal-change or feature-add or feature-change or increment-create or increment-target or increment-done or request-rescope or request-merge or request-supersede or request-drop or direct"`
	Object    string `json:"object,omitempty"    jsonschema:"description=The existing object it acts on (a change or a target or a request) - or #N: what operation N of the same delta made"`
	Module    string `json:"module,omitempty"    jsonschema:"description=The module an added goal or feature or a created increment or direct work goes in"`
	Stream    string `json:"stream,omitempty"    jsonschema:"description=The stream a created increment is in (increment-create): an increment is always created in its stream"`
	Under     string `json:"under,omitempty"     jsonschema:"description=The goal or feature an added one serves or sits under - or #N: the one operation N of the same delta adds"`
	Increment string `json:"increment,omitempty" jsonschema:"description=The increment an object is aimed at (increment-target) - or for a goal or feature operation the increment what it adds or changes goes to (a demand about an increment defaults it) - or #N: the one operation N of the same delta creates"`
	Into      string `json:"into,omitempty"      jsonschema:"description=The request a merged or superseding one goes to"`
	Title     string `json:"title,omitempty"     jsonschema:"description=Its wording: an added object's title or a request's new title"`
	Details   string `json:"details,omitempty"`
	For       string `json:"for,omitempty"       jsonschema:"description=An increment's proposal: the delivery goal this operation answers"`
	Role      string `json:"role,omitempty"      jsonschema:"description=For a feature-add: display-switch marks the feature that puts the increment's new screens behind its switch - built first and required when the increment changes screens; no-screen marks a feature that changes no screen though its module draws one (a guard or a migration or a parser) - it needs no switch and is tagged no-screen at apply"`
	Why       string `json:"why"                 jsonschema:"description=Why this operation answers the demand"`
}

// RoleDisplaySwitch is a feature-add's role: the increment's display switch.
const RoleDisplaySwitch = "display-switch"

// RoleNoScreen is a feature-add's role: the feature changes no screen although
// its module's code draws one - the display-switch guard does not count it, and
// it is tagged CapNoScreen at apply (RQ-420). The
// gui-tested gate still holds it when it realizes screen code.
const RoleNoScreen = "no-screen"

// CapDisplaySwitch tags the feature that is an increment's display switch.
const CapDisplaySwitch = "display-switch"

// DeltaRef is how an operation names what an earlier operation of the same
// delta makes: "#" and that operation's index (from 0) - a feature under the
// goal "#0" adds, an increment-target of "#1" at "#2". Apply resolves it to
// the id that operation made.
const DeltaRef = "#"

// Applied says which object an applied operation changed or created.
type Applied struct {
	Op int    `json:"op" jsonschema:"description=The operation's index in the delta (from 0)"`
	ID string `json:"id" jsonschema:"description=The object it changed or created"`
}
