// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// NO STATE IS READ BACK FROM THE WORDS IT WAS WRITTEN IN:
// a stopped to-do's status, a blocked plan's kind and an incident's cause are
// typed fields, and stopOf, endRefused and incidentActs / personActKey read
// them - a text that says otherwise is not read.

// A stopped to-do is read by the report kept on it, not by its waits line:
// a line that says « blocked » on a failed report reads as failed, a line
// with no report reads as not stopped, and clearStop ends the stop whole.
func TestAStoppedToDoIsReadByItsTypedReport(t *testing.T) {
	failed := api.WorkTodo{Todo: schedapi.Todo{ID: "m.TD-1", State: schedapi.StateWaiting, Waits: "blocked: it waits on m.RQ-9",
		Result: &schedapi.Result{Status: schedapi.StatusFail, Summary: "the build broke\nits log follows"}}}
	if s, sum := stopOf(failed); s != schedapi.StatusFail || sum != "the build broke" {
		t.Errorf("a failed report under a « blocked » line: got %q %q, want fail and its summary's first line", s, sum)
	}
	words := api.WorkTodo{Todo: schedapi.Todo{ID: "m.TD-2", State: schedapi.StateWaiting, Waits: "fail: the build broke"}}
	if stopped(words) {
		t.Error("a waits line that says « fail: » with no report kept is read as stopped: the text is read")
	}
	ready := failed
	ready.State = schedapi.StateReady
	if stopped(ready) {
		t.Error("a to-do back in the queue reads as stopped: only a waiting one is")
	}
	ok := failed
	ok.Result = &schedapi.Result{Status: schedapi.StatusSuccess, Summary: "done"}
	if stopped(ok) {
		t.Error("a success kept on a waiting to-do reads as stopped")
	}
	clearStop(&failed)
	if stopped(failed) || failed.Waits != "" || failed.Result != nil {
		t.Errorf("clearStop leaves the stop: %+v", failed)
	}
}

// A blocked plan's actions come from its kind, its note saying nothing - or
// saying another kind's words: an end refused is offered the end's actions,
// a block by hand the person's own hold, whatever the note.
func TestABlockedPlansActionsComeFromItsKind(t *testing.T) {
	personBlock := cwmapi.LifecycleEntry{To: api.PlanBlocked, Transition: "block", By: cwmapi.Actor{ID: "p", Kind: api.ActorPerson}, Note: "its to-dos are done"}
	for _, c := range []struct {
		kind, note    string
		refused, hold bool
		expect        string
	}{
		{api.BlockEndRefused, "nothing", true, false, expectRefused},
		{api.BlockMoveRefused, "nothing", true, false, expectRefused},
		{api.BlockHandTestFailed, "its to-dos are done, but x is not verified", false, false, expectBlocked},
		{api.BlockParked, "its to-dos are done", false, false, expectBlocked},
		{api.BlockManual, "its to-dos are done, but x is not verified", false, true, expectBlocked},
		{"", "a hand test failed: x", false, true, expectBlocked}, // blocked before kinds: by hand
	} {
		pf := api.PlanFields{State: api.PlanBlocked, Block: c.kind, Note: c.note, Trail: []cwmapi.LifecycleEntry{personBlock}}
		if got := endRefused(pf); got != c.refused {
			t.Errorf("%q: endRefused %v, want %v", c.kind, got, c.refused)
		}
		if _, got := ownHold(&pf); got != c.hold {
			t.Errorf("%q: ownHold %v, want %v", c.kind, got, c.hold)
		}
		st := stand{plan: api.PlannedObject{ID: "m.RQ-1", PlanFields: pf}, state: stateLabels[api.PlanBlocked], why: c.note}
		if got := expected(st, nil, nil); got.Case != c.expect {
			t.Errorf("%q: expected case %q, want %q", c.kind, got.Case, c.expect)
		}
	}
	// Its kind is read only while it is blocked.
	if endRefused(api.PlanFields{State: api.PlanReady, Block: api.BlockEndRefused}) {
		t.Error("a plan no longer blocked reads as its end refused")
	}
}

// The person's act a K2 blocker calls for is its typed act, with an empty
// reason.
func TestAPersonsActIsReadFromItsTypedAct(t *testing.T) {
	for act, want := range map[string]string{
		api.ActAccept: "act_accept", api.ActPlan: "act_plan", api.ActUnblock: "act_unblock", api.InteractionValidate: "act_validate",
		api.InteractionChallenge: "act_challenge", api.InteractionDecide: "act_decide", "confirm": "act_confirm", "": "act_act", "odd": "act_act",
	} {
		if got := personActKey(api.Blocker{Kind: api.BlockerAct, Act: act}); got != want {
			t.Errorf("%q: %s, want %s", act, got, want)
		}
	}
}

// forbiddenTextReads are the reads of state from its words this package no
// longer makes, each with the files it is looked for
// in ("" = every file of the package). increment_blockers_say.go is allowed
// everywhere: its regexes put a reason in the page's language - presentation,
// no state is decided by them. The waits' own kinds (after, held by the
// review, a dependency) are still read by their prefix:.
var forbiddenTextReads = []struct {
	what  string
	re    *regexp.Regexp
	files string
}{
	{"a stop read from the waits line's « fail: » / « blocked: » prefix",
		regexp.MustCompile(`strings\.\w+\(\s*\w+\.Waits,\s*(s|schedapi\.Status\w+)\s*\)|range \[\]string\{schedapi\.StatusFail, schedapi\.StatusBlocked\}`), ""},
	{"a stop read from a waits value's literal « fail: » / « blocked: » prefix",
		regexp.MustCompile(`strings\.\w+\(\s*([\w.]*[Ww]aits?\w*|w)\s*,\s*"(fail|blocked):`), ""},
	{"an end refused read from its note (« its to-dos are done »)", regexp.MustCompile(`strings\.\w+\([^\n]*"its to-dos are done`), ""},
	{"an end refused read from its note (« is not verified »)", regexp.MustCompile(`strings\.\w+\([^\n]*"is not verified"`), ""},
	{"a failed hand test read from its note", regexp.MustCompile(`strings\.\w+\([^\n]*handTestFailed`), ""},
	{"an incident's or a K2 blocker's acts chosen by its reason's words", regexp.MustCompile(`strings\.\w+\(\s*(b\.)?[Rr]eason\b`), "increment_blockers_draw.go"},
}

// No file of the package reads a typed state back from its words; falsified
// by putting one read back (a prefix of the waits line in stopOf).
func TestNoStateIsReadFromTheWordsItWasWrittenIn(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	read := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "increment_blockers_say.go" {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		read++
		for _, p := range forbiddenTextReads {
			if p.files != "" && p.files != f {
				continue
			}
			for _, m := range p.re.FindAllString(string(raw), -1) {
				t.Errorf("%s: %s: %s", f, p.what, m)
			}
		}
	}
	if read < 50 {
		t.Fatalf("only %d files read: the guard reads nothing", read)
	}
}
