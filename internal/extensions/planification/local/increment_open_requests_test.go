// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// AN INCREMENT'S PAGE OPENS ON ITS OPEN REQUESTS (view 4
// of delivery-views-v2-mockup.html): its tabs are Open requests, Goals,
// Increment, Information and History; its head counts done, running, on you and to do; the Open
// requests tab lists each open request with its step dots and its step and
// who, in the Running tab's order, then « n done requests » folded - never a
// dropped one. No panel waits on a switch: the pane is these
// tabs alone, the Blockers tab among them while IN-056's switch admits it.
func TestAnIncrementsPageOpensOnItsOpenRequests(t *testing.T) {
	long.Test(t)
	t.Parallel()
	cwmDir := filepath.Join("..", "contrib", "embed", "cwm")
	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(cwmDir, rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	for _, panel := range []string{"increment-open-requests", "increment-page", "increment-information", "increment-journal"} {
		if strings.Contains(read("object-panels/"+panel+".yaml"), "in062") {
			t.Errorf("%s still waits on IN-062's switch", panel)
		}
	}
	if s := strings.ReplaceAll(read("object-types/increment.yaml"), "\r\n", "\n"); !strings.Contains(s, "  tabs: [increment-open-requests, increment-goals, increment-page, increment-blockers, increment-information, increment-journal]\n  only: true\n") ||
		strings.Contains(s, "variants:") {
		t.Error("the increment's pane is not Open requests, Goals, Increment, (Blockers,) Information, History, with no variant")
	}
	if strings.Contains(read("capabilities/release.yaml"), "in062") {
		t.Error("the release capability still declares in062()")
	}
	for _, gone := range []string{"increment", "increment-requests", "increment-dependencies", "increment-proposal", "increment-history"} {
		if _, err := os.Stat(filepath.Join(cwmDir, "object-panels", gone+".yaml")); err == nil {
			t.Errorf("the former pane's %s panel is still there", gone)
		}
	}
	for f, name := range map[string]string{"increment-open-requests": "Demandes ouvertes", "increment-page": "Incrément", "increment-information": "Informations", "increment-journal": "Historique"} {
		if !strings.Contains(read("object-panels/"+f+".i18n.yaml"), "name: "+name) {
			t.Errorf("%s: its French name is not %s", f, name)
		}
	}

	w := runningIncrementsWorld(t)
	inc := increments(t, w.ctx)
	tab := func(tab, lang string) (string, map[string]any) {
		t.Helper()
		def, err := inc.PanePage(w.ctx, api.IncrementPaneInput{ID: w.built, Tab: tab, Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the %s tab must display: %v", tab, err)
		}
		b, _ := json.Marshal(def.DSLTree)
		var tree map[string]any
		_ = json.Unmarshal(b, &tree)
		return string(b), tree
	}
	head, _ := tab("open-requests", "")
	for _, want := range []string{
		`"data-view":"increment-open-requests"`, `"data-view":"increment-page-head"`, `"content":"` + w.built + `"`,
		`"content":"Invoices get paid"`, `"text":"running"`, `"text":"P2"`, `"text":"ships in 1.1.0"`, `"kind":"gauge"`,
		`"content":"1 done · 1 running · 1 on you · 4 to do"`,
		`"kind":"box"`, `"load":{`, `"tab":"open-request-list"`, `"placeholder":{`,
	} {
		if !strings.Contains(head, want) {
			t.Errorf("the Open requests tab lacks %s: %s", want, head)
		}
	}
	if strings.Contains(head, `"data-view":"increment-actions"`) || strings.Contains(head, "Where each goal stands") {
		t.Errorf("the Open requests tab draws the actions or the goals: %s", head)
	}

	list, tree := tab("open-request-list", "")
	for _, want := range []string{
		`"text":"Open requests (6)"`, `"kind":"stepper"`,
		`"data-step-who":"intake · session-b"`, `"data-step-who":"go · on you"`,
		`"kind":"accordion"`, `"data-view":"increment-done-requests"`, `"label":"1 done"`, `"data-group":"running"`, `"data-group":"on-you"`,
		`"data-done-request":"` + w.reqs["done"] + `"`,
	} {
		if !strings.Contains(list, want) {
			t.Errorf("the open requests lack %s: %s", want, list)
		}
	}
	// The groups open at first, the done requests folded (the done fold's
	// keys, in order, hold no defaultOpen before its panels).
	if i := strings.Index(list, `"data-view":"increment-done-requests"`); i < 0 || strings.Contains(list[i:i+strings.Index(list[i:], `"panels"`)], `"defaultOpen"`) {
		t.Errorf("the done requests are not folded: %s", list)
	}
	if strings.Contains(list, w.reqs["dropped"]) {
		t.Errorf("a dropped request is listed: %s", list)
	}
	var rows []string
	for _, l := range (runningTab{tree: tree}).lines() {
		if strings.HasPrefix(l, "RQ:") {
			rows = append(rows, strings.SplitN(l, ":", 3)[1])
		}
	}
	want := []string{w.reqs["running"], w.reqs["on-you"], w.reqs["failed"], w.reqs["blocked"], w.reqs["todo-a"], w.reqs["todo-b"]}
	if strings.Join(rows, " ") != strings.Join(want, " ") {
		t.Errorf("the open requests:\n got %v\nwant %v (running, on you, failed, blocked, then the rest by id; never the done one)", rows, want)
	}

	if fr, _ := tab("open-request-list", "fr"); !strings.Contains(fr, `"text":"Demandes ouvertes (6)"`) || !strings.Contains(fr, `"label":"1 faite"`) || !strings.Contains(fr, `"data-step-who":"go · à vous"`) {
		t.Errorf("the French open requests: %s", fr)
	}
	if fr, _ := tab("open-requests", "fr"); !strings.Contains(fr, `"content":"1 faites · 1 en cours · 1 à vous · 4 à faire"`) {
		t.Errorf("the French head: %s", fr)
	}

	// Every tab draws the page's head; the Increment tab carries what a
	// person manages, its proposal and its dependencies.
	if goals, _ := tab("goals", ""); !strings.Contains(goals, `"data-view":"increment-page-head"`) || strings.Contains(goals, `"data-view":"increment-head"`) {
		t.Errorf("the Goals tab does not draw the page's head: %s", goals)
	}
	page, _ := tab("page", "")
	for _, want := range []string{`"data-view":"increment-page"`, `"data-view":"increment-page-head"`, `"data-view":"increment-actions"`, `"data-view":"increment-more"`, `"text":"Its proposal"`, `"text":"What it depends on"`, `"data-view":"increment-unlinked"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the Increment tab lacks %s: %s", want, page)
		}
	}
}

// THE OPEN REQUESTS TAB'S BADGE: increments
// open-request-count answers the count of the page's « Open requests (n) » -
// running, on you and to do, never a done or dropped one - in orange.
func TestAnIncrementsOpenRequestCountIsItsPagesHeading(t *testing.T) {
	long.Test(t)
	t.Parallel()
	if s, err := os.ReadFile(filepath.Join("..", "contrib", "embed", "cwm", "object-panels", "increment-open-requests.yaml")); err != nil ||
		!strings.Contains(string(s), `count: "ktw-agents:increments/open-request-count"`) {
		t.Errorf("the Open requests panel declares no count ktw-agents:increments/open-request-count (%v)", err)
	}
	w := runningIncrementsWorld(t)
	inc := increments(t, w.ctx)
	def, err := inc.PanePage(w.ctx, api.IncrementPaneInput{ID: w.built, Tab: "open-request-list"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(def.DSLTree)
	heading := 0
	if _, after, ok := strings.Cut(string(b), `"text":"Open requests (`); ok {
		fmt.Sscanf(after, "%d", &heading)
	}
	if heading == 0 {
		t.Fatalf("the fixture's page has no « Open requests (n) » heading: %s", b)
	}
	c, err := inc.OpenRequestCount(w.ctx, api.IncrementPaneInput{ID: w.built, Tab: "open-requests"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Count != heading || c.Color != "orange" {
		t.Errorf("open-request-count = %+v, want {count: %d (the heading's), color: orange}", c, heading)
	}
}
