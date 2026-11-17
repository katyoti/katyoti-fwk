// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/demosignoff/api"
)

func item(g api.SignoffGroup) cwmapi.Object {
	o := cwmapi.Object{ID: "demo.login", Type: api.ItemType, Name: "Login page"}
	SetGroup(&o, g)
	return o
}

var two = api.SignoffSettings{RequiredReviewers: 2}

// Only a reviewer's approval counts, once; the requirement is one at least.
func TestApprovalsCountReviewersOnce(t *testing.T) {
	g := api.SignoffGroup{Reviewers: []string{"alice", "bob"}, Approvals: []string{"alice", "alice", "mallory"}}
	if got := Approvals(g); len(got) != 1 || got[0] != "alice" {
		t.Fatalf("Approvals = %v, want [alice]", got)
	}
	if SignedOff(g, two) {
		t.Error("one approval of two reads as signed off")
	}
	if !SignedOff(g, api.SignoffSettings{}) {
		t.Error("an unset requirement is one approval")
	}
	g.Approvals = append(g.Approvals, "bob")
	if !SignedOff(g, two) {
		t.Error("two approvals of two do not read as signed off")
	}
}

// The before_update hook refuses an approved verdict short of its approvals,
// saying how many, and accepts every other write.
func TestCheckWriteRefusesAnEarlyApproval(t *testing.T) {
	g := api.SignoffGroup{Reviewers: []string{"alice", "bob"}, Approvals: []string{"alice"}, Verdict: api.VerdictApproved}
	got := CheckWrite(GroupOf(item(g)), two)
	if got.Accept || !strings.Contains(got.Reason, "needs 2 approval(s)") || !strings.Contains(got.Reason, "it has 1") {
		t.Fatalf("CheckWrite = %+v, want a refusal naming 2 needed and 1 given", got)
	}
	g.Approvals = append(g.Approvals, "bob")
	if got := CheckWrite(g, two); !got.Accept {
		t.Errorf("a complete sign-off refused: %s", got.Reason)
	}
	if got := CheckWrite(api.SignoffGroup{Verdict: api.VerdictChanges}, two); !got.Accept {
		t.Errorf("a changes verdict refused: %s", got.Reason)
	}
}

func TestLintFindings(t *testing.T) {
	if got := Lint(api.SignoffGroup{Reviewers: []string{"a"}}); got.Status != "ok" || len(got.Findings) != 0 {
		t.Errorf("a clean sign-off = %+v", got)
	}
	got := Lint(api.SignoffGroup{Verdict: api.VerdictChanges, Approvals: []string{"x"}})
	rules := map[string]bool{}
	for _, f := range got.Findings {
		rules[f.Rule] = true
	}
	for _, r := range []string{api.RuleNoReviewer, api.RuleChangesWithoutNotes, api.RuleApprovalNotReviewer} {
		if !rules[r] {
			t.Errorf("finding %s missing from %+v", r, got.Findings)
		}
	}
	if got.Status != "findings" {
		t.Errorf("status = %q", got.Status)
	}
}

func TestVerifyVerdicts(t *testing.T) {
	done := api.SignoffGroup{Reviewers: []string{"a", "b"}, Approvals: []string{"a", "b"}, Verdict: api.VerdictApproved, Notes: "ok"}
	cases := []struct {
		name string
		g    api.SignoffGroup
		rule string
		want string
	}{
		{"approved with its approvals", done, "", api.StatusPass},
		{"changes asked", api.SignoffGroup{Verdict: api.VerdictChanges}, "approvals", api.StatusFail},
		{"pending", api.SignoffGroup{Reviewers: []string{"a"}}, "", api.StatusUnproven},
		{"notes present", done, "notes", api.StatusPass},
		{"notes missing", api.SignoffGroup{}, "notes", api.StatusFail},
		{"unknown rule", done, "colour", api.StatusUnproven},
	}
	for _, c := range cases {
		if got := Verify(c.g, two, c.rule); got.Status != c.want {
			t.Errorf("%s: %+v, want %s", c.name, got, c.want)
		}
	}
}

// The board is a valid page of the DSL, with one act of each verb a page runs.
func TestBoardPageIsAValidPageWithEveryVerb(t *testing.T) {
	obj := item(api.SignoffGroup{Reviewers: []string{"alice", "bob"}, Approvals: []string{"alice"}, Verdict: api.VerdictPending})
	def := BoardPage("", obj, two, []string{"demo.timeout"}, []string{"t on_update: 1/2 approval(s), verdict pending"})
	if def.Kind != "page" {
		t.Fatalf("kind = %q", def.Kind)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("not a valid page: %v", err)
	}
	raw, _ := json.Marshal(def)
	s := string(raw)
	for _, want := range []string{
		`"chat":{"task":"` + api.SignoffRequestTask,
		`"navigate":{"object":"demo.timeout"`,
		`"create":{"type":"signoff-item"`,
		`"uri":"` + api.ApproveURI,
		`"reviewer":"${item.name}"`,
		`"uri":"` + api.ContestURI,
		`"uri":"` + api.WriteURI,
		api.ContestFormType, api.VerdictFormType, api.ReviewerRowType,
		"1 / 2", "1/2 approval(s)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the board lacks %s", want)
		}
	}
	var page map[string]any
	_ = json.Unmarshal(raw, &page)
	if err := render.ValidatePage(page["dslTree"]); err != nil {
		t.Errorf("the tree as decoded JSON: %v", err)
	}
}

// The board answers in the person's language: French when asked, English by
// default; the data (ids, the verdict, the log) as written.
func TestBoardPageAnswersInThePersonsLanguage(t *testing.T) {
	obj := item(api.SignoffGroup{Reviewers: []string{"alice"}, Verdict: api.VerdictPending})
	sections := func(lang string) string {
		def := BoardPage(lang, obj, two, []string{"demo.timeout"}, nil)
		if err := def.Validate(); err != nil {
			t.Fatalf("%q: %v", lang, err)
		}
		raw, _ := json.Marshal(def)
		if !strings.Contains(string(raw), `"demo.timeout"`) || !strings.Contains(string(raw), api.VerdictPending) {
			t.Errorf("%q: the data is kept as written", lang)
		}
		var titles []string
		var walk func(any)
		walk = func(v any) {
			switch x := v.(type) {
			case map[string]any:
				if x["kind"] == "section" {
					titles = append(titles, x["title"].(string))
				}
				for _, c := range x {
					walk(c)
				}
			case []any:
				for _, c := range x {
					walk(c)
				}
			}
		}
		var page map[string]any
		_ = json.Unmarshal(raw, &page)
		walk(page["dslTree"])
		sort.Strings(titles)
		return def.Title + ": " + strings.Join(titles, "|")
	}
	if got := sections("fr"); got != "Validation: Activité|Contester un autre élément|Contesté par|Relecteurs|Validation" {
		t.Errorf("in French: %s", got)
	}
	for _, lang := range []string{"", "en"} {
		if got := sections(lang); got != "Sign-off: Activity|Contest another item|Contested by|Reviewers|Sign-off" {
			t.Errorf("%q in English: %s", lang, got)
		}
	}
}
