// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// The number of slides asked is exact: a deck task never takes it as an aim,
// one slide asked is possible (and has no cover), and a content template that
// needs more slides than asked is the user's choice, asked before the deck is
// written — never settled by the agent.
func TestDeckTasksKeepTheSlideCountAsked(t *testing.T) {
	var input struct {
		Types map[string]struct {
			Fields map[string]struct {
				Description string   `yaml:"description"`
				Min         *float64 `yaml:"min"`
			} `yaml:"fields"`
		} `yaml:"types"`
	}
	data, err := embeddedFS.ReadFile("embed/tasks/slidedeck-fast-create.input.dsl.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := config.DecodeBytes(data, marshal.YAML, &input); err != nil {
		t.Fatal(err)
	}
	slides := input.Types["SlidedeckFastCreateInput"].Fields["slides"]
	if slides.Min == nil || *slides.Min != 1 {
		t.Errorf("the form refuses one slide: slides.min = %v, want 1", slides.Min)
	}
	if strings.Contains(slides.Description, "Environ") {
		t.Errorf("the form states the count as an aim: %q", slides.Description)
	}

	for task, wants := range map[string][]string{
		"slidedeck-fast-create": {
			"EXACTLY that many slides",
			"ONE slide asked is one\n  content slide, with no cover",
			"only when `slides` is 2 or more",
			"Ask ONE `ask-user` question, before writing the\n  deck",
			"neither exceed `slides` nor merge or\n  drop sections silently",
		},
		"deck-from-object": {
			"a hard\nmaximum, never exceeded",
			"Ask ONE `ask-user` question\n  before writing the deck",
		},
	} {
		body, err := embeddedFS.ReadFile("embed/tasks/" + task + ".md")
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ReplaceAll(string(body), "\r\n", "\n")
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not say %q", task, want)
			}
		}
		for _, aim := range []string{"about that many slides", "About one slide per minute"} {
			if strings.Contains(text, aim) {
				t.Errorf("%s still takes the count as an aim: %q", task, aim)
			}
		}
	}
}
