// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// callAnswering calls a tool and answers the server's elicitation requests on
// the way with answer (the form's result); it returns the tool's text, isError
// and the forms the server sent.
func (c *client) callAnswering(name string, args map[string]any, answer map[string]any) (string, bool, []map[string]any) {
	c.t.Helper()
	c.id++
	msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	if _, err := c.in.Write(append(msg, '\n')); err != nil {
		c.t.Fatal(err)
	}
	var forms []map[string]any
	for c.out.Scan() {
		var m map[string]any
		if err := json.Unmarshal(c.out.Bytes(), &m); err != nil {
			c.t.Fatalf("unreadable line %q", c.out.Text())
		}
		if m["method"] == "elicitation/create" {
			forms = append(forms, m["params"].(map[string]any))
			reply, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m["id"], "result": answer})
			fmt.Fprintf(c.in, "%s\n", reply)
			continue
		}
		res := m["result"].(map[string]any)
		isErr, _ := res["isError"].(bool)
		return res["content"].([]any)[0].(map[string]any)["text"].(string), isErr, forms
	}
	c.t.Fatal("the server ended")
	return "", false, nil
}

// AN AUTHENTICATOR CODE: a client with forms is asked for the
// code by elicitation - the code reaches the verb as the relay's proof, never
// the tool's arguments - and a declined form runs nothing; a client without
// forms takes the code typed, and without one it runs nothing.
func TestACodeAsksTheFormOrTakesItTyped(t *testing.T) {
	dir := installation(t, "", "ktw-agents", "katyoti")
	decide := func() map[string]any {
		return map[string]any{"decision": "m.OD-1", "option": "files", "why": "by code", "confirm": "code"}
	}

	// A client that shows forms.
	c := startServer(t, dir)
	c.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"elicitation": map[string]any{}}})
	text, isErr, forms := c.callAnswering("decide", decide(), map[string]any{"action": "accept", "content": map[string]any{"code": "654321"}})
	if len(forms) != 1 || !strings.Contains(fmt.Sprint(forms[0]["message"]), "decide:m.OD-1") {
		t.Fatalf("forms asked: %v; want one naming the act", forms)
	}
	r := relayedOf(t, text)
	if isErr || r == nil || r["confirmed"] != "totp" || r["proof"] != "654321" || r["act"] != "decide:m.OD-1" {
		t.Fatalf("code by the form: isError=%v relayed %v text=%q; want the decision run, confirmed totp with the code", isErr, r, text)
	}
	if strings.Contains(strings.Split(text, "\n")[1], "654321") {
		t.Errorf("the code reached the command line: %q", text)
	}
	text, isErr, _ = c.callAnswering("decide", decide(), map[string]any{"action": "decline"})
	if !isErr || !strings.Contains(text, "declined") || !strings.Contains(text, "not run") {
		t.Fatalf("form declined: isError=%v text=%q; want refused, not run", isErr, text)
	}

	// A client without forms.
	p := startServer(t, dir)
	p.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}})
	text, isErr, forms = p.callAnswering("decide", decide(), nil)
	if !isErr || len(forms) != 0 || !strings.Contains(text, "shows no form") {
		t.Fatalf("no form, no code: isError=%v forms=%v text=%q; want refused, asking for the code typed", isErr, forms, text)
	}
	typed := decide()
	typed["code"] = "111222"
	text, isErr, _ = p.callAnswering("decide", typed, nil)
	if r := relayedOf(t, text); isErr || r == nil || r["confirmed"] != "totp" || r["proof"] != "111222" {
		t.Fatalf("code typed: isError=%v relayed %v; want confirmed totp with it", isErr, r)
	}
}
