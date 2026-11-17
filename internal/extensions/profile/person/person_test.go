// SPDX-License-Identifier: Apache-2.0

package person

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
)

// A process an agent's session runs is caused by the agent; one a person runs
// is not an agent's.
func TestAnAgentSessionIsTheCause(t *testing.T) {
	env := map[string]string{"CLAUDECODE": "1"}
	c, ok := AgentSessionCause(func(k string) string { return env[k] })
	if !ok || c.Kind != cause.KindAgent || c.ID != "claude-code" || c.Session != "" {
		t.Fatalf("under Claude Code: %+v, %v; want {agent claude-code}", c, ok)
	}
	// Its session's id, when the session gives one, rides with it
	//
	env["CLAUDE_CODE_SESSION_ID"] = "6f0493c9"
	if c, _ := AgentSessionCause(func(k string) string { return env[k] }); c.Session != "6f0493c9" {
		t.Fatalf("session: %q; want 6f0493c9", c.Session)
	}
	if c, ok := AgentSessionCause(func(string) string { return "" }); ok {
		t.Fatalf("a person's shell: %+v; want no agent", c)
	}
}
