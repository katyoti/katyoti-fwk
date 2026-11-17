// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// syncFollower is what follows a source's sync: the source.sync events CWM
// publishes on the source's subject, in order, as « phase read/total why ».
type syncFollower struct {
	mu   sync.Mutex
	seen []string
}

func (f *syncFollower) on(_ string, payload any) {
	ev, ok := payload.(cwmapi.ObjectRaisedEventPayload)
	if !ok || ev.Event != api.EventSourceSync {
		return
	}
	var p api.SourceSyncProgress
	if err := json.Unmarshal([]byte(ev.Summary), &p); err != nil {
		f.mu.Lock()
		f.seen = append(f.seen, "not JSON: "+ev.Summary)
		f.mu.Unlock()
		return
	}
	line := fmt.Sprintf("%s %d/%d", p.Phase, p.Read, p.Total)
	if p.Error != "" {
		line += " " + p.Error
	}
	if p.Source != ev.ObjectID {
		line += " on " + ev.ObjectID + " for " + p.Source
	}
	f.mu.Lock()
	f.seen = append(f.seen, line)
	f.mu.Unlock()
}

func (f *syncFollower) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	got := f.seen
	f.seen = nil
	return got
}

// : a source's sync publishes where it stands as source.sync
// events raised on the source - started, items read of those its connector
// named, then failed with why or ended -, which a run and the source's card
// follow on the source's subject; inside a sync-all too, whose own progress
// bar stays quiet about the syncs it calls.
func TestASourceSyncPublishesItsProgressPerSource(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()
	ctx, _, svc, _ := pickWorldExts(t, nil, application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
		return bus, nil
	}))
	pickVersion = "v1"
	defer func() { pickVersion = "v1" }()

	// CWM raises only a declared event type naming a point: the events seen
	// prove source.sync declared, at on_source_sync.
	follow := &syncFollower{}
	defer bus.SubscribeAny(follow.on)()

	// Three items, one its connector cannot read: the sync fails, saying why.
	_, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: []api.SelectedItem{
		{Key: "A1", Title: "Acme ERP"}, {Key: "A2", Title: "Acme Stock"}, {Key: "BAD", Title: "Gone"}}})
	if err != nil {
		t.Fatal(err)
	}
	if ap, err := svc.Apply(ctx, api.ApplyInput{ID: "m"}); err != nil || len(ap.Results) != 1 || len(ap.Results[0].Failed) != 1 {
		t.Fatalf("applied, a sync with an item not read fails: %v %+v", err, ap)
	}
	got := follow.take()
	want := []string{"started 0/0", "items 0/3", "items 1/3", "items 2/3", "failed 2/3 1 élément(s) sur 3 n'ont pas pu être lus : BAD: not found in the map"}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("the failing sync's events:\n got %q\nwant %q", got, want)
	}

	// Every item read: it ends.
	if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: []api.SelectedItem{
		{Key: "A1", Title: "Acme ERP"}, {Key: "A2", Title: "Acme Stock"}}}); err != nil {
		t.Fatal(err)
	}
	if ap, err := svc.Apply(ctx, api.ApplyInput{ID: "m"}); err != nil || len(ap.Results) != 1 || len(ap.Results[0].Failed) != 0 {
		t.Fatalf("applied, a sync reading every item: %v %+v", err, ap)
	}
	want = []string{"started 0/0", "items 0/2", "items 1/2", "items 2/2", "ended 2/2"}
	if got := follow.take(); strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("the whole sync's events:\n got %q\nwant %q", got, want)
	}

	// Within a sync-all its syncs publish all the same.
	if _, err := svc.SyncAll(ctx, api.SyncAllInput{ID: "m"}); err != nil {
		t.Fatal(err)
	}
	if got := follow.take(); strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("a sync-all's events for the source:\n got %q\nwant %q", got, want)
	}

	// : items read but failing to anchor never count, during
	// the loop as at its end - a follower never sees the count go back.
	if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: []api.SelectedItem{
		{Key: "A1", Title: "Acme ERP"}, {Key: "LOST1", Title: "Lost one"}, {Key: "LOST2", Title: "Lost two"}}}); err != nil {
		t.Fatal(err)
	}
	if ap, err := svc.Apply(ctx, api.ApplyInput{ID: "m"}); err != nil || len(ap.Results) != 1 || len(ap.Results[0].Failed) != 2 {
		t.Fatalf("applied, a sync with items failing to anchor fails: %v %+v", err, ap)
	}
	got = follow.take()
	read := -1
	for _, line := range got {
		var phase string
		var n, of int
		if _, err := fmt.Sscanf(line, "%s %d/%d", &phase, &n, &of); err != nil {
			t.Fatalf("event %q: %v", line, err)
		}
		if n < read {
			t.Fatalf("the count went back at %q: %q", line, got)
		}
		read = n
	}
	// One event per item tried, then the closing one: the two lost never count.
	want = []string{"started 0/0", "items 0/3", "items 1/3", "items 1/3", "items 1/3", "failed 1/3 2 élément(s) sur 3"}
	last := len(want) - 1
	if len(got) != len(want) || strings.Join(got[:last], " | ") != strings.Join(want[:last], " | ") || !strings.HasPrefix(got[last], want[last]) {
		t.Fatalf("the anchor-failing sync's events:\n got %q\nwant %q", got, want)
	}
}
