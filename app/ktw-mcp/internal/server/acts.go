// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// THE PERSON'S ACTS AS TOOLS (level 1): deciding, answering or
// dismissing an Inbox entry, reporting their to-do, testing or verifying by
// hand, holding or resuming an item, waiving a module update. Each is a tool of
// its own marked anthropic/requiresUserInteraction: Claude Code asks the person
// on every call, in every mode, at the PC or on the phone under Remote Control.
// Once they allowed it, the tool runs the verb as the person (--by their profile
// handle) with the act relayed and confirmed at the prompt (RelayedActEnv); the
// verb passes it only while the person has relaying on (iam's one rule,
// person.AcceptRelayed). The plain program tools keep refusing these acts and
// never pass a relayed act on.

// RelayedActEnv carries the relayed act into the verb's process
// (person.RelayedActEnv on the other side).
const RelayedActEnv = "KTW_RELAYED_ACT"

// MetaRequiresUserInteraction makes Claude Code ask the person on every call.
const MetaRequiresUserInteraction = "anthropic/requiresUserInteraction"

// act is one of the person's acts as a tool: its arguments and the command
// line it runs, with the key of the act a confirmation binds to.
type act struct {
	name, description string
	props             map[string]map[string]any
	required          []string
	// line builds the command line and the act's key from the tool's
	// arguments and the person's handle.
	line func(a map[string]string, person string) (args []string, key string, err error)
	// program runs it; empty is ktw-agents, whose verbs take the act relayed.
	// Another program's act is an operation the channel's allow-list keeps
	// from the program tools (stopping a universe): the person allowing the
	// call is its confirmation, and its fixed command line is not checked
	// against the allow-list - nothing is relayed.
	program string
}

// prover is a way of confirming a relayed act beyond the prompt and an
// authenticator code, served by another program: a build adds its provers at
// init (provers). prove answers the proof the relay carries, or the tool
// result to answer instead - the code to show the person, or why the proof
// failed, the act not run.
type prover struct {
	mode  string // the confirm value
	doc   string // its words in the confirm description
	prove func(s *Server, a act, key, person, proof string) (string, map[string]any)
}

// provers are the confirm modes added to prompt and code; none in the open
// source build.
var provers []prover

// proverNamed returns the prover of a confirm mode.
func proverNamed(mode string) (prover, bool) {
	for _, p := range provers {
		if p.mode == mode {
			return p, true
		}
	}
	return prover{}, false
}

// orList joins words as "a, b or c".
func orList(words []string) string {
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " or " + words[len(words)-1]
}

// confirmModes are the confirm values a relayed act takes.
func confirmModes() []string {
	modes := []string{"prompt", "code"}
	for _, p := range provers {
		modes = append(modes, p.mode)
	}
	return modes
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func enum(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": values}
}

func byPerson(person string) string {
	b, _ := json.Marshal(map[string]string{"id": person, "kind": "person"})
	return string(b)
}

// acts are the person's acts, in the order their tools are listed.
var acts = []act{
	{name: "decide", description: "Decide an open decision as the person: choose one of its options, saying why. Claude Code asks the person to allow it first.",
		props:    map[string]map[string]any{"decision": str("the decision's id"), "option": str("the option chosen, one of its labels"), "why": str("why this option")},
		required: []string{"decision", "option"},
		line: func(a map[string]string, person string) ([]string, string, error) {
			in, _ := json.Marshal(map[string]any{"id": a["decision"], "selected": []string{a["option"]}, "rationale": a["why"],
				"actor": map[string]string{"id": person, "kind": "person"}})
			return []string{"action", "run", "--uri", "ktw-methods:decisions/resolve", "--input", string(in)}, "decide:" + a["decision"], nil
		}},
	{name: "answer-inbox", description: "Answer an Inbox entry as the person: approve or reject a verdict (a rejection says what to change), give a text answer, or pick a choice's option. Claude Code asks the person to allow it first.",
		props: map[string]map[string]any{"entry": str("the Inbox entry's id"), "decision": enum("approved or rejected", "approved", "rejected"),
			"note": str("the answer's text: what to change for a rejection, the answer for a question"), "choice": str("for a choice: the option chosen")},
		required: []string{"entry"},
		line: func(a map[string]string, person string) ([]string, string, error) {
			args := []string{"inbox", "answer", "--id", a["entry"], "--by", person}
			for _, f := range []string{"decision", "note", "choice"} {
				if a[f] != "" {
					args = append(args, "--"+f, a[f])
				}
			}
			return args, "answer:" + a["entry"], nil
		}},
	{name: "dismiss-inbox", description: "Dismiss an Inbox entry as the person. Claude Code asks the person to allow it first.",
		props: map[string]map[string]any{"entry": str("the Inbox entry's id")}, required: []string{"entry"},
		line: func(a map[string]string, _ string) ([]string, string, error) {
			return []string{"inbox", "dismiss", "--id", a["entry"]}, "dismiss:" + a["entry"], nil
		}},
	{name: "report-todo", description: "Report one of the person's to-dos as the person: approve or send back a validation, or give a decision or what was asked. Claude Code asks the person to allow it first.",
		props: map[string]map[string]any{"todo": str("the to-do's id"), "summary": str("one line: what the person says"),
			"verdict": enum("for a validation: approved or rejected", "approved", "rejected"), "answer": str("for a decision or a provide step: the answer"),
			"status": enum("success (default) or fail or blocked", "success", "fail", "blocked")},
		required: []string{"todo", "summary"},
		line: func(a map[string]string, person string) ([]string, string, error) {
			status := a["status"]
			if status == "" {
				status = "success"
			}
			args := []string{"todos", "report", "--id", a["todo"], "--by", byPerson(person), "--status", status, "--summary", a["summary"]}
			for _, f := range []string{"verdict", "answer"} {
				if a[f] != "" {
					args = append(args, "--"+f, a[f])
				}
			}
			return args, "report:" + a["todo"], nil
		}},
	{name: "test-by-hand", description: "Record the person's hand test of a deliverable: it passes (what they saw) or fails (how). Claude Code asks the person to allow it first.",
		props: map[string]map[string]any{"request": str("the request's id"), "deliverable": str("the deliverable's title"),
			"result": enum("passes or fails", "passes", "fails"), "note": str("what was seen, or how it fails")},
		required: []string{"request", "deliverable", "result"},
		line: func(a map[string]string, person string) ([]string, string, error) {
			return []string{"requests", "test", "--id", a["request"], "--deliverable", a["deliverable"], "--result", a["result"],
				"--note", a["note"], "--by", byPerson(person)}, "test:" + a["request"] + "#" + a["deliverable"], nil
		}},
	{name: "verify-by-hand", description: "Record a deliverable verified by the person by hand, with what they saw. Claude Code asks the person to allow it first.",
		props:    map[string]map[string]any{"request": str("the request's id"), "deliverable": str("the deliverable's title"), "evidence": str("what the person saw")},
		required: []string{"request", "deliverable", "evidence"},
		line: func(a map[string]string, person string) ([]string, string, error) {
			return []string{"requests", "verify", "--id", a["request"], "--deliverable", a["deliverable"], "--evidence", a["evidence"],
				"--by", byPerson(person)}, "verify:" + a["request"] + "#" + a["deliverable"], nil
		}},
	{name: "hold-item", description: "Put one item of the person's queue on hold, with why. Claude Code asks the person to allow it first.",
		props: map[string]map[string]any{"key": str("the item's key in the person's queue"), "reason": str("why it waits")}, required: []string{"key"},
		line: func(a map[string]string, person string) ([]string, string, error) {
			return []string{"requests", "hold", "--key", a["key"], "--reason", a["reason"], "--by", byPerson(person)}, "hold:" + a["key"], nil
		}},
	{name: "resume-item", description: "Take an item on hold back to the person's queue. Claude Code asks the person to allow it first.",
		props: map[string]map[string]any{"key": str("the item's key")}, required: []string{"key"},
		line: func(a map[string]string, person string) ([]string, string, error) {
			return []string{"requests", "unhold", "--key", a["key"], "--by", byPerson(person)}, "unhold:" + a["key"], nil
		}},
	{name: "waive-module-update", description: "Waive a request's module update as the person, saying why its module needs none. Claude Code asks the person to allow it first.",
		props: map[string]map[string]any{"request": str("the request's id"), "note": str("why its module needs no update")}, required: []string{"request", "note"},
		line: func(a map[string]string, person string) ([]string, string, error) {
			return []string{"requests", "waive", "--id", a["request"], "--note", a["note"], "--by", byPerson(person)}, "waive:" + a["request"], nil
		}},
	{name: "stop-universe", program: "katyoti", description: "Stop a running universe - its hub and the members it spawned - for the person, to restart it with `katyoti universe start <name>`. Claude Code asks the person to allow it first; the program tool refuses it.",
		props: map[string]map[string]any{"universe": str("the universe's name")}, required: []string{"universe"},
		line: func(a map[string]string, _ string) ([]string, string, error) {
			return []string{"universe", "stop", a["universe"]}, "stop-universe:" + a["universe"], nil
		}},
}

func actNamed(name string) (act, bool) {
	for _, a := range acts {
		if a.name == name {
			return a, true
		}
	}
	return act{}, false
}

// actTools are the act tools, served when ktw-agents is: each asks the person.
func (s *Server) actTools() []map[string]any {
	if _, ok := s.exe["ktw-agents"]; !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(acts))
	for _, a := range acts {
		if prog := a.program; prog != "" && s.exe[prog] == "" {
			continue
		}
		props := map[string]any{}
		for k, v := range a.props {
			props[k] = v
		}
		if a.program == "" { // a relayed act: confirmed at the prompt, or proven
			docs := []string{"prompt (the default: Claude Code's prompt)", "code (a code from their authenticator app, after the prompt)"}
			var proven []string
			for _, p := range provers {
				docs = append(docs, p.mode+" ("+p.doc+")")
				proven = append(proven, p.mode)
			}
			props["confirm"] = enum("how the person confirms it: "+orList(docs), confirmModes()...)
			props["code"] = str("code only, where the client shows no form: the 6 digits the person read from their authenticator app")
			if len(proven) > 0 {
				props["proof"] = str(orList(proven) + " only, on the second call: the proof the first call answered")
			}
		}
		out = append(out, map[string]any{
			"name":        a.name,
			"description": a.description,
			"inputSchema": map[string]any{"type": "object", "properties": props, "required": a.required},
			"_meta":       map[string]any{MetaRequiresUserInteraction: true},
		})
	}
	return out
}

// callAct runs one of the person's acts, confirmed at the prompt, as the
// person: the channel's allow-list still applies; the person's acts rule does
// not, this being their door.
func (s *Server) callAct(a act, raw map[string]any) map[string]any {
	in := map[string]string{}
	for k, v := range raw {
		if t, ok := v.(string); ok {
			in[k] = strings.TrimSpace(t)
		}
	}
	for _, r := range a.required {
		if in[r] == "" {
			return toolResult(fmt.Sprintf("%s: %s is required", a.name, r), true)
		}
	}
	person, err := s.person()
	if err != nil {
		return toolResult(fmt.Sprintf("%s: who the person is cannot be read: %v", a.name, err), true)
	}
	args, key, err := a.line(in, person)
	if err != nil {
		return toolResult(err.Error(), true)
	}
	if a.program != "" {
		if s.exe[a.program] == "" {
			return toolResult(fmt.Sprintf("%s: %s is not served here", a.name, a.program), true)
		}
		return s.run(a.program, args)
	}
	if why := s.allowListRefusal("ktw-agents", parseLine(args)); why != "" {
		return toolResult(why, true)
	}
	relay := map[string]string{"person": person, "confirmed": "prompt", "act": key, "at": time.Now().UTC().Format(time.RFC3339)}
	switch in["confirm"] {
	case "", "prompt":
	case "code":
		code := in["code"]
		if code == "" {
			if !s.canElicit() {
				return toolResult(fmt.Sprintf("%s: ask the person for the code their authenticator app shows for KaTyoti, then call %s again with the same arguments and code - this client shows no form", a.name, a.name), true)
			}
			got, err := s.askCode(key)
			if err != nil {
				return toolResult(fmt.Sprintf("%s: no code - %v; the act was not run", a.name, err), true)
			}
			code = got
		}
		relay["confirmed"], relay["proof"] = "totp", code
	default:
		p, ok := proverNamed(in["confirm"])
		if !ok {
			return toolResult(fmt.Sprintf("%s: confirm is %s, not %q", a.name, orList(confirmModes()), in["confirm"]), true)
		}
		proof, res := p.prove(s, a, key, person, in["proof"])
		if res != nil {
			return res // the code to show, or why the proof failed
		}
		relay["confirmed"], relay["proof"] = p.mode, proof
	}
	relayJSON, _ := json.Marshal(relay)
	return s.run("ktw-agents", args, RelayedActEnv+"="+string(relayJSON))
}

// actionValue runs `ktw-agents action run --uri uri --input in --out json` and
// decodes its value into out.
func (s *Server) actionValue(uri string, in any, out any) error {
	raw, _ := json.Marshal(in)
	res := s.run("ktw-agents", []string{"action", "run", "--uri", uri, "--input", string(raw), "--out", "json"})
	text := res["content"].([]map[string]any)[0]["text"].(string)
	if isErr, _ := res["isError"].(bool); isErr {
		return fmt.Errorf("%s", strings.TrimSpace(text))
	}
	body := text
	if i := strings.Index(body, "\n"); i >= 0 && strings.HasPrefix(body, "exit: ") {
		body = body[i+1:]
	}
	if i := strings.Index(body, "--- stderr ---"); i >= 0 {
		body = body[:i]
	}
	var env struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal([]byte(body), &env); err != nil || len(env.Value) == 0 {
		return fmt.Errorf("unreadable answer: %s", strings.TrimSpace(body))
	}
	return json.Unmarshal(env.Value, out)
}

// person is the person using this installation: their profile handle, read
// once from `katyoti profile show --json`.
func (s *Server) person() (string, error) {
	s.personOnce.Do(func() {
		exe := filepath.Join(s.opts.Dir, "katyoti"+exeSuffix())
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "profile", "show", "--json")
		cmd.Dir = workDir()
		// The profile is the installation's, not a universe's: a universe's
		// data or workspace set for the session would send it astray.
		for _, kv := range os.Environ() {
			if !strings.HasPrefix(kv, "KTW_DATA=") && !strings.HasPrefix(kv, "KTW_WORKSPACE=") {
				cmd.Env = append(cmd.Env, kv)
			}
		}
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			s.personErr = err
			return
		}
		var p struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		}
		if err := json.Unmarshal(out.Bytes(), &p); err != nil || p.ID == "" || p.Source == "placeholder" {
			s.personErr = fmt.Errorf("no profile set (katyoti profile init)")
			return
		}
		s.personID = p.ID
	})
	return s.personID, s.personErr
}

// personState holds the person's handle, read once.
type personState struct {
	personOnce sync.Once
	personID   string
	personErr  error
}
