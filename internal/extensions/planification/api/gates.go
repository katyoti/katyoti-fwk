// SPDX-License-Identifier: Apache-2.0

package api

import cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"

// The gates of planification's phases, declared in its pack (cwm/gates/). A
// type or a pack adds its requirements to them (gate-requirements/); a
// workflow transition names one with `gate: "gate('<name>').passes"`.
const (
	// GateReadyToPlan — what it is and what it depends on are known.
	GateReadyToPlan = "ready-to-plan"
	// GateReadyToBuild — specified and challenged by a person.
	GateReadyToBuild = "ready-to-build"
	// GateReadyToTest — its verification declared and green, no blocking lint.
	GateReadyToTest = "ready-to-test"
	// GateReadyToRelease — every user-visible change in the change log.
	GateReadyToRelease = "ready-to-release"
)

// CapNoScreen tags a feature that changes no screen although its module's code
// draws one (a pack shipping object panels, the GUI): a code removal, a
// migration, a guard. The gui-tested requirement of ready-to-test does not
// hold it - unless it realizes screen code or a criterion serving it runs a
// live GUI test, which say otherwise (RQ-301).
const CapNoScreen = "no-screen"

// CapGateWait is carried by an object whose move a gate refused while the
// planner resolves the unmet requirements: the move. What resolves each
// requirement is read from the edges — the requests that resolve the object
// (or the object's own request), the inbox asks it is asked-in — filed since.
const CapGateWait = "gate-wait"

// GateWaitFields is the gate-wait field-group (cap-gate-wait).
type GateWaitFields struct {
	Gate       string       `json:"gate" jsonschema:"description=The gate that refused the move"`
	Transition string       `json:"transition,omitempty" jsonschema:"description=The refused transition"`
	To         string       `json:"to" jsonschema:"description=The state the move goes to; tried again once nothing resolving it is open"`
	By         cwmapi.Actor `json:"by" jsonschema:"description=Who asked for the move; the retry moves as them"`
	Since      string       `json:"since,omitempty" jsonschema:"description=When the move was first refused (RFC 3339); what was filed before belongs to an earlier wait"`
}
