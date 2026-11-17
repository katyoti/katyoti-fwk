// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"errors"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// MODULE TO MODULE — a module asks another by filing a request into it, about
// an object; it lists what it has asked. A question is done when answered.

var shippingAsks = cwmapi.Actor{ID: "shipping", Kind: api.RequesterModule}

// A module files a request into another, about an object there: the target
// sees it new, the asking module lists it as its own.
func TestAModuleAsksAnother(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	cwmapp.Module(t, ctx, cwm, "shipping")
	uc := usecase(t, ctx, cwm, "An invoice is sent")
	r := file(t, ctx, req, api.FileInput{Title: "Expose the invoice total", Module: "billing", About: uc, Kind: api.KindBuild, By: shippingAsks})
	if r.Module != "billing" || r.State != api.PlanReady || r.Requester != shippingAsks {
		t.Fatalf("filed = %+v; want new in billing, asked by the shipping module", r)
	}
	module, _ := cwmapi.ModuleOfObjectID(r.ID)
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: r.ID})
	if err != nil || got.Object.EdgeTo(api.EdgeAbout) != uc {
		t.Fatalf("about edge = %q, %v; want %s", got.Object.EdgeTo(api.EdgeAbout), err, uc)
	}
	file(t, ctx, req, api.FileInput{Title: "Asked by a person", Module: "billing"})
	out, err := req.List(ctx, api.ListInput{Requester: "shipping"})
	if err != nil || len(out.Requests) != 1 || out.Requests[0].ID != r.ID {
		t.Fatalf("shipping's outbox = %+v, %v; want only %s", out.Requests, err, r.ID)
	}
	if _, err := req.File(ctx, api.FileInput{Title: "From nowhere", Module: "billing", By: cwmapi.Actor{ID: "nowhere", Kind: api.RequesterModule}}); err == nil {
		t.Error("a request from a module that does not exist was filed")
	}
}

// A question is done only once answered; the answer is recorded on it.
func TestAQuestionIsDoneWhenAnswered(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	cwmapp.Module(t, ctx, cwm, "shipping")
	q := file(t, ctx, req, api.FileInput{Title: "Which currency do totals use?", Module: "billing", Kind: api.KindQuestion, By: shippingAsks})
	_, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: q.ID, By: builder})
	if !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "answered") {
		t.Fatalf("done without an answer: %v, want refused (a question is done when answered)", err)
	}
	if _, err := req.Answer(ctx, api.AnswerInput{ID: q.ID, Text: "  ", By: person}); err == nil {
		t.Error("an empty answer was taken")
	}
	out, err := req.Answer(ctx, api.AnswerInput{ID: q.ID, Text: "Euros, two decimals.", By: person})
	if err != nil {
		t.Fatal(err)
	}
	a := out.Request.Answer
	if out.Request.State != api.StateDone || a == nil || a.Text != "Euros, two decimals." || a.By != person || a.At == "" {
		t.Fatalf("answered = %s, %+v; want done with the answer, who and when", out.Request.State, a)
	}
	b := file(t, ctx, req, api.FileInput{Title: "Build it", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Answer(ctx, api.AnswerInput{ID: b.ID, Text: "no", By: person}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("answering a build request: %v, want refused (only a question is answered)", err)
	}
}

// A question is answered at once, wherever its plan stands: it is done.
func TestANewQuestionIsAnsweredAtOnce(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	q := file(t, ctx, req, api.FileInput{Title: "Is the total VAT included?", Module: "billing", Kind: api.KindQuestion})
	out, err := req.Answer(ctx, api.AnswerInput{ID: q.ID, Text: "Yes.", By: person})
	if err != nil || out.Request.State != api.StateDone {
		t.Fatalf("answered from new: %s, %v; want done", out.Request.State, err)
	}
}
