// SPDX-License-Identifier: Apache-2.0

package remote_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// --- M6: every remote read is verified against its address ---

// A body the aggregator answers that does not hash to the address the index
// holds is refused, never served: the reader used to trust the wire.
func TestRemoteGetBody_RefusesATamperedBody(t *testing.T) {
	real := []byte(`{"name":"real"}`)
	hash := contribapi.HashContent(real)
	e := contribapi.Contribution{Provider: "p", Type: "task", ID: "t1", Layers: contribapi.DataDescriptor(hash, int64(len(real)))}
	f := &fakeSender{pages: [][]contribapi.Contribution{{e}}, bodies: map[string][]byte{hash: []byte(`{"name":"tampered"}`)}}
	_, err := remote.New(f).GetBody(context.Background(), "p:/task/t1")
	var mismatch contribapi.ErrContentMismatch
	if !errors.As(err, &mismatch) || mismatch.Want != hash {
		t.Fatalf("tampered body: err = %v, want ErrContentMismatch against %s", err, hash)
	}
	// The same bytes, untampered, are served.
	f.bodies[hash] = real
	got, err := remote.New(f).GetBody(context.Background(), "p:/task/t1")
	if err != nil || string(got) != string(real) {
		t.Fatalf("real body: %s %v", got, err)
	}
	// A snapshot reads through the same verified fetch.
	f.bodies[hash] = []byte(`{"name":"tampered"}`)
	snap, err := remote.TakeSnapshot(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snap.GetBody(context.Background(), "p:/task/t1"); !errors.As(err, &mismatch) {
		t.Fatalf("snapshot served a tampered body: err = %v", err)
	}
}

func TestRemoteOpenLayer_RefusesTamperedContent(t *testing.T) {
	real := []byte("# the skill\n")
	hash := contribapi.HashContent(real)
	f := &fakeSender{opens: map[string]mbapi.OpenLayerOutput{
		"p:/skill/s#instructions": {Ref: contribapi.LayerRef{Hash: hash, Size: int64(len(real)), MediaType: "text/markdown"}, Content: []byte("# not the skill\n")},
	}}
	_, _, err := remote.New(f).OpenLayer(context.Background(), "p:/skill/s", "instructions")
	var mismatch contribapi.ErrContentMismatch
	if !errors.As(err, &mismatch) || mismatch.Layer != "instructions" {
		t.Fatalf("tampered layer: err = %v, want ErrContentMismatch on instructions", err)
	}
	f.opens["p:/skill/s#instructions"] = mbapi.OpenLayerOutput{Ref: contribapi.LayerRef{Hash: hash, Size: int64(len(real))}, Content: real}
	rc, _, err := remote.New(f).OpenLayer(context.Background(), "p:/skill/s", "instructions")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	if string(got) != string(real) {
		t.Fatalf("real layer: %q", got)
	}
}

// --- M6: the server-side filter is used, not the whole universe ---

func TestRemoteGetContributionsByType_PassesTheTypeFilter(t *testing.T) {
	f := &fakeSender{pages: [][]contribapi.Contribution{{
		entry("a", "skill", "one", nil), entry("a", "task", "two", nil), entry("b", "task", "three", nil),
	}}}
	got, err := remote.New(f).GetContributionsByType(context.Background(), "task")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("listed %d entries, want the 2 tasks", len(got))
	}
	if len(f.filters) != 1 || len(f.filters[0].Types) != 1 || f.filters[0].Types[0] != "task" {
		t.Fatalf("the aggregator was asked with filters %+v, want Types=[task]", f.filters)
	}
}

// Locating one entry for a body asks for that entry, by provider, type and
// id — not for the universe, once per body.
func TestRemoteGetBody_LocatesTheEntryThroughTheFilter(t *testing.T) {
	body := []byte(`{"name":"two"}`)
	f := &fakeSender{
		pages:  [][]contribapi.Contribution{{entry("a", "skill", "one", nil), entry("a", "task", "two", body)}},
		bodies: map[string][]byte{contribapi.HashContent(body): body},
	}
	got, err := remote.New(f).GetBody(context.Background(), "a:/task/two")
	if err != nil || string(got) != string(body) {
		t.Fatalf("GetBody: %s %v", got, err)
	}
	if len(f.filters) != 1 {
		t.Fatalf("%d index calls for one body, want 1", len(f.filters))
	}
	fl := f.filters[0]
	if strings.Join(fl.Providers, ",") != "a" || strings.Join(fl.Types, ",") != "task" || fl.IDGlob != "two" {
		t.Fatalf("located through filter %+v, want provider a, type task, id two", fl)
	}
}

// --- L4: an index that restarts under the cursor is read once, whole ---

// restartSender answers page 0 with the old index's first entry and, on the
// follow-up, restarts at offset 0 with the new index — what the aggregator
// does when the catalog reloaded between two pages.
type restartSender struct{ calls int }

func (r *restartSender) Notify(context.Context, string, any, ...ktsapi.CallOption) error { return nil }

func (r *restartSender) Call(_ context.Context, _ string, params any, _ ...ktsapi.CallOption) (json.RawMessage, error) {
	r.calls++
	in := params.(mbapi.CatalogListContributionsInput)
	if in.Cursor == "" {
		return json.Marshal(mbapi.CatalogListContributionsOutput{
			Contributions: []contribapi.Contribution{entry("old", "skill", "stale", nil)},
			Pagination:    mbapi.Pagination{Offset: 0, PageSize: 1, HasMore: true, NextCursor: "1@old"},
		})
	}
	return json.Marshal(mbapi.CatalogListContributionsOutput{
		Contributions: []contribapi.Contribution{entry("new", "skill", "fresh", nil)},
		Pagination:    mbapi.Pagination{Offset: 0, PageSize: 1, HasMore: false},
	})
}

func TestRemoteIndex_DropsWhatItReadFromAnIndexThatRestarted(t *testing.T) {
	r := &restartSender{}
	got, err := remote.New(r).Index(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Provider != "new" {
		t.Fatalf("index = %+v, want only the restarted index's entry (the stale one dropped)", got)
	}
	if r.calls != 2 {
		t.Fatalf("%d calls, want 2", r.calls)
	}
}
