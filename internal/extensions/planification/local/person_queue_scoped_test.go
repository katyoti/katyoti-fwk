// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	plancore "github.com/katyoti/katyoti-fwk/internal/extensions/planification/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// AN OBJECT'S « À FAIRE » IS THE WHOLE QUEUE KEPT TO IT:
// the items of a release's or an increment's queue, read from the demands
// whose work goes to it and the inbox's own questions only - a release's pane
// waits on them -, are exactly those the whole queue keeps of it.
func TestAnObjectsQueueIsTheWholeQueueKeptToIt(t *testing.T) {
	long.Test(t)
	t.Parallel()
	w := runningIncrementsWorld(t)
	if n := sameScopedQueues(t, w.ctx); n == 0 {
		t.Fatal("no release or increment has an item: the world proves nothing")
	}
}

// The same on a copy of this repository's modules: every release and every
// increment of the development workspace's planning. On demand (it copies
// the modules, some 80 MB, and indexes them):
//
//	KTW_VIEW_TIMING=1 go test ./internal/extensions/planification/local/ -run TestAnObjectsQueueIsTheWholeQueueKeptToItOnTheModules -count=1 -v -timeout 20m
func TestAnObjectsQueueIsTheWholeQueueKeptToItOnTheModules(t *testing.T) {
	t.Parallel()
	if os.Getenv("KTW_VIEW_TIMING") == "" {
		t.Skip("on demand: KTW_VIEW_TIMING=1 (copies modules/ and indexes it)")
	}
	ctx, cwm, _ := world(t)
	at, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: "billing"})
	if err != nil {
		t.Fatalf("find the world's project: %v", err)
	}
	modules := at.Path
	for filepath.Base(modules) != "modules" && filepath.Dir(modules) != modules {
		modules = filepath.Dir(modules)
	}
	if err := copyModules(filepath.Join("..", "..", "..", "..", "modules"), modules); err != nil {
		t.Fatalf("copy the modules: %v", err)
	}
	if _, err := cwm.ObjectScan(ctx, cwmapi.ObjectScanInput{}); err != nil {
		t.Fatalf("index the copy: %v", err)
	}
	if n := sameScopedQueues(t, ctx); n == 0 {
		t.Fatal("no release or increment of the modules has an item: the copy proves nothing")
	}
}

// sameScopedQueues checks every release's and increment's items, scoped
// (ObjectQueueItems) and kept from the whole queue (PersonQueue), are the
// same; it says how many of them hold an item.
func sameScopedQueues(t *testing.T, ctx context.Context) int {
	t.Helper()
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	rs, err := releases(t, ctx).List(ctx, api.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs.Releases {
		ids = append(ids, r.ID)
	}
	is, err := increments(t, ctx).List(ctx, api.ListIncrementsInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range is.Increments {
		ids = append(ids, i.ID)
	}
	js := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	holding := 0
	for _, id := range ids {
		items, err := plancore.ObjectQueueItems(ctx, id, "")
		if err != nil {
			t.Fatalf("ObjectQueueItems %s: %v", id, err)
		}
		q, err := views.PersonQueue(ctx, api.PersonQueueInput{Target: id})
		if err != nil {
			t.Fatalf("PersonQueue %s: %v", id, err)
		}
		n := len(q.Items)
		if scoped, whole := js(items), js(q.Items); scoped != whole {
			t.Errorf("%s's queue scoped differs from the whole queue kept to it:\nscoped %s\nwhole  %s", id, scoped, whole)
		}
		if n > 0 {
			holding++
		}
	}
	t.Logf("%d releases and increments, %d holding an item", len(ids), holding)
	return holding
}
