// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
)

// THE HISTORY PANEL — every object's History panel draws what happened to it,
// newest first, as a timeline: an object created, changed, then moved shows
// three entries, the move first naming where it came from and why; in French
// on a French screen; an object with nothing recorded says so.
func TestTheHistoryPanelDrawsWhatHappenedNewestFirst(t *testing.T) {
	ctx, cwm, svc := setup(t)
	o := create(t, ctx, cwm, "billing", "billing.refunds", "Refunds")
	o.Description = "paid back within a week"
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	history(t, ctx, svc, "billing.refunds", 2)
	out, err := cwm.ObjectMove(ctx, cwmapi.ObjectMoveInput{ID: "billing.refunds", NewParent: "payments", NewModule: "payments", Reason: "refunds are a payments concern"})
	if err != nil {
		t.Fatal(err)
	}
	history(t, ctx, svc, out.ID, 3)
	items := func(lang string) []map[string]any {
		t.Helper()
		page, err := svc.HistoryPage(ctx, api.HistoryPageInput{ID: out.ID, Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		if err := page.Validate(); err != nil {
			t.Fatalf("the page must display: %v", err)
		}
		b, _ := json.Marshal(page.DSLTree)
		var tree map[string]any
		_ = json.Unmarshal(b, &tree)
		for _, k := range tree["children"].([]any) {
			if n := k.(map[string]any); n["kind"] == "timeline" {
				var got []map[string]any
				for _, it := range n["items"].([]any) {
					got = append(got, it.(map[string]any))
				}
				return got
			}
		}
		t.Fatalf("no timeline in %s", b)
		return nil
	}
	en := items("en")
	if len(en) != 3 {
		t.Fatalf("entries %v; want three (moved, changed, created)", en)
	}
	if title := en[0]["title"].(string); !strings.HasPrefix(title, "Moved from billing.refunds") || !strings.Contains(en[0]["body"].(string), "payments concern") {
		t.Errorf("the newest entry %v; want the move, from billing.refunds, with its reason", en[0])
	}
	if !strings.HasPrefix(en[1]["title"].(string), "Changed") || !strings.HasPrefix(en[2]["title"].(string), "Created") {
		t.Errorf("entries %v; want the change then the creation, newest first", en)
	}
	if fr := items("fr"); !strings.HasPrefix(fr[2]["title"].(string), "Créé") {
		t.Errorf("on a French screen the creation reads %q; want Créé", fr[2]["title"])
	}
	create(t, ctx, cwm, "billing", "billing.quiet", "Quiet")
	page, err := svc.HistoryPage(ctx, api.HistoryPageInput{ID: "billing.none"})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(page.DSLTree); !strings.Contains(string(b), "Nothing has been recorded") {
		t.Errorf("an object with nothing recorded draws %s; want it said", b)
	}
}
