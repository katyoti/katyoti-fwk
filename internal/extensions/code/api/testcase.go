// SPDX-License-Identifier: Apache-2.0

package api

import "time"

// TestRunStatus is a test-case's last_run_status field. Distinct from
// UCStatus — a test-case has a single run outcome; a UC aggregates
// many.
type TestRunStatus string

const (
	TestRunPass         TestRunStatus = "pass"
	TestRunFail         TestRunStatus = "fail"
	TestRunHarnessError TestRunStatus = "harness-error"
	TestRunNotRun       TestRunStatus = "not-run"
)

// TestCase is the on-the-wire shape of a `test-case` item. Mirrors
// app/ktw-agents/embed/cwm/item_types/test-case.yaml.
type TestCase struct {
	ID     string `json:"id"                            jsonschema:"Item UUID"`
	NodeID string `json:"node_id"                       jsonschema:"Owning module-node UUID"`
	Title  string `json:"title"                         jsonschema:"Short description (≤ 120 chars)"`
	Body   string `json:"body,omitempty"                jsonschema:"Optional rationale / notes"`

	// Typed body
	TCID          string        `json:"tc_id"                       jsonschema:"Layer-prefixed stable id (L1-uc001-foo, L2-uc003-suspend)"`
	Layer         UCLayer       `json:"layer"                       jsonschema:"L1 | L2 | L3"`
	TestRef       string        `json:"test_ref"                    jsonschema:"Canonical reference: <pkg>_test.go::TestName | tests/L2/*.tasktest | tests/L3/*/TASK.md"`
	LastRunStatus TestRunStatus `json:"last_run_status,omitempty"   jsonschema:"Updated by verify; honor-system protected"`
	LastRunAt     *time.Time    `json:"last_run_at,omitempty"`
	LastRunID     string        `json:"last_run_id,omitempty"       jsonschema:"UUID of the test-run item from the most recent verify"`
	Negative      bool          `json:"negative,omitempty"          jsonschema:"True for negative-DoD assertions (must-not-happen)"`
	Tags          []string      `json:"tags,omitempty"`
}
