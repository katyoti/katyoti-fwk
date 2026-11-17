// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE INCREMENT'S BLOCKERS TAB: its panel is the pane's fourth
// tab - after Increment, before Information -, its count on it, shown to
// every user (no display switch admits it: is retired); the tab
// draws the pane's head and a section loaded on its own, which draws `increments
// blockers <id>` - here the person's answer an increment waits on through its
// dependency, under K1 with who acts and the action opening it.
func TestAnIncrementsBlockersTabDrawsItsBlockers(t *testing.T) {
	t.Parallel()
	panels := filepath.Join("..", "contrib", "embed", "cwm")
	b, err := os.ReadFile(filepath.Join(panels, "object-panels", "increment-blockers.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`source: "ktw-agents:increments/pane-page"`, `count: "ktw-agents:increments/blocker-count"`, `tab: "blockers"`, `- "increment"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("increment-blockers.yaml: no %s", want)
		}
	}
	if strings.Contains(string(b), "when:") {
		t.Error("increment-blockers.yaml: the Blockers tab is filtered - it shows to every user")
	}
	if fr, err := os.ReadFile(filepath.Join(panels, "object-panels", "increment-blockers.i18n.yaml")); err != nil || !strings.Contains(string(fr), "name: Blocages") {
		t.Errorf("the tab's French name: %v %s", err, fr)
	}
	if typ, _ := os.ReadFile(filepath.Join(panels, "object-types", "increment.yaml")); !strings.Contains(string(typ), "increment-page, increment-blockers, increment-information,") {
		t.Error("the increment's pane does not list the Blockers tab")
	}

	ctx, cwm, req := world(t)
	_, numbering := incrementFeature(t, ctx, cwm, req, "Invoices are numbered", "Number the invoices")
	personStep(t, ctx, numbering, "give-numbering", api.InteractionProvide)
	mail, mailing := incrementFeature(t, ctx, cwm, req, "Invoices reach their customers by mail", "Mail the numbered invoices")
	if _, err := req.Depend(ctx, api.DependInput{ID: mailing, On: numbering}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, mailing, api.StepIntake).ID, session, schedapi.Result{Summary: "a build"})

	inc := increments(t, ctx)
	tab, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: mail.ID, Tab: "blockers"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(tab.DSLTree)
	for _, want := range []string{`"data-view":"increment-page-head"`, `"id":"blockers"`, `"tab":"blocker-list"`, `"round":"${selection.blockers_round}"`, `"load":{`, `"placeholder":{`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the Blockers tab has no %s: %s", want, raw)
		}
	}
	// It opens with no Recompute pressed: its section reads round 0.
	if data, _ := json.Marshal(tab.Data); string(data) != `{"selection":{"blockers_round":0}}` {
		t.Errorf("the Blockers tab's data: %s", data)
	}
	list, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: mail.ID, Tab: "blocker-list", Language: "fr", Round: 4})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(list.DSLTree)
	for _, want := range []string{"Cet incrément ne peut pas finir aujourd'hui.", "1 action vous revient", `"id":"blockers-recompute"`, `"value":5`, "Réparé automatiquement (0)", "K1 · Vos réponses", `"id":"blocker-K1-` + numbering + `"`, `"id":"blockers-next"`, `"object":"` + numbering + `"`, "Répondre", "Qui agit :", "K6 · Incidents"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the Blockers tab's section has no %s: %s", want, raw)
		}
	}
	if strings.Contains(string(raw), `"id":"blocker-K1-`+mailing+`"`) {
		t.Errorf("the held work %s is listed as a root of its own", mailing)
	}
}
