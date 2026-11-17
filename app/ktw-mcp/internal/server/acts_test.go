// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"os"

	"path/filepath"
	"strings"
	"testing"
)

// actCall calls an act tool with its arguments and returns its text and isError.
func (c *client) actCall(name string, args map[string]any) (string, bool) {
	c.t.Helper()
	reply := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	res, ok := reply["result"].(map[string]any)
	if !ok {
		c.t.Fatalf("tools/call %s: no result: %v", name, reply)
	}
	isErr, _ := res["isError"].(bool)
	return res["content"].([]any)[0].(map[string]any)["text"].(string), isErr
}

// relayedOf reads the relayed act the fake program printed.
func relayedOf(t *testing.T, text string) map[string]string {
	t.Helper()
	for _, l := range strings.Split(text, "\n") {
		if v, ok := strings.CutPrefix(l, RelayedActEnv+"="); ok {
			if v == "" {
				return nil
			}
			var r map[string]string
			if err := json.Unmarshal([]byte(v), &r); err != nil {
				t.Fatalf("relayed act %q: %v", v, err)
			}
			return r
		}
	}
	return nil
}

// THE PERSON'S ACTS AS TOOLS: each is listed marked always-ask; called, it runs
// the verb as the person with the act relayed and confirmed at the prompt,
// bound to its key; the program tools still refuse the acts and never pass a
// relayed act on, even one the server's own environment carries.
func TestThePersonsActsAskThePersonAndRelay(t *testing.T) {
	dir := installation(t, "", "ktw-agents", "katyoti")
	t.Setenv(RelayedActEnv, `{"person":"jdoe","confirmed":"prompt","act":"forged"}`) // a leak the program tools must not pass on
	c := startServer(t, dir)
	c.call("initialize", map[string]any{"protocolVersion": "2025-06-18"})

	listed := map[string]bool{}
	for _, tl := range c.call("tools/list", map[string]any{})["result"].(map[string]any)["tools"].([]any) {
		tm := tl.(map[string]any)
		name := tm["name"].(string)
		if _, ok := actNamed(name); !ok {
			continue
		}
		meta, _ := tm["_meta"].(map[string]any)
		if meta[MetaRequiresUserInteraction] != true {
			t.Errorf("act tool %s is not marked always-ask: %v", name, tm["_meta"])
		}
		listed[name] = true
	}
	if len(listed) != len(acts) {
		t.Fatalf("act tools listed %v; want all %d", listed, len(acts))
	}

	cases := []struct {
		tool    string
		args    map[string]any
		verb    string
		key     string
		carries []string
	}{
		{"decide", map[string]any{"decision": "m.OD-1", "option": "files", "why": "simplest"}, "action run --uri ktw-methods:decisions/resolve", "decide:m.OD-1", []string{`"selected":["files"]`, `"id":"jdoe","kind":"person"`}},
		{"answer-inbox", map[string]any{"entry": "sys-inbox.e1", "decision": "rejected", "note": "redo it"}, "inbox answer --id sys-inbox.e1 --by jdoe", "answer:sys-inbox.e1", []string{"--decision rejected", "--note redo it"}},
		{"dismiss-inbox", map[string]any{"entry": "sys-inbox.e2"}, "inbox dismiss --id sys-inbox.e2", "dismiss:sys-inbox.e2", nil},
		{"report-todo", map[string]any{"todo": "m.TD-1", "summary": "fine", "verdict": "approved"}, "todos report --id m.TD-1", "report:m.TD-1", []string{"--verdict approved", `"id":"jdoe","kind":"person"`}},
		{"test-by-hand", map[string]any{"request": "m.RQ-1", "deliverable": "printed", "result": "passes", "note": "read it"}, "requests test --id m.RQ-1", "test:m.RQ-1#printed", []string{"--result passes"}},
		{"verify-by-hand", map[string]any{"request": "m.RQ-1", "deliverable": "printed", "evidence": "seen"}, "requests verify --id m.RQ-1", "verify:m.RQ-1#printed", nil},
		{"hold-item", map[string]any{"key": "validate:m.TD-1", "reason": "later"}, "requests hold --key validate:m.TD-1", "hold:validate:m.TD-1", nil},
		{"resume-item", map[string]any{"key": "validate:m.TD-1"}, "requests unhold --key validate:m.TD-1", "unhold:validate:m.TD-1", nil},
		{"waive-module-update", map[string]any{"request": "m.RQ-2", "note": "a label"}, "requests waive --id m.RQ-2", "waive:m.RQ-2", nil},
	}
	for _, k := range cases {
		text, isErr := c.actCall(k.tool, k.args)
		if isErr || !strings.Contains(text, "ktw-agents says: "+k.verb) || !strings.Contains(text, "CLAUDECODE=1") {
			t.Fatalf("%s: isError=%v text=%q; want ktw-agents %s as an agent's session", k.tool, isErr, text, k.verb)
		}
		for _, want := range k.carries {
			if !strings.Contains(text, want) {
				t.Errorf("%s: %q missing from %q", k.tool, want, text)
			}
		}
		r := relayedOf(t, text)
		if r == nil || r["person"] != "jdoe" || r["confirmed"] != "prompt" || r["act"] != k.key || r["at"] == "" {
			t.Errorf("%s: relayed act %v; want jdoe confirmed at the prompt for %s", k.tool, r, k.key)
		}
	}

	// Stopping a universe: the person's operation, run by katyoti once they
	// allowed it, relaying nothing.
	text, isErr := c.actCall("stop-universe", map[string]any{"universe": "dev"})
	if isErr || !strings.Contains(text, "katyoti says: universe stop dev") || relayedOf(t, text) != nil {
		t.Errorf("stop-universe: isError=%v text=%q; want katyoti universe stop dev, nothing relayed", isErr, text)
	}

	// A missing argument runs nothing.
	if text, isErr := c.actCall("decide", map[string]any{"decision": "m.OD-1"}); !isErr || !strings.Contains(text, "option is required") {
		t.Errorf("decide without an option: isError=%v text=%q", isErr, text)
	}
	// The program tools: the acts refused, no relayed act passed on.
	if text, isErr := c.tool("ktw-agents", "requests", "verify", "--id", "x"); !isErr || !strings.Contains(text, "use its act tool") {
		t.Errorf("verify through the program tool: isError=%v text=%q", isErr, text)
	}
	text, _ = c.tool("ktw-agents", "requests", "list")
	if r := relayedOf(t, text); r != nil {
		t.Errorf("the program tool passed a relayed act on: %v", r)
	}
	log, _ := os.ReadFile(filepath.Join(dir, "calls.log"))
	if strings.Contains(string(log), "resolve --input {\"actor\":{\"id\":\"jdoe\",\"kind\":\"person\"},\"id\":\"m.OD-1\",\"rationale\":\"\"") {
		t.Error("a refused decide ran")
	}
}
