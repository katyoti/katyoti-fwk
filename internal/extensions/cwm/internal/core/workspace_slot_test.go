// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
)

// THE WORKSPACE SLOT. Two repositories served in one universe
// each hold a module `inbox`: their objects' tokens collide, so the workspace
// slot - the project's id - is what keeps their subjects apart. The store
// derives it from its project, the subscriber from the same project with the
// same function (api.ProjectEventScope), and neither passes it to the other.

// twoRepoUniverse is a universe home naming its universe, as Univers.yaml does.
func twoRepoUniverse(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config", "Univers.yaml"), []byte("version: 1\nname: two-repos\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// repoStore opens a store on a project, its tenant and universe resolved as
// cwmlocal resolves them, on the universe's one in-process bus.
func repoStore(t *testing.T, home, projectRoot string, bus eventbus.EventBus) *Service {
	t.Helper()
	tenant, universe := cenv.ResolveTenantUniverse(home)
	svc, err := New(stdctx.Background(), &CWMServiceConfig{
		WorkspaceRoot: freshWorkspace(t), ProjectRoot: projectRoot,
		TenantID: tenant, UniverseID: universe, EventBus: bus,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

// mintedProject is a project root whose project object a first store minted
// (ProjectInit) - the repository as committed, before this store opens it.
func mintedProject(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	first, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.ProjectInit(stdctx.Background(), api.ProjectInitInput{Name: name}); err != nil {
		t.Fatalf("ProjectInit: %v", err)
	}
	_ = first.Close()
	return root
}

func TestTwoRepositoriesModulesOfOneNameNeverShareASubject(t *testing.T) {
	ctx := stdctx.Background()
	home := twoRepoUniverse(t)
	bus := eventbus.NewMemoryEventBus()

	// Repository A is opened with its id already minted; repository B mints
	// it while its store is open - its events carry it at once.
	rootA := mintedProject(t, "repo-a")
	rootB := t.TempDir()
	a := repoStore(t, home, rootA, bus)
	b := repoStore(t, home, rootB, bus)
	if got := b.Events.Workspace(); got != api.UnboundWorkspace {
		t.Fatalf("B before its id is minted publishes under %q, want %q", got, api.UnboundWorkspace)
	}
	if _, err := b.ProjectInit(ctx, api.ProjectInitInput{Name: "repo-b"}); err != nil {
		t.Fatalf("ProjectInit B: %v", err)
	}

	// Each subscriber derives its project's scope itself, from the project.
	tA, uA, wsA := api.ProjectEventScope(home, rootA)
	tB, uB, wsB := api.ProjectEventScope(home, rootB)
	if wsA == api.UnboundWorkspace || wsB == api.UnboundWorkspace || wsA == wsB {
		t.Fatalf("workspace slots A=%q B=%q: want two distinct project ids", wsA, wsB)
	}
	// The universe token is the same for the publisher and the subscriber.
	if uA != a.Config.UniverseID || uB != b.Config.UniverseID || uA != "two-repos" || tA != a.Config.TenantID || tB != b.Config.TenantID {
		t.Fatalf("subscriber scope %s/%s, %s/%s; publishers %s/%s, %s/%s: want the same tenant and universe (two-repos)",
			tA, uA, tB, uB, a.Config.TenantID, a.Config.UniverseID, b.Config.TenantID, b.Config.UniverseID)
	}
	if a.Events.Workspace() != wsA || b.Events.Workspace() != wsB {
		t.Fatalf("publishers' slots A=%q B=%q, subscribers derive %q %q", a.Events.Workspace(), b.Events.Workspace(), wsA, wsB)
	}

	chA, cancelA, err := a.EventsSubscribe(ctx, api.EventsSubscribeInput{TopicPattern: api.ProjectEventsPattern(tA, uA, wsA)})
	if err != nil {
		t.Fatal(err)
	}
	defer cancelA()
	chB, cancelB, err := b.EventsSubscribe(ctx, api.EventsSubscribeInput{TopicPattern: api.ProjectEventsPattern(tB, uB, wsB)})
	if err != nil {
		t.Fatal(err)
	}
	defer cancelB()

	// The same object id in the same module, in each repository.
	const id = "inbox.n1"
	for _, s := range []*Service{a, b} {
		if _, err := s.ObjectCreate(ctx, api.ObjectCreateInput{Module: "inbox", Object: treeObject(id, "")}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	subjA := api.ObjectCreatedSubject(tA, uA, wsA, api.SubjectToken(id))
	subjB := api.ObjectCreatedSubject(tB, uB, wsB, api.SubjectToken(id))
	if subjA == subjB {
		t.Fatalf("both repositories publish %s", subjA)
	}
	for _, c := range []struct {
		name string
		ch   <-chan api.EventEnvelope
		want string
	}{{"A", chA, subjA}, {"B", chB, subjB}} {
		got := drainQuiet(c.ch, 300*time.Millisecond)
		if len(got) != 1 || got[0].Topic != c.want {
			topics := make([]string, len(got))
			for i, e := range got {
				topics[i] = e.Topic
			}
			t.Errorf("subscriber of %s heard %v, want only %s", c.name, topics, c.want)
			continue
		}
		var p events.ObjectPayload
		if err := json.Unmarshal(got[0].Payload, &p); err != nil || p.ObjectID != id || p.Module != "inbox" {
			t.Errorf("subscriber of %s: payload %s (%v)", c.name, got[0].Payload, err)
		}
	}
}

// drainQuiet collects what a channel delivers until it stays quiet for quiet.
func drainQuiet(ch <-chan api.EventEnvelope, quiet time.Duration) []api.EventEnvelope {
	var out []api.EventEnvelope
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, e)
		case <-time.After(quiet):
			return out
		}
	}
}

// A store given its workspace slot keeps it: minting the project's id does
// not change what it was told.
func TestAGivenWorkspaceSlotIsKeptOverProjectInit(t *testing.T) {
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(), WorkspaceID: "given"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.ProjectInit(stdctx.Background(), api.ProjectInitInput{Name: "p"}); err != nil {
		t.Fatal(err)
	}
	if got := svc.Events.Workspace(); got != "given" {
		t.Errorf("slot %q after ProjectInit, want the given one", got)
	}
}
