// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// DELIVERY'S « DELIVERED » TAB (view 5 of
// delivery-views-v2-mockup.html): a Delivered tab after Open increments, its badge the
// count of the increments done, its content a section reading
// increments/delivered-page - the increments done grouped by the release they
// shipped in, newest release first and « No release » last, each group a fold
// (the newest open), each line its id, title, what it delivered in one line,
// its number of requests and the day its delivery was approved; French on
// fr, with no English left.
func TestDeliveryHasADeliveredTab(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	older := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.0.0", Date: "2026-01-15"})
	newer := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2026-06-15"})
	done := func(title, description, in string) string {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title, Description: description, By: person})
		if err != nil {
			t.Fatal(err)
		}
		id := out.Increment.ID
		if in != "" {
			if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: id, In: in}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: id, By: person, Force: true, Note: forcedNote}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	// One delivered the way a person approves it: its work, then the approval.
	approved := increment(t, ctx, "Invoices by mail")
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: approved.ID, In: newer.ID}); err != nil {
		t.Fatal(err)
	}
	workIn(t, ctx, req, approved.ID, "Totals are rounded wrong")
	reviewDelivery(t, ctx, approved.ID, nil)
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, approved.ID, "approve-delivery").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "delivered", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := inc.Get(ctx, api.GetInput{ID: approved.ID}); got.Increment.State != api.IncrementDone {
		t.Fatalf("its delivery approved: %s; want done", got.Increment.State)
	}
	links := done("Payment links", "A link in each invoice pays it.\nThe bank confirms it.", newer.ID)
	first := done("First invoices", "Invoices are numbered.", older.ID)
	loose := done("A loose fix", "", "")
	open := increment(t, ctx, "Still open").ID
	today := time.Now().Format("02/01")
	sum, err := inc.Summary(ctx, api.GetInput{ID: approved.ID})
	if err != nil {
		t.Fatal(err)
	}

	tabs := func(lang string) (ids []string, delivered string) {
		t.Helper()
		def, err := sm.DeliveryPage(ctx, api.DeliveryPageInput{Target: "billing", Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the Delivery view must display: %v", err)
		}
		var tree struct {
			Tabs []json.RawMessage `json:"tabs"`
		}
		raw, _ := json.Marshal(def.DSLTree)
		_ = json.Unmarshal(raw, &tree)
		for _, tb := range tree.Tabs {
			var head struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(tb, &head)
			ids = append(ids, head.ID)
			if head.ID == "delivered" {
				delivered = string(tb)
			}
		}
		return ids, delivered
	}

	ids, tab := tabs("")
	if strings.Join(ids, ",") != "planned,running,open,delivered" {
		t.Fatalf("the tabs are %v; want Delivered after Open increments", ids)
	}
	for _, want := range []string{`"label":"Delivered"`, `"badge":4`, `"badgeColor":"green"`, `"data-view":"delivery-delivered"`,
		`"method":"ktw-agents:increments/delivered-page"`, `"id":"delivered-groups"`} {
		if !strings.Contains(tab, want) {
			t.Errorf("the Delivered tab lacks %s: %s", want, tab)
		}
	}
	if _, fr := tabs("fr"); !strings.Contains(fr, `"label":"Livrés"`) || !strings.Contains(fr, `"language":"fr"`) {
		t.Errorf("the French Delivered tab: %s", fr)
	}

	page := func(lang string) string {
		t.Helper()
		def, err := inc.DeliveredPage(ctx, api.DeliveredIncrementsInput{Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the Delivered tab must display: %v", err)
		}
		b, _ := json.Marshal(def.DSLTree)
		return string(b)
	}
	en := page("")
	// Grouped by release, newest first, « No release » last; each a fold,
	// the newest open.
	at := func(s, what string) int { return strings.Index(s, what) }
	groups := []int{at(en, `"id":"delivered-`+newer.ID+`"`), at(en, `"id":"delivered-`+older.ID+`"`), at(en, `"id":"delivered-none"`)}
	for n := range groups {
		if groups[n] < 0 || (n > 0 && groups[n] < groups[n-1]) {
			t.Fatalf("the groups are not the newest release, the older, then No release: %v in %s", groups, en)
		}
	}
	for _, want := range []string{`"kind":"accordion"`, `"multiple":true`, `"defaultOpen":["delivered-` + newer.ID + `"]`,
		`"badge":2`, `"label":"1.1.0"`, `"label":"1.0.0"`, `"label":"No release"`, `"data-count":3`} {
		if !strings.Contains(en, want) {
			t.Errorf("the groups lack %s: %s", want, en)
		}
	}
	// Each increment the planification card: id, title
	// what it delivered in two lines at most, its requests, its approval's day;
	// the approved one first in its release.
	for _, want := range []string{`"data-open":"` + approved.ID + `"`, `"content":"Invoices by mail","data-title":"` + approved.ID + `"`,
		`"content":"A link in each invoice pays it.","data-delivered-what":"` + links + `"`, `"content":"Invoices are numbered."`,
		fmt.Sprintf(`"content":"%d request%s"`, sum.Requests, map[bool]string{true: "", false: "s"}[sum.Requests == 1]),
		`"content":"approved ` + today + `","data-approved":"` + approved.ID + `"`, `"content":"0 requests"`,
		`"data-increment":"` + first + `"`, `"data-increment":"` + loose + `"`,
		`"data-view":"delivered-rows"`, `"nodeField":"card"`, `"toneField":"tone"`, `"tone":"green"`, `"columns":3`, `"lineClamp":2`} {
		if !strings.Contains(en, want) {
			t.Errorf("the lines lack %s: %s", want, en)
		}
	}
	if sum.Requests < 1 {
		t.Errorf("the approved increment counts %d requests; want its work", sum.Requests)
	}
	if strings.Contains(en, "The bank confirms it.") {
		t.Error("what an increment delivered shows on more than one line")
	}
	if strings.Contains(en, `"data-increment":"`+open+`"`) {
		t.Error("an open increment shows in Delivered")
	}
	if a, l := at(en, `"data-increment":"`+approved.ID+`"`), at(en, `"data-increment":"`+links+`"`); a < 0 || l < 0 || a > l {
		t.Errorf("in its release the newest approved is not first: %d %d", a, l)
	}

	// French, plurals right, no English left.
	fr := page("fr")
	for _, want := range []string{`"label":"Sans version"`, `"content":"approuvé le ` + today + `"`, `"content":"0 demande"`,
		fmt.Sprintf(`"content":"%d demande%s"`, sum.Requests, map[bool]string{true: "", false: "s"}[sum.Requests <= 1])} {
		if !strings.Contains(fr, want) {
			t.Errorf("the French tab lacks %s: %s", want, fr)
		}
	}
	for _, english := range []string{`"approved `, ` request"`, ` requests"`, "No release", `"Delivered`, `"Open `} {
		if strings.Contains(fr, english) {
			t.Errorf("the French tab keeps the English %q: %s", english, fr)
		}
	}
}
