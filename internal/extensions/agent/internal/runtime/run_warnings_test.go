// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// warnings records what an agent warns on its run.
type warnings []string

func (w *warnings) warn(r string) { *w = append(*w, r) }

func (w warnings) has(part string) bool {
	for _, r := range w {
		if strings.Contains(r, part) {
			return true
		}
	}
	return false
}

// What goes wrong around an agent's step while its run goes on is a warning on
// the run (it was only logged, or dropped): a failing error hook, a failing
// stream hook (once per turn), a tool input that is not a JSON object.
func TestAnAgentWarnsOnItsRun(t *testing.T) {
	var got warnings
	failing := errors.New("hook down")
	m := &AgentLoopMachine{warn: got.warn, loop: NewLoopContext(), plugins: NewPluginSet(), hooks: &HookChain{
		OnError:       []func(api.LoopContext, *api.LoopError) error{func(api.LoopContext, *api.LoopError) error { return failing }},
		OnModelStream: []func(api.LoopContext, api.StreamChunk) error{func(api.LoopContext, api.StreamChunk) error { return failing }},
	}}
	m.fireOnError(&api.LoopError{Phase: "llm", Err: errors.New("boom")})
	var warned bool
	m.streamHooks(api.StreamChunk{Type: "thinking_delta"}, &warned)
	m.streamHooks(api.StreamChunk{Type: "thinking_delta"}, &warned)
	if !got.has("error hook failed") || len(got) != 2 || !got.has("stream hook failed") {
		t.Errorf("warnings = %q; want the error hook's and the stream hook's (once)", got)
	}

	got = nil
	mm, as := awaitingMachine(func(context.Context, json.RawMessage) (*api.ToolCallResult, error) {
		return &api.ToolCallResult{Data: "ok"}, nil
	}, `[1, 2]`, &HookChain{})
	mm.warn = got.warn
	dispatchOnce(t, mm, as)
	if !got.has("not a JSON object") {
		t.Errorf("warnings = %q; want the tool input that is not an object", got)
	}
}

// An "always" rule a person chose that cannot be saved is a warning on the run:
// they believe it remembered.
func TestARuleThatCannotBeSavedIsAWarning(t *testing.T) {
	const bad = "file-read-without-a-colon"
	if _, err := permrules.ParseRule(bad); err == nil {
		t.Skip("the rule parses; pick one that does not")
	}
	var got warnings
	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionAllowAlways, selectedRule: bad}
	d, _ := dispatcherWithTool("file-read", toolsapi.PermissionResult{Behavior: toolsapi.PermissionAsk, Message: "ok?"})
	ctx := machine.WithRunWarner(newAppCtxWithGateAndProfiles(t, gate, &fakeProfiles{profile: &permrules.Profile{Name: "p"}}), got.warn)
	applyPermissionGate(ctx, d, "file-read", json.RawMessage(`{}`))
	if !got.has("could not be saved") {
		t.Errorf("warnings = %q; want the rule not saved", got)
	}
}
