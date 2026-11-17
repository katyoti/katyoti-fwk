// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// The pane's reason is what a person reads: the innermost message, without the
// dispatcher's URI plumbing, the guard prefix, or the expression's caret.
func TestHumanReason(t *testing.T) {
	cases := []struct{ in, want string }{
		{
			in:   "the guard \"gate('ready')\": ActionService.Run ktw-agents:/gates/gate-function (in-process): gates: no gate named \"ready\" (1:1)\n | gate('ready')\n | ^",
			want: "gates: no gate named \"ready\"",
		},
		{
			in:   "the guard \"verify() && x\": unknown name x (1:13)\n | verify() && x\n | ............^",
			want: "unknown name x",
		},
		{
			in:   "the guard \"gate('a')\" does not hold (gate(\"a\"): ActionService.Run ktw-agents:/gates/gate-function (in-process): boom)",
			want: "the guard \"gate('a')\" does not hold (gate(\"a\"): boom)",
		},
		{in: "cwm: not found: object \"x\"", want: "cwm: not found: object \"x\""},
	}
	for _, c := range cases {
		if got := humanReason(c.in); got != c.want {
			t.Errorf("humanReason(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

// A gate's checklist is drawn item by item under the option, so a refusal that
// only repeats it shows no reason; another function's answer keeps a readable
// one, never a structured answer inlined.
func TestARefusalTheChecklistDrawsShowsNoReason(t *testing.T) {
	checklist := map[string]any{"gate": "ready-to-test", "passes": false, "items": []any{
		map[string]any{"name": "criteria", "status": "unmet", "reason": "no criterion"},
	}}
	gate := cwmapi.FunctionAnswer{Function: "gate", Args: []any{"ready-to-test"}, Answer: checklist}
	guard := "gate('ready-to-test')"
	cases := []struct {
		name  string
		calls []cwmapi.FunctionAnswer
		want  string
	}{
		{"the checklist alone", []cwmapi.FunctionAnswer{gate}, ""},
		{"the checklist and another function", []cwmapi.FunctionAnswer{gate,
			{Function: "verify", Answer: false}},
			"the guard \"gate('ready-to-test')\" does not hold (verify(): false)"},
		{"a structured answer of another function", []cwmapi.FunctionAnswer{gate,
			{Function: "lint", Args: []any{"x"}, Answer: map[string]any{"findings": []any{1}}}},
			"the guard \"gate('ready-to-test')\" does not hold (lint(\"x\"))"},
		{"no checklist", []cwmapi.FunctionAnswer{{Function: "verify", Answer: false}},
			"the guard \"verify()\" does not hold (verify(): false)"},
	}
	for _, c := range cases {
		g := guard
		if c.name == "no checklist" {
			g = "verify()"
		}
		reason := fmt.Sprintf("the guard %q does not hold (%s: %s)", g, "gate(\"ready-to-test\")", `{"gate":"ready-to-test","items":[{"name":"criteria"}]}`)
		if c.name == "no checklist" {
			reason = "the guard \"verify()\" does not hold (verify(): false)"
		}
		refused := &cwmapi.TransitionRefusedError{Guard: g, Reason: reason, Calls: c.calls}
		got := refusalReason(refused, humanReason(reason))
		if got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
		if strings.Contains(got, "{") {
			t.Errorf("%s: the reason inlines a structured answer: %q", c.name, got)
		}
	}
}
