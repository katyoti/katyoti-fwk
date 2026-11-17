// SPDX-License-Identifier: Apache-2.0

package simulator

import (
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// TestSimulator_ReturnsConfiguredOutput (UC-150) — the simplest possible
// strategy contract: `type: simulator` with `config.output: <value>` runs
// and returns the configured value verbatim. The L2 closure proves this
// end-to-end via the CLI; this L1 test isolates the factory + machine path.
func TestSimulator_ReturnsConfiguredOutput(t *testing.T) {
	t.Run("map output", func(t *testing.T) {
		out := runSimulator(t, &taskapi.TaskExecRequest{
			Config: map[string]any{
				"duration_ms": 0,
				"output":      map[string]any{"summary": "uc-150-ok", "value": 42},
			},
		})
		m, _ := out.(map[string]any)
		if m == nil {
			t.Fatalf("output not a map: %#v", out)
		}
		// Note: simulator's resolveRequest round-trips config through JSON
		// (json.Marshal → Unmarshal), so int literals become float64. Compare
		// against the post-round-trip type.
		if m["summary"] != "uc-150-ok" || m["value"] != float64(42) {
			t.Errorf("output = %#v, want summary=uc-150-ok + value=42", m)
		}
	})

	t.Run("string output", func(t *testing.T) {
		out := runSimulator(t, &taskapi.TaskExecRequest{
			Config: map[string]any{
				"duration_ms": 0,
				"output":      "uc-150-string-output",
			},
		})
		if out != "uc-150-string-output" {
			t.Errorf("output = %v, want uc-150-string-output", out)
		}
	})
}
