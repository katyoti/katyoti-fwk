// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storeset"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/spf13/cobra"
)

// memberRepository makes a repository holding a project object of key and
// the module folders given.
func memberRepository(t *testing.T, root, key string, modules ...string) string {
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

// openUniverse runs an application whose CWM service is opened over the
// universe at cfgRoot, attaching the members given; its service.
func openUniverse(t *testing.T, root string, members ...string) cwmapi.CWMService {
	t.Helper()
	return openUniverseWith(t, root, nil, members...)
}

// openUniverseWith is openUniverse with the extensions extra beside CWM's.
func openUniverseWith(t *testing.T, root string, extra []application.Extension, members ...string) cwmapi.CWMService {
	t.Helper()
	cfgRoot := application.EnvOf(filepath.Join(root, "universe")).HomeDir()
	if err := os.MkdirAll(cfgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := "projects:\n"
	for _, m := range members {
		reg += "  - " + filepath.ToSlash(m) + "\n"
	}
	if err := os.WriteFile(filepath.Join(cfgRoot, cenv.AttachedProjectsFile), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
	all := []application.Extension{
		func(init *application.ApplicationInit) {
			init.AddContribute("test", ".root", func(app *application.Application) error {
				application.PutService[appcore.CobraCommand](app, "root", appcore.CobraCommand{Cmd: &cobra.Command{Use: "ktw-test"}})
				return nil
			})
		},
		manifestlocal.Extension(),
		ExtensionWith(Options{WorkspaceRoot: cfgRoot, ObjectTypeValidation: cwmapi.TypeValidationWarn}),
	}
	all = append(all, extra...)
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), all...)
	app.SetProvider("ktw-test")
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	svc, err := application.UseService[cwmapi.CWMService](app.Context())
	if err != nil {
		t.Fatalf("resolve CWMService: %v", err)
	}
	t.Cleanup(func() {
		if c, ok := svc.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	})
	return svc
}

func noteIn(id string) cwmapi.Object {
	o := cwmapi.Object{ID: id, Type: "note"}
	o.Name = id
	return o
}

// THE CWM SERVICE SERVES EVERY ATTACHED MEMBER.
// A universe attaching two repositories - each its project object and its
// modules - is served by one store per member behind the one api.CWMService:
// an object of each read by bare id (the only member holding it) and by
// qualified id <key>:<id>, both listed and searched with each object's
// project named, a create landing in the member holding the module, a bare
// id both hold refused naming both keys. One attached member still gets the
// plain single store.
func TestTheServiceServesEveryAttachedMember(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(root, "installation"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	ctx := context.Background()

	alpha := memberRepository(t, root, "alpha", "crm", "shared")
	beta := memberRepository(t, root, "beta", "erp", "shared")
	svc := openUniverse(t, root, alpha, beta)
	if _, ok := svc.(*storeset.Served); !ok {
		t.Fatalf("two members are served by %T, want the store set's front", svc)
	}

	// A create lands in the member holding the module; a module both hold is
	// named with its project.
	for _, c := range []struct{ module, id string }{
		{"crm", "crm.N-1"}, {"erp", "erp.N-1"}, {"alpha:shared", "shared.N-1"}, {"beta:shared", "shared.N-1"},
	} {
		if _, err := svc.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: c.module, Object: noteIn(c.id)}); err != nil {
			t.Fatalf("create %s in %s: %v", c.id, c.module, err)
		}
	}
	if _, err := svc.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "shared", Object: noteIn("shared.N-2")}); err == nil ||
		!strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "beta") {
		t.Errorf("create in a module both members hold = %v; want refused naming both", err)
	}
	for _, f := range []string{
		filepath.Join(alpha, "modules", "crm"), filepath.Join(beta, "modules", "erp"),
	} {
		if n := countObjectFiles(t, f); n != 1 {
			t.Errorf("%s holds %d object files, want 1", f, n)
		}
	}
	if n := countObjectFiles(t, filepath.Join(alpha, "modules", "erp")); n != 0 {
		t.Errorf("an object of beta was written in alpha")
	}

	// Read by bare id and by qualified id.
	for _, id := range []string{"crm.N-1", "erp.N-1", "alpha:crm.N-1", "beta:erp.N-1", "beta:shared.N-1"} {
		got, err := svc.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: id})
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if want := id[strings.Index(id, ":")+1:]; got.Object.ID != want {
			t.Errorf("get %s = %s", id, got.Object.ID)
		}
	}
	_, err := svc.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: "shared.N-1"})
	if err == nil || !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "beta") {
		t.Errorf("get a bare id both members hold = %v; want refused naming both", err)
	}

	// Listed and searched across both, each object's project named.
	list, err := svc.ObjectList(ctx, cwmapi.ObjectListInput{Type: "note", Limit: -1})
	if err != nil {
		t.Fatal(err)
	}
	if got := located(list.Objects, list.Projects); got != "alpha:crm.N-1 beta:erp.N-1 alpha:shared.N-1 beta:shared.N-1" {
		t.Errorf("list = %s", got)
	}
	found, err := svc.ObjectSearch(ctx, cwmapi.ObjectSearchInput{Query: "N-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := located(found.Objects, found.Projects); !strings.Contains(got, "alpha:crm.N-1") || !strings.Contains(got, "beta:erp.N-1") {
		t.Errorf("search = %s; want both members' objects, each with its project", got)
	}

	// One member: the plain single store, as always.
	solo := memberRepository(t, filepath.Join(root, "solo"), "gamma", "crm")
	if svc := openUniverse(t, filepath.Join(root, "solo"), solo); svc == nil {
		t.Fatal("no service")
	} else if _, ok := svc.(*storeset.Served); ok {
		t.Errorf("one member is served through the store set; want the plain store")
	}
}

func located(objs []cwmapi.Object, projects []string) string {
	if len(projects) != len(objs) {
		return "projects not named per object"
	}
	var parts []string
	for i, o := range objs {
		parts = append(parts, projects[i]+":"+o.ID)
	}
	return strings.Join(parts, " ")
}

func countObjectFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".cwm.yaml") {
			n++
		}
		return nil
	})
	return n
}

// A MEMBER CREATED BY THE HUB JOINS BESIDE ANOTHER. A project
// the hub creates holds only the seed of its project object in its store
// until its first opening. Attached beside a member, its store
// mints that object from the seed - journaled, the observers attached first -
// before the store set joins it under the seed's name, which cwmapi.ProjectKeyOf
// reads before the opening: the CWM service opens, both members served.
func TestASeededMemberIsMintedBeforeTheStoreSetJoinsIt(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(root, "installation"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	ctx := context.Background()

	alpha := memberRepository(t, root, "alpha", "crm")
	// A hub-created project: a folder whose store holds only the seed, named
	// apart from its folder so the key read is the seed's.
	seeded := filepath.Join(root, "new-folder")
	store := application.EnvOf(seeded).HomeDir()
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, cwmapi.ProjectSeedFile), []byte("name: delta\ntitle: Delta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := cwmapi.ProjectKeyOf(seeded); got != "delta" {
		t.Fatalf("ProjectKeyOf(seed-only member) = %q, want the seed's name delta", got)
	}
	if got := cwmapi.ProjectKeyOf(alpha); got != "alpha" {
		t.Fatalf("ProjectKeyOf(member with a project object) = %q, want alpha", got)
	}

	var mu sync.Mutex
	var seen []string
	observer := cwmapi.ObserveEvents(cwmapi.EventObserver{Name: "test-mint", Handle: func(_ context.Context, ev cwmapi.EventEnvelope) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, ev.Topic+" "+string(ev.Payload))
	}})
	// The in-process bus CWM publishes on (a host binary registers it).
	bus := application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
		return eventbus.NewMemoryEventBus(), nil
	})
	svc := openUniverseWith(t, root, []application.Extension{bus, observer}, seeded, alpha)
	if _, ok := svc.(*storeset.Served); !ok {
		t.Fatalf("two members are served by %T, want the store set's front", svc)
	}
	f, err := projectfile.Read(seeded)
	if err != nil || f.ID != "delta" || f.Name != "Delta" || f.UUID == "" {
		t.Fatalf("the seeded member's project object = %+v (%v); want minted from the seed: id delta, name Delta, a uuid", f, err)
	}
	if _, err := os.Stat(filepath.Join(store, cwmapi.ProjectSeedFile)); !os.IsNotExist(err) {
		t.Errorf("the seed is kept once minted (%v)", err)
	}
	if got := cwmapi.ProjectKeyOf(seeded); got != "delta" {
		t.Errorf("ProjectKeyOf once minted = %q, want delta", got)
	}
	// Both members are served under their keys.
	if _, err := svc.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "alpha:crm", Object: noteIn("crm.N-1")}); err != nil {
		t.Fatalf("create in alpha:crm: %v", err)
	}
	got, err := svc.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: "delta:delta"})
	if err != nil || got.Object.ID != "delta" {
		t.Fatalf("get delta:delta (the minted project object) = %+v, %v", got.Object, err)
	}
	// The mint is journaled: the observers were attached before it. The
	// seeded member is attached first, so no other store's observers (they
	// share the bus) are there yet to see it.
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		journaled := false
		for _, e := range seen {
			if strings.Contains(e, `"object_id":"delta"`) {
				journaled = true
			}
		}
		mu.Unlock()
		if journaled {
			break
		}
		if time.Now().After(deadline) {
			mu.Lock()
			t.Fatalf("the seeded member's project object was minted unobserved; events seen: %v", seen)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
