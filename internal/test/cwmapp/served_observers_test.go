// SPDX-License-Identifier: Apache-2.0

package cwmapp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// observedMember makes a repository holding a project object of key (no uuid:
// every such member publishes under one workspace slot, so only the store an
// event comes from tells it apart) and the module folders given.
func observedMember(t *testing.T, root, key string, modules ...string) string {
	t.Helper()
	dir := filepath.Join(root, "repo-"+key)
	for _, m := range modules {
		if err := os.MkdirAll(filepath.Join(dir, "modules", m), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, cwmapi.ProjectFileName), []byte("id: "+key+"\ntype: project\nname: "+key+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func observedNote(id string) cwmapi.Object {
	o := cwmapi.Object{ID: id, Type: "note"}
	o.Name = id
	return o
}

// EACH STORE'S OBSERVERS HEAR ITS OWN EVENTS, NAMING ITS PROJECT (
// ). The stores of two served projects publish on one in-process bus:
// an observer attached to each store would hear every write twice if it heard
// the bus. Each hears its store's writes only - each write seen once - and its
// context names the store's project key (cwmapi.EventProject), so a reaction
// writes back into the project the event came from: a system module ensured
// as <key>:sys-<name> lands in that project only, and a file appended by its
// qualified id too. One project served alone names no key.
func TestEachStoresObserversHearItsOwnEventsNamingItsProject(t *testing.T) {
	root := workspaceDir(t)
	alpha := observedMember(t, root, "alpha", "crm")
	beta := observedMember(t, root, "beta", "erp")
	var mu sync.Mutex
	seen := map[string][]string{} // object id -> the project key of each hearing
	observer := cwmapi.ObserveEvents(cwmapi.EventObserver{Name: "test-count", Handle: func(hctx context.Context, ev cwmapi.EventEnvelope) {
		if !strings.HasSuffix(ev.Topic, ".created") {
			return
		}
		var p struct {
			ObjectID string `json:"object_id"`
			Type     string `json:"type"`
		}
		if json.Unmarshal(ev.Payload, &p) != nil || p.Type != "note" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		seen[p.ObjectID] = append(seen[p.ObjectID], cwmapi.EventProject(hctx))
	}})
	bus := func() application.Extension {
		return application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		})
	}
	ctx, svc := Members(t, root, []string{alpha, beta}, bus(), observer, SystemModules(t))
	for _, c := range []struct{ module, id string }{{"crm", "crm.N-1"}, {"erp", "erp.N-1"}} {
		if _, err := svc.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: c.module, Object: observedNote(c.id)}); err != nil {
			t.Fatalf("create %s: %v", c.id, err)
		}
	}
	waitHeard := func(want map[string]string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			mu.Lock()
			done := true
			for id := range want {
				if len(seen[id]) == 0 {
					done = false
				}
			}
			mu.Unlock()
			if done || time.Now().After(deadline) {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(200 * time.Millisecond) // a second hearing would land by now
		mu.Lock()
		defer mu.Unlock()
		for id, key := range want {
			if got := seen[id]; len(got) != 1 || got[0] != key {
				t.Errorf("the write of %s was heard as %q; want once, naming the project %q", id, got, key)
			}
		}
	}
	waitHeard(map[string]string{"crm.N-1": "alpha", "erp.N-1": "beta"})

	// A system module ensured in one project lands there only; a file is
	// appended to one of its objects by qualified id.
	if created, err := cwmapi.EnsureSystemModule(ctx, svc, "alpha:sys-test", "test"); err != nil || !created {
		t.Fatalf("EnsureSystemModule(alpha:sys-test) = %v, %v; want created", created, err)
	}
	if created, err := cwmapi.EnsureSystemModule(ctx, svc, "alpha:sys-test", "test"); err != nil || created {
		t.Errorf("EnsureSystemModule(alpha:sys-test) again = %v, %v; want there already", created, err)
	}
	if _, err := os.Stat(filepath.Join(beta, "modules", "sys-test")); !os.IsNotExist(err) {
		t.Errorf("alpha:sys-test was written in beta (%v)", err)
	}
	if got, err := svc.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: "alpha:sys-test"}); err != nil || got.Object.Name != "sys-test" {
		t.Fatalf("get alpha:sys-test = %+v, %v; want the module, named bare", got.Object, err)
	}
	if _, err := svc.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "alpha:sys-test", Object: observedNote("sys-test.D-1")}); err != nil {
		t.Fatalf("create alpha:sys-test.D-1: %v", err)
	}
	if _, err := svc.ObjectNamedFileDeclare(ctx, cwmapi.ObjectNamedFileDeclareInput{
		ID: "alpha:sys-test.D-1", Name: "entries", File: cwmapi.NamedFile{Path: "days/d-1.jsonl"},
	}); err != nil {
		t.Fatalf("declare entries on alpha:sys-test.D-1: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.ObjectNamedFileWrite(ctx, cwmapi.ObjectNamedFileWriteInput{
			ID: "alpha:sys-test.D-1", Name: "entries", Content: []byte("line\n"), Append: true,
		}); err != nil {
			t.Fatalf("append to alpha:sys-test.D-1: %v", err)
		}
	}
	read, err := svc.ObjectNamedFileRead(ctx, cwmapi.ObjectNamedFileReadInput{ID: "alpha:sys-test.D-1", Name: "entries"})
	if err != nil || string(read.Content) != "line\nline\n" {
		t.Errorf("read alpha:sys-test.D-1 entries = %q, %v; want both lines", read.Content, err)
	}
	if _, err := cwmapi.EnsureSystemModule(ctx, svc, "alpha:crm", "x"); err == nil {
		t.Errorf("EnsureSystemModule(alpha:crm) accepted a module not named sys-")
	}

	// One project served alone: its observers name no project, and a bare
	// system module is ensured as always.
	mu.Lock()
	seen = map[string][]string{}
	mu.Unlock()
	soloRoot := filepath.Join(root, "solo")
	solo := observedMember(t, soloRoot, "gamma", "crm")
	_, one := Members(t, soloRoot, []string{solo}, bus(), observer, SystemModules(t))
	if _, err := one.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "crm", Object: observedNote("crm.N-9")}); err != nil {
		t.Fatal(err)
	}
	waitHeard(map[string]string{"crm.N-9": ""})
	if created, err := cwmapi.EnsureSystemModule(ctx, one, "sys-test", "test"); err != nil || !created {
		t.Errorf("EnsureSystemModule(sys-test) on one project = %v, %v; want created", created, err)
	}
}
