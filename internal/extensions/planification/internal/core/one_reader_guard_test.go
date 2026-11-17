// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// No view reads a stopped step from its wait's text but the one reader
// : outside blockers.go, nothing tests a to-do's Waits
// for the fail or blocked prefix - every view asks stopOf / stopped /
// stopStatus, and « À faire » asks the reader who moves it.
func TestNoViewClassifiesAStopFromItsWaitsText(t *testing.T) {
	text := regexp.MustCompile(`(HasPrefix|TrimPrefix|CutPrefix|Contains)\([^()]*Waits[^()]*schedapi\.Status(Fail|Blocked)`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == "blockers.go" || strings.HasSuffix(f, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if text.MatchString(line) {
				t.Errorf("%s:%d reads a stopped step from its wait's text - ask stopOf (blockers.go): %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// No view words a hold from a wait's text: in the
// files whose lines say what holds a request - the increment's summary and
// pane, a request's where-in-work, the demands, the release page, the request
// view and the deck - no line reads a to-do's or a queue entry's Waits; each
// asks the one reader (holdOf, viewStepHold) and words it through holdWords
// (blockers.go). The increment summary's own Waits lines are its output, not
// a wait's text: out.Waits where it is filled, s.Waits where it is drawn.
func TestNoViewWordsAHoldFromTheWaitText(t *testing.T) {
	waits := regexp.MustCompile(`([\w.\]\)]+)\.Waits\b`)
	output := map[string]string{"increment.go": "out", "release_page.go": "s"}
	views := []string{"increment.go", "increment_view.go", "increment_pane.go", "demands.go",
		"release_page.go", "request_view.go", "deck_stand.go", "deck.go"}
	for _, f := range views {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			for _, m := range waits.FindAllStringSubmatch(line, -1) {
				if m[1] == output[f] {
					continue
				}
				t.Errorf("%s:%d words a hold from a wait's text (%s) - ask the one reader and word it with holdWords (blockers.go): %s", f, i+1, m[0], strings.TrimSpace(line))
			}
		}
	}
	if raw, err := os.ReadFile("blockers.go"); err != nil || !strings.Contains(string(raw), "func holdWords(") {
		t.Errorf("blockers.go holds no holdWords, the one wording of a hold (err %v)", err)
	}
}
