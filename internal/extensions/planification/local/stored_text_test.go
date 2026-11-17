// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// STORED TEXT IS UTF-8 WITH LF — a delta reported through a Windows command
// line keeps its guillemets (« never stored as Â«) and its line ends LF; a
// rescope's wording is stored with LF line ends, and the stored details it was
// not asked to change stay as they were.

func TestADeltaIsStoredAsUTF8WithLF(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	d := file(t, ctx, req, api.FileInput{Title: "Customers get their invoices by mail", Module: "billing", Kind: api.KindDemand, By: person})
	pullAndReport(t, ctx, stepOf(t, ctx, d.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a demand"})
	s := schedOf(t, ctx)
	rec := stepOf(t, ctx, d.ID, api.StepReconcile).ID
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: rec, By: session}); err != nil {
		t.Fatal(err)
	}
	// What a code-page hop and a Windows line end make of « A rule » — kept.
	delta := []map[string]any{{"op": api.OpFeatureAdd, "module": "billing",
		"title":   "Invoices are mailed Â« now Â»",
		"details": "statement: Â« A rule Â» â€” first\r\nsecond",
		"why":     "what the demand asks"}}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: rec, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "done", By: session,
		More: map[string]any{api.DeltaKey: delta}}}); err != nil {
		t.Fatal(err)
	}
	got, err := req.Get(ctx, api.GetInput{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Request.Delta) != 1 {
		t.Fatalf("the demand's delta: %+v; want its one operation", got.Request.Delta)
	}
	op := got.Request.Delta[0]
	if op.Title != "Invoices are mailed « now »" || op.Details != "statement: « A rule » — first\nsecond" {
		t.Errorf("the stored delta: %q / %q; want UTF-8 guillemets and LF", op.Title, op.Details)
	}
}

func TestARescopeStoresLF(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild, Details: "By mail.", By: person})
	got, err := req.Rescope(ctx, api.RescopeInput{ID: r.ID, Details: "By e-mail,\r\nwith the PDF.\r\n", Note: "paper\r\nis gone", By: session})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Request.Details, "\r") || !strings.Contains(got.Request.Details, "By e-mail,\nwith the PDF.") {
		t.Errorf("rescoped details: %q; want LF line ends", got.Request.Details)
	}
}

func TestARescopeLeavesTheStoredDetailsItWasNotGiven(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	// Text the stored-text repair would rewrite: a rescope of the title alone
	// keeps it byte for byte.
	stored := "Paid by card, Â« not Â» by cheque - Â©2026"
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild, Details: stored, By: person})
	got, err := req.Rescope(ctx, api.RescopeInput{ID: r.ID, Title: "Send invoices by e-mail", Note: "paper is gone", By: session})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Request.Details, stored+"\n\n_Rescoped by ") {
		t.Errorf("rescoped details: %q; want the stored ones %q kept byte for byte", got.Request.Details, stored)
	}
}
