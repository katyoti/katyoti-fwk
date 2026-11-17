// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"strings"
	"testing"
	"time"
)

// A step whose input cannot be evaluated fails, saying so — its child never
// runs with the raw expression as its input (it did, a log line saying so).
func TestAStepWhoseInputCannotBeEvaluatedFails(t *testing.T) {
	registerSimulator(t)
	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"name":  "child",
				"task":  map[string]any{"type": "simulator"},
				"input": map[string]any{"x": "${state.results.missing.deeper.call(}"},
			},
		},
	}
	r := runWorkflow(t, cfg, 5*time.Second)
	msg := ""
	if r.Error != nil {
		msg = r.Error.Error()
	}
	if out, ok := r.Output.(map[string]any); ok {
		if e, _ := out["error"].(string); e != "" {
			msg = e
		}
	}
	if !strings.Contains(msg, "cannot be evaluated") {
		t.Fatalf("result = %+v (error %q); want the step failed because its input cannot be evaluated", r, msg)
	}
}
