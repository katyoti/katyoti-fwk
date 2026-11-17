// SPDX-License-Identifier: Apache-2.0

package guidelint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
)

const goodRecipe = `
name: Add a capability to my universe
description: Install a sidecar and wire it into this universe.
hashtags: [marketplace, add capability]
level: beginner
requires: [open-a-universe]
related: [ktw-agents:/skill/doc-create]
steps:
  - id: open
    say: Open the Marketplace from the ribbon.
    target: {guide: ribbon.marketplace}
    await: click
  - id: install
    say: Press {control:marketplace.install} on the capability you want, then {control:marketplace.tab|configsets}.
    target: {guide: marketplace.row, name: ktw-example}
  - id: wait
    say: Wait for the list to show.
    await: route
    route: marketplace
  - id: wired
    say: The capability is now wired.
    check: ktw-univers:guide-checks/sidecar-wired
`

func rules(r verify.LintResult) map[string]verify.LintSeverity {
	out := map[string]verify.LintSeverity{}
	for _, f := range r.Findings {
		out[f.Rule] = f.Severity
	}
	return out
}

func TestLintRecipeBytes_AGoodRecipeIsClean(t *testing.T) {
	r := LintRecipeBytes([]byte(goodRecipe))
	if r.Status != "pass" || len(r.Findings) != 0 {
		t.Fatalf("good recipe: %s %+v", r.Status, r.Findings)
	}
}

// Each rule, tripped by the smallest change to the good recipe. A rule removed
// from the lint makes its case fail here.
func TestLintRecipeBytes_EachRuleTrips(t *testing.T) {
	cases := []struct {
		rule, from, to string
		sev            verify.LintSeverity
	}{
		{"yaml-invalid", "name: Add", "name: [Add", verify.SeverityError},
		{"unknown-field", "level: beginner", "levle: beginner", verify.SeverityError},
		{"name-missing", "name: Add a capability to my universe", "name: ''", verify.SeverityError},
		{"description-missing", "description: Install a sidecar and wire it into this universe.", "", verify.SeverityWarn},
		{"hashtags-missing", "hashtags: [marketplace, add capability]", "", verify.SeverityWarn},
		{"level-invalid", "level: beginner", "level: novice", verify.SeverityError},
		{"requires-invalid", "requires: [open-a-universe]", "requires: [Open A Universe]", verify.SeverityError},
		{"related-invalid", "related: [ktw-agents:/skill/doc-create]", "related: [doc-create]", verify.SeverityError},
		{"video-invalid", "level: beginner", "level: beginner\nvideo: file://c/x.mp4", verify.SeverityError},
		{"step-unknown-field", "    await: click", "    await: click\n    wait: 1", verify.SeverityError},
		{"step-id-missing", "  - id: open\n    say", "  - say", verify.SeverityError},
		{"step-id-invalid", "  - id: open\n", "  - id: Open It\n", verify.SeverityError},
		{"step-id-duplicate", "  - id: install\n", "  - id: open\n", verify.SeverityError},
		{"step-say-missing", "    say: Open the Marketplace from the ribbon.", "    say: ''", verify.SeverityError},
		{"step-click-without-target", "    target: {guide: ribbon.marketplace}\n", "", verify.SeverityError},
		{"step-route-missing", "    route: marketplace\n", "", verify.SeverityError},
		{"step-await-invalid", "    await: click", "    await: hover", verify.SeverityError},
		{"step-target-empty", "{guide: ribbon.marketplace}", "{name: x}", verify.SeverityError},
		{"step-target-by-text", "{guide: ribbon.marketplace}", "{role: button, label: Marketplace}", verify.SeverityError},
		{"step-target-unknown-field", "{guide: ribbon.marketplace}", "{guide: ribbon.marketplace, kind: x}", verify.SeverityError},
		{"step-uri-invalid", "check: ktw-univers:guide-checks/sidecar-wired", "check: not a uri", verify.SeverityError},
		{"step-control-malformed", "    say: Open the Marketplace from the ribbon.", "    say: Open {control:Ribbon Marketplace} from the ribbon.", verify.SeverityError},
		{"step-control-by-label", "    say: Open the Marketplace from the ribbon.", "    say: Open the **Marketplace** from the ribbon.", verify.SeverityError},
		{"step-do-and-check", "    check: ktw-univers:guide-checks/sidecar-wired", "    check: ktw-univers:guide-checks/sidecar-wired\n    do: ktw-univers:guide-checks/wire", verify.SeverityError},
	}
	for _, c := range cases {
		t.Run(c.rule, func(t *testing.T) {
			if !strings.Contains(goodRecipe, c.from) {
				t.Fatalf("fixture no longer contains %q", c.from)
			}
			got := rules(LintRecipeBytes([]byte(strings.Replace(goodRecipe, c.from, c.to, 1))))
			if sev, ok := got[c.rule]; !ok || sev != c.sev {
				t.Errorf("rule %s: got %v", c.rule, got)
			}
		})
	}
}

// A guide is a Help me (steps), a Show me (a video, no steps) or an Explain
// me (a markdown body alone): each kind is accepted, an empty one refused.
func TestLintRecipeBytes_EachKindIsAGuideAndAnEmptyOneIsRefused(t *testing.T) {
	head := "name: x\ndescription: y\nhashtags: [z]\n"
	for name, body := range map[string]string{
		"help me":    "steps:\n  - id: one\n    say: Do this.\n",
		"show me":    "video: https://example.org/v.mp4\nbody: What the video shows.\n",
		"explain me": "body: |\n  A software factory is the workshop that builds your applications.\n",
	} {
		if got := rules(LintRecipeBytes([]byte(head + body))); got["guide-empty"] != "" || got["unknown-field"] != "" {
			t.Errorf("%s: %v; want it accepted", name, got)
		}
	}
	if got := rules(LintRecipeBytes([]byte(head + "steps: []\n"))); got["guide-empty"] != verify.SeverityError {
		t.Errorf("an empty guide: %v; want guide-empty", got)
	}
}

func TestLintRecipeBytes_TooLong(t *testing.T) {
	long := "name: x\ndescription: y\nhashtags: [z]\nsteps:\n"
	for i := 0; i < 13; i++ {
		long += fmt.Sprintf("  - id: s%d\n    say: a step.\n", i)
	}
	if got := rules(LintRecipeBytes([]byte(long))); got["too-long"] != verify.SeverityWarn {
		t.Errorf("13 steps: %v", got)
	}
}

// okDetail is the output schema of a method answering {ok, detail}.
var okDetail = json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"},"detail":{"type":"string"}}}`)

// TestRecipeChecksAreReadOnly is the G-002 probe of modules/guide: a recipe's
// check must name a method that declares itself read-only, answers at once and
// says `ok` and `detail`. Every other shape is refused, one case at a time,
// and the good one passes — so the test cannot pass by refusing everything.
func TestRecipeChecksAreReadOnly(t *testing.T) {
	recipe, err := guideapi.ParseRecipe([]byte(goodRecipe))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		action Action
		err    error
		rule   string
	}{
		{"read-only sync ok/detail", Action{ReadOnly: true, OutputSchema: okDetail}, nil, ""},
		{"mutating", Action{ReadOnly: false, OutputSchema: okDetail}, nil, "check-not-read-only"},
		{"asynchronous", Action{ReadOnly: true, IsAsync: true, OutputSchema: okDetail}, nil, "check-async"},
		{"no ok/detail", Action{ReadOnly: true, OutputSchema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"string"}}}`)}, nil, "check-output"},
		{"no output schema", Action{ReadOnly: true}, nil, "check-output"},
		{"not published", Action{}, errors.New("no such method"), "check-unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var asked []string
			describe := func(_ context.Context, uri string) (Action, error) {
				asked = append(asked, uri)
				return c.action, c.err
			}
			res := LintChecks(context.Background(), recipe, describe)
			if len(asked) != 1 || asked[0] != "ktw-univers:guide-checks/sidecar-wired" {
				t.Fatalf("described %v, want the recipe's one check", asked)
			}
			got := rules(res)
			if c.rule == "" {
				if res.Status != "pass" {
					t.Fatalf("good check refused: %+v", res.Findings)
				}
				return
			}
			if got[c.rule] != verify.SeverityError || res.Status != "fail" {
				t.Errorf("want %s error, got %s %v", c.rule, res.Status, got)
			}
		})
	}
}

// A tour names no check, so nothing is described and nothing is refused.
func TestLintChecks_ATourHasNothingToCheck(t *testing.T) {
	tour := guideapi.Recipe{Name: "Tour", Steps: []guideapi.Step{{ID: "look", Say: "Look here."}}}
	res := LintChecks(context.Background(), tour, func(context.Context, string) (Action, error) {
		t.Fatal("a tour described an action")
		return Action{}, nil
	})
	if res.Status != "pass" {
		t.Fatalf("tour: %+v", res)
	}
}

// A guide shown once at start names its place in the queue; a negative one is
// refused, a once_key without it is a warning.
func TestLintRecipeBytes_ShownOnce(t *testing.T) {
	head := "name: x\ndescription: y\nhashtags: [z]\nbody: Welcome.\n"
	if got := rules(LintRecipeBytes([]byte(head + "shown_once: 1\nonce_key: \"1.006\"\n"))); len(got) != 0 {
		t.Errorf("a welcome shown once: %v; want it clean", got)
	}
	if got := rules(LintRecipeBytes([]byte(head + "shown_once: -1\n"))); got["shown-once-negative"] != verify.SeverityError {
		t.Errorf("shown_once -1: %v", got)
	}
	if got := rules(LintRecipeBytes([]byte(head + "once_key: v2\n"))); got["once-key-unused"] != verify.SeverityWarn {
		t.Errorf("once_key alone: %v", got)
	}
}
