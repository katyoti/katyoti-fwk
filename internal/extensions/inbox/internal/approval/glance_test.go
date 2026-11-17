// SPDX-License-Identifier: Apache-2.0

package approval

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	planapi "github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

type glanceStreams struct{ planapi.StreamService }

func (glanceStreams) List(context.Context, planapi.ListStreamsInput) (planapi.ListStreamsOutput, error) {
	return planapi.ListStreamsOutput{Streams: []planapi.Stream{{ID: "p.SM-1", Name: "KaTyoti"}}}, nil
}

type glanceReleases struct{ planapi.ReleaseService }

func (glanceReleases) List(context.Context, planapi.ListInput) (planapi.ListReleasesOutput, error) {
	return planapi.ListReleasesOutput{Releases: []planapi.Release{
		{ID: "p.RL-1", Name: "Shipped", State: planapi.ReleaseFrozen, Stream: "p.SM-1", ReleaseFields: planapi.ReleaseFields{Version: "1.000", Date: "2026-09-18"}},
		{ID: "p.RL-2", Name: "A software factory", State: planapi.ReleasePlanned, Stream: "p.SM-1", ReleaseFields: planapi.ReleaseFields{Version: "1.006", Date: "2026-10-31"}},
	}}, nil
}

type glanceIncrements struct{ planapi.IncrementService }

func (glanceIncrements) List(context.Context, planapi.ListIncrementsInput) (planapi.ListIncrementsOutput, error) {
	return planapi.ListIncrementsOutput{Increments: []planapi.Increment{
		{ID: "p.IN-1", Title: "Delivery at a glance", State: planapi.IncrementOpen, Stream: "p.SM-1", ShipsIn: "p.RL-2", Priority: "P0"},
		{ID: "p.IN-2", Title: "Concept guides", State: planapi.IncrementOpen, Stream: "p.SM-1", Priority: "P1"},
	}}, nil
}

func (glanceIncrements) Summary(_ context.Context, in planapi.GetInput) (planapi.IncrementSummaryOutput, error) {
	if in.ID == "p.IN-1" {
		return planapi.IncrementSummaryOutput{Done: 16, Total: 21, Awaiting: 2}, nil
	}
	return planapi.IncrementSummaryOutput{Done: 0, Total: 3}, nil
}

type glanceViews struct{ planapi.ViewService }

func (glanceViews) PersonQueue(context.Context, planapi.PersonQueueInput) (planapi.PersonQueueOutput, error) {
	return planapi.PersonQueueOutput{Items: []planapi.PersonItem{
		{Action: "decide", Title: "Phone pages?", Asks: "Choose one of its options", Object: "k.OD-9", ReleaseTitle: "1.006 — A software factory", IncrementTitle: "Delivery at a glance"},
		{Action: "validate", Title: "Review the study", Asks: "Accept the result", Object: "b.RQ-3", Request: "b.RQ-3"},
		{Action: "answer", Title: "Put aside", Held: &planapi.Hold{Reason: "later"}},
	}}, nil
}

// THE PHONE READS DELIVERY AND « À FAIRE » (feature, decision
// ): the page server answers /overview - each stream's next
// planned release, the increments it ships apart, their progress and what
// waits on the person - and /todo - what waits, by release then increment, an
// item linked to its one-tap card when one is open; both read-only (GET only,
// nothing on them posts), over the same listener as the cards.
func TestThePhoneReadsDeliveryAndToDoReadOnly(t *testing.T) {
	ctx, _ := cwmapp.New(t,
		application.DeclareSimpleService[planapi.StreamService](func(context.Context) (planapi.StreamService, error) { return glanceStreams{}, nil }),
		application.DeclareSimpleService[planapi.ReleaseService](func(context.Context) (planapi.ReleaseService, error) { return glanceReleases{}, nil }),
		application.DeclareSimpleService[planapi.IncrementService](func(context.Context) (planapi.IncrementService, error) { return glanceIncrements{}, nil }),
		application.DeclareSimpleService[planapi.ViewService](func(context.Context) (planapi.ViewService, error) { return glanceViews{}, nil }),
	)
	s := New(0, nil, func(context.Context) (string, error) { return origin, nil })
	now := time.Now()
	s.Now = func() time.Time { return now }
	hs := httptest.NewUnstartedServer(s.Handler())
	hs.Config.BaseContext = func(net.Listener) context.Context { return ctx }
	hs.Start()
	t.Cleanup(hs.Close)
	// An approval open on the request awaiting validation: its card is linked.
	open, err := s.Open(ctx, api.ApprovalOpenInput{Person: "jdoe", Act: "planification/validate b.RQ-3", Title: "Review the study"})
	if err != nil {
		t.Fatal(err)
	}

	code, page := get(t, hs.URL+"/overview")
	if code != http.StatusOK {
		t.Fatalf("/overview: %d %s", code, page)
	}
	for _, want := range []string{"KaTyoti", "Next release: 1.006 — A software factory · 2026-10-31", "2 waiting on you", "Ships in it", "Delivery at a glance", "16/21 done", "Not in that release yet", "Concept guides"} {
		if !strings.Contains(page, want) {
			t.Errorf("/overview lacks %q", want)
		}
	}
	if strings.Contains(page, "1.000") {
		t.Error("/overview names the frozen 1.000 as the next release")
	}

	code, page = get(t, hs.URL+"/todo")
	if code != http.StatusOK {
		t.Fatalf("/todo: %d %s", code, page)
	}
	for _, want := range []string{"1.006 — A software factory › Delivery at a glance", "Phone pages?", "In no release › outside any increment", "Review the study", `href="` + strings.TrimPrefix(open.Link, origin) + `"`} {
		if !strings.Contains(page, want) {
			t.Errorf("/todo lacks %q", want)
		}
	}
	if strings.Contains(page, "Put aside") {
		t.Error("/todo shows an item on hold")
	}
	if strings.Index(page, "Delivery at a glance") > strings.Index(page, "In no release") {
		t.Error("/todo does not put what is in no release last")
	}

	for _, p := range []string{"/overview", "/todo"} {
		_, body := get(t, hs.URL+p)
		if strings.Contains(body, "<form") || strings.Contains(body, "method:'POST'") || strings.Contains(body, "<button") {
			t.Errorf("%s acts: it holds a form, a button or a post", p)
		}
		if code, _ := post(t, hs.URL+p, nil); code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s answered %d; want 405 (read-only)", p, code)
		}
	}
}
