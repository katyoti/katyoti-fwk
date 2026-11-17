// SPDX-License-Identifier: Apache-2.0

package api

import cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"

// An agent tests first: a deliverable no check verifies is given to an agent
// before a person — it writes the check that proves it (falsified once), records
// it on the request and runs it; one only a person can judge it marks by_hand,
// saying how and why. The step that does it is StepAutoTest, run by
// AutoTestTask.
const (
	StepAutoTest = "auto-test"
	AutoTestTask = "auto-test"
)

// AutoTestInput asks an agent to test a request's deliverables no check
// verifies — all of them, or those named.
type AutoTestInput struct {
	ID           string       `json:"id" jsonschema:"description=The request,required"`
	Deliverables []string     `json:"deliverables,omitempty" jsonschema:"description=The deliverables to test (titles); all those no check verifies when empty"`
	By           cwmapi.Actor `json:"by" jsonschema:"description=Who asks,required"`
}
