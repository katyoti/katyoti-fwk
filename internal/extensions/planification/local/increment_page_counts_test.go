// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN INCREMENT PAGE SAYS WHERE IT STANDS AT A GLANCE (feature):
// its Open requests tab says its progress once (one bar of its requests and
// its counts), in one wording (« awaiting a person »), and no id as a label:
// every object by its title.
func TestAnIncrementPageSaysItsProgressOnceAndNamesItByItsTitle(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers")
	target := func(title string) string {
		t.Helper()
		uc := usecase(t, ctx, cwm, title)
		if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: uc, Increment: in.ID}); err != nil {
			t.Fatal(err)
		}
		return uc
	}
	mail, csv := target("Mail the invoice"), target("Export to CSV")
	target("Print the invoice")
	shared := file(t, ctx, req, api.FileInput{Title: "One sender for every channel", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Assess(ctx, api.AssessInput{ID: shared.ID, Serves: mail, By: intake}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: shared.ID, To: csv, Kind: api.EdgeServes}); err != nil {
		t.Fatal(err)
	}

	def, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: "open-requests"})
	if err != nil {
		t.Fatal(err)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("the increment's pane must display: %v", err)
	}
	b, _ := json.Marshal(def.DSLTree)
	raw := string(b)
	var tree any
	_ = json.Unmarshal(b, &tree)
	if !strings.Contains(raw, `"data-view":"increment-page-counts"`) || strings.Count(raw, `"kind":"gauge"`) != 1 {
		t.Error("its progress is not said once: one bar of its requests and its counts")
	}
	if regexp.MustCompile(`\d+ of \d+ goals and features done`).MatchString(raw) {
		t.Error("its progress is said twice: the requests bar and a text")
	}
	if strings.Contains(raw, "Awaiting approval") {
		t.Error("the page says « Awaiting approval »: one wording, « Awaiting a person »")
	}
	id := regexp.MustCompile(`[a-z-]+\.(IN|RQ|RL|SM|UC|F|G)-\d+`)
	var labels func(n any)
	labels = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			// Every object is named by its title: an id shows only in its own
			// id badge (mono) or after a title in brackets.
			for _, k := range []string{"content", "text", "title", "meta", "label"} {
				if s, ok := v[k].(string); ok && id.MatchString(s) && v["mono"] != true && !idBesideTitle(id, s) {
					t.Errorf("an id shows as a label: %s %q", k, s)
				}
			}
			for k, c := range v {
				if k != "navigate" && k != "action" && k != "args" && k != "submit" {
					labels(c)
				}
			}
		case []any:
			for _, c := range v {
				labels(c)
			}
		}
	}
	labels(tree)
}
