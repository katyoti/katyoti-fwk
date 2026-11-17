// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// PUBLICATIONS, end to end on a real CWM: a publication renders its view and
// hands the documents to a converter â€” here a fake one standing for
// a Confluence connector â€” and records how it went.

const fakeConverterPrefix = "ktw-test:fake/"

var (
	convertedMu sync.Mutex
	converted   map[string]any // the last input the fake converter received
)

func fakeConvert(in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	if strings.HasSuffix(in.URI, "/fail") {
		return actionsapi.RunOutput{}, errors.New("the target refused the pages")
	}
	var got map[string]any
	if err := json.Unmarshal(in.Input, &got); err != nil {
		return actionsapi.RunOutput{}, err
	}
	convertedMu.Lock()
	converted = got
	convertedMu.Unlock()
	docs, _ := got[api.PublishDocumentsKey].([]any)
	raw, _ := json.Marshal(map[string]any{"pages": len(docs)})
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

func publications(t *testing.T, ctx context.Context) api.PublicationService {
	t.Helper()
	svc, err := application.UseService[api.PublicationService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// Publishing a stream hands the converter its settings and the whole page set,
// and records the outcome on the publication.
func TestAPublicationHandsItsPagesToTheConverter(t *testing.T) {
	t.Parallel()
	ctx, _, _, _, _ := planWorld(t)
	pubs := publications(t, ctx)
	p, err := pubs.Create(ctx, api.CreatePublicationInput{
		Name: "KaTyoti on Confluence", View: api.ViewStream, Scope: "KaTyoti", Lang: "fr",
		Target: fakeConverterPrefix + "publish", Settings: map[string]any{"folder": "docs/confluence-tew/katyoti"},
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := pubs.Preview(ctx, api.GetInput{ID: p.Publication.ID})
	if err != nil || len(preview.Documents) != 5 {
		t.Fatalf("Preview: %d documents, %v; want the 5 pages of the stream", len(preview.Documents), err)
	}
	out, err := pubs.Publish(ctx, api.GetInput{ID: p.Publication.ID})
	if err != nil {
		t.Fatal(err)
	}
	last := out.Publication.Last
	if last == nil || last.Documents != 5 || last.Error != "" || !strings.Contains(last.Result, `"pages":5`) {
		t.Fatalf("last = %+v, want 5 documents published", last)
	}
	convertedMu.Lock()
	got := converted
	convertedMu.Unlock()
	if got["folder"] != "docs/confluence-tew/katyoti" {
		t.Errorf("the converter got settings %v, want the folder", got)
	}
	docs, _ := got[api.PublishDocumentsKey].([]any)
	if len(docs) != 5 || !strings.Contains(string(mustJSON(t, docs[1])), "Journal des changements") {
		t.Errorf("the converter got %d documents, the second %s", len(docs), mustJSON(t, docs[1]))
	}
	again, err := pubs.Get(ctx, api.GetInput{ID: p.Publication.ID})
	if err != nil || again.Publication.Last == nil || again.Publication.Last.At == "" {
		t.Errorf("the outcome is not kept on the publication: %+v, %v", again.Publication, err)
	}
}

// A converter that fails leaves its error on the publication, and Publish says so.
func TestAFailedPublicationIsRecorded(t *testing.T) {
	t.Parallel()
	ctx, _, _, _, inc := planWorld(t)
	pubs := publications(t, ctx)
	p, err := pubs.Create(ctx, api.CreatePublicationInput{
		Name: "Plan page", View: api.ViewReleasePlan, Scope: inc.ID, Target: fakeConverterPrefix + "fail",
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := pubs.Publish(ctx, api.GetInput{ID: p.Publication.ID})
	if err == nil {
		t.Fatal("a failed publication reported success")
	}
	if last := out.Publication.Last; last == nil || !strings.Contains(last.Error, "refused") || last.Documents != 1 {
		t.Fatalf("last = %+v, want the converter's error kept", last)
	}
}

// A release released publishes again the publications marked on_release â€” and
// only those.
func TestAReleasePublishesItsPagesAgain(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	pubs := publications(t, ctx)
	// Filed first, so it would be published before `on` if the flag were ignored.
	off, err := pubs.Create(ctx, api.CreatePublicationInput{Name: "by hand", View: api.ViewChangeLog, Target: fakeConverterPrefix + "publish"})
	if err != nil {
		t.Fatal(err)
	}
	on, err := pubs.Create(ctx, api.CreatePublicationInput{Name: "log", View: api.ViewChangeLog, Target: fakeConverterPrefix + "publish", OnRelease: true})
	if err != nil {
		t.Fatal(err)
	}
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: intake}); err != nil {
		t.Fatal(err)
	}
	changelogListed(t, ctx, cwm, r.ID)
	if _, err := rel.Release(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, err := pubs.Get(ctx, api.GetInput{ID: on.Publication.ID})
		if err == nil && got.Publication.Last != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s was not published again 5s after the release", on.Publication.ID)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got, _ := pubs.Get(ctx, api.GetInput{ID: off.Publication.ID}); got.Publication.Last != nil {
		t.Errorf("%s (not on_release) was published: %+v", off.Publication.ID, got.Publication.Last)
	}
}

// A view it cannot render, or one missing its scope, is refused at creation.
func TestAPublicationNamesAViewItCanRender(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	pubs := publications(t, ctx)
	for _, in := range []api.CreatePublicationInput{
		{Name: "x", View: "gantt", Target: fakeConverterPrefix + "publish"},
		{Name: "x", View: api.ViewReleaseBoard, Target: fakeConverterPrefix + "publish"},
		{Name: "x", View: api.ViewChangeLog},
	} {
		if _, err := pubs.Create(ctx, in); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("Create %+v: %v, want refused", in, err)
		}
	}
	if _, err := pubs.Create(ctx, api.CreatePublicationInput{Name: "log", View: api.ViewChangeLog, Target: fakeConverterPrefix + "publish"}); err != nil {
		t.Errorf("a project change log (no scope needed): %v", err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// A release republishes only the pages that show it: its own board, its
// module's change log; another release's board and another module's change log
// are left as they are.
func TestAReleaseRepublishesOnlyThePagesThatShowIt(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	cwmapp.Module(t, ctx, cwm, "shipping")
	pubs := publications(t, ctx)
	rel := releases(t, ctx)
	shipped := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	other := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2.0.0"})
	create := func(name, view, scope string) string {
		t.Helper()
		p, err := pubs.Create(ctx, api.CreatePublicationInput{Name: name, View: view, Scope: scope, Target: fakeConverterPrefix + "publish", OnRelease: true})
		if err != nil {
			t.Fatal(err)
		}
		return p.Publication.ID
	}
	// The ones left alone are filed first: they would go first if scope were ignored.
	otherBoard := create("other board", api.ViewReleaseBoard, other.ID)
	otherLog := create("shipping log", api.ViewChangeLog, "shipping")
	board := create("board", api.ViewReleaseBoard, shipped.ID)
	log := create("billing log", api.ViewChangeLog, "billing")

	if _, err := rel.Freeze(ctx, api.MoveInput{ID: shipped.ID, By: intake}); err != nil {
		t.Fatal(err)
	}
	changelogListed(t, ctx, cwm, shipped.ID)
	if _, err := rel.Release(ctx, api.MoveInput{ID: shipped.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	published := func(id string) bool {
		got, err := pubs.Get(ctx, api.GetInput{ID: id})
		return err == nil && got.Publication.Last != nil
	}
	deadline := time.Now().Add(5 * time.Second)
	for !(published(board) && published(log)) {
		if time.Now().After(deadline) {
			t.Fatalf("5s after the release: board published %v, billing log %v; want both", published(board), published(log))
		}
		time.Sleep(20 * time.Millisecond)
	}
	if published(otherBoard) || published(otherLog) {
		t.Errorf("published pages that do not show the release: other board %v, shipping log %v", published(otherBoard), published(otherLog))
	}
}
