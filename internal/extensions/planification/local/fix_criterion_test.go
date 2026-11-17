// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// criterionWorld is the planning world with acceptance criteria and their
// verdicts declared.
func criterionWorld(t *testing.T) (context.Context, cwmapi.CWMService, api.RequestService) {
	t.Helper()
	ctx, cwm, req := worldWith(t, fstest.MapFS{
		"object-types/acceptance-criterion.yaml": {Data: []byte("name: acceptance-criterion\nfolder: criteria\nid_prefix: AC\ncapabilities: [want, checkable]\n")},
		"object-types/verdict.yaml":              {Data: []byte("name: verdict\nfolder: verdicts\n")},
	})
	return ctx, cwm, req
}

// verdictCriterion makes an acceptance criterion serving a feature, with a verdict of
// that status and evidence; it answers its id.
func verdictCriterion(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, feature, title, status, evidence string) string {
	t.Helper()
	ac := cwmapi.Object{Type: "acceptance-criterion", Edges: []cwmapi.ObjectEdge{{Kind: "serves", To: feature}}}
	ac.Parent, ac.Name = feature, title
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: ac})
	if err != nil {
		t.Fatal(err)
	}
	id := out.Object.ID
	v := cwmapi.Object{Type: "verdict", ID: "billing.verdict-" + strings.TrimPrefix(id, "billing.")}
	v.Parent, v.Name = id, "Verdict: "+title
	_ = v.SetCapability("verdict", map[string]any{"status": status, "evidence": evidence, "kind": "command"})
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: v}); err != nil {
		t.Fatal(err)
	}
	return id
}

// : a failed acceptance criterion of a feature an open
// increment holds files its correction in one action - a fix on that feature,
// in that increment, naming the criterion and its failure; a criterion that
// passed, or a feature no open increment holds, files nothing.
func TestAFailedCriterionFilesItsFixIntoTheIncrement(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := criterionWorld(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	f := want(t, ctx, cwm, "feature", "Mail the invoices")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	failed := verdictCriterion(t, ctx, cwm, f, "Each mailed invoice shows its due date", "fail", "--- FAIL: TestDueDate\n    due date missing")
	passed := verdictCriterion(t, ctx, cwm, f, "Each invoice is mailed once", "pass", "ok")

	out, err := req.FixCriterion(ctx, api.FixCriterionInput{Criterion: failed, By: person})
	if err != nil {
		t.Fatalf("fix of a failed criterion: %v", err)
	}
	g, err := req.Get(ctx, api.GetInput{ID: out.Request.ID})
	if err != nil {
		t.Fatal(err)
	}
	r := g.Request
	if r.Kind != api.KindFix || !strings.Contains(r.Title, failed) || !strings.Contains(r.Details, "due date missing") {
		t.Errorf("the fix: kind %q, title %q, details %q; want a fix naming %s and its failure", r.Kind, r.Title, r.Details, failed)
	}
	if td := stepOf(t, ctx, out.Request.ID, api.StepIntake); !containsLabel(td.Labels, "increment:"+in.ID) {
		t.Errorf("the fix's to-do labels %v; want it in increment %s", td.Labels, in.ID)
	}
	if again, err := req.FixCriterion(ctx, api.FixCriterionInput{Criterion: failed, By: person}); err != nil || again.Request.ID != out.Request.ID {
		t.Errorf("a second fix of the same criterion = %s (%v); want the open fix %s", again.Request.ID, err, out.Request.ID)
	}
	if _, err := req.FixCriterion(ctx, api.FixCriterionInput{Criterion: passed, By: person}); err == nil {
		t.Error("a criterion that passed filed a fix")
	}
	lone := want(t, ctx, cwm, "feature", "Print the invoices")
	alone := verdictCriterion(t, ctx, cwm, lone, "Each printed invoice fits a page", "fail", "too long")
	if _, err := req.FixCriterion(ctx, api.FixCriterionInput{Criterion: alone, By: person}); err == nil || !strings.Contains(err.Error(), "no increment") {
		t.Errorf("a feature no increment holds = %v, want refused naming it", err)
	}
}

func containsLabel(labels []string, l string) bool {
	for _, x := range labels {
		if x == l {
			return true
		}
	}
	return false
}
