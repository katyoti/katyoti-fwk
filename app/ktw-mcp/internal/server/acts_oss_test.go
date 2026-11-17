// SPDX-License-Identifier: Apache-2.0

package server

import (
	"strings"
	"testing"
)

// The open source build confirms an act at the prompt or by a code only: no
// prover is served, a mode only a development build adds is refused, nothing
// run.
func TestTheOpenSourceBuildServesNoProver(t *testing.T) {
	dir := installation(t, "", "ktw-agents", "katyoti")
	c := startServer(t, dir)
	c.call("initialize", map[string]any{"protocolVersion": "2025-06-18"})

	if len(provers) != 0 {
		t.Fatalf("provers %v; want none in the open source build", provers)
	}
	decide := map[string]any{"decision": "m.OD-1", "option": "files", "why": "x", "confirm": "entra"}
	if text, isErr := c.actCall("decide", decide); !isErr || !strings.Contains(text, "confirm is prompt or code") || strings.Contains(text, "says:") {
		t.Fatalf("confirm entra: isError=%v text=%q; want refused (prompt or code), not run", isErr, text)
	}
}
