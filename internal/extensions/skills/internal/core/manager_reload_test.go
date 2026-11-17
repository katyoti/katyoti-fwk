// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"io"
	"sync"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// mutableProvider is a test fake that satisfies loaderBackedProvider and
// wraps a MapLoader whose contents can be mutated between scans — so the
// Reload test can simulate a sidecar producing a new SKILL.md without
// rebuilding the whole provider graph.
type mutableProvider struct {
	provider string
	files    map[string]string // path → bytes
	loader   *loader.MapLoader
}

func newMutableProvider(name string, initial map[string]string) *mutableProvider {
	files := map[string]string{}
	for k, v := range initial {
		files[k] = v
	}
	return &mutableProvider{
		provider: name,
		files:    files,
		loader:   loader.NewMapLoader(files),
	}
}

func (p *mutableProvider) Type() string     { return mbapi.ResourceTypeSkill }
func (p *mutableProvider) Provider() string { return p.provider }
func (p *mutableProvider) Enumerate(_ context.Context) ([]contribapi.Contribution, error) {
	return nil, nil // not exercised by SkillManager scan; safe to noop
}
func (p *mutableProvider) Identity() contribapi.Identity { return contribapi.Identity{Mutable: true} }
func (p *mutableProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}
func (p *mutableProvider) SourceLoader() loader.Loader { return p.loader }
func (p *mutableProvider) SourceRoot() string          { return "." }

// addSkill mutates the underlying file map — the loader sees the new
// file on the next scan because MapLoader reads from the shared map.
func (p *mutableProvider) addSkill(filename, body string) {
	p.files[filename] = body
}

// ctxWithProvider builds an Application context that exposes `prov` via
// `application.UseExtensions[contribapi.ContributionProvider]`. Threads the
// standard Phase 1 (register point) → Phase 2 (contribute) lifecycle by
// wrapping the registration + contribution in an Extension and running
// the application.
//
// We register a fresh `em.NewListPoint[contribapi.ContributionProvider]()`
// here rather than importing `manifest/internal/core` — Go's `internal/`
// rule prevents the cross-package import, and the EM keys points by Go
// type so a locally-built point is indistinguishable from the production
// one as far as `application.UseExtensions[T]` is concerned.
func ctxWithProvider(t *testing.T, prov *mutableProvider) context.Context {
	t.Helper()
	ext := func(init *application.ApplicationInit) {
		init.AddRegisterPoints("skills.reload_test", ".point", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
		})
		init.AddContribute("skills.reload_test", ".provider", func(app *application.Application) error {
			return application.Contribute(app, contribapi.ContributionProvider(prov))
		})
	}
	app := application.New(application.Config(), ext)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	return application.WithApplication(context.Background(), app)
}

// minimalSKILL returns a valid SKILL.md body with the given name + a
// trivial description. Used by Reload tests to inject "new" skills.
func minimalSKILL(name string) string {
	return "---\nname: " + name + "\ndescription: \"test skill " + name + "\"\n---\n# " + name + "\n\nBody.\n"
}

// TestSkillManager_Reload_PicksUpNewSkill — the headline behaviour:
// add a SKILL.md after construction, call Reload, assert the new
// skill is visible via GetSkill (and the old one still is). URI form is
// `<provider>:/skill/<name>` per the ms-rc1 URI normalisation.
func TestSkillManager_Reload_PicksUpNewSkill(t *testing.T) {
	prov := newMutableProvider("test", map[string]string{
		"flow-spec.md": minimalSKILL("flow-spec"),
	})
	ctx := ctxWithProvider(t, prov)

	mgr, err := NewSkillManager(ctx, &SkillManagerConfig{})
	if err != nil {
		t.Fatalf("NewSkillManager: %v", err)
	}

	// Initial state: only flow-spec.
	if _, err := mgr.GetSkill("test:/skill/flow-spec"); err != nil {
		t.Fatalf("pre-Reload GetSkill(flow-spec): %v", err)
	}
	if _, err := mgr.GetSkill("test:/skill/doc-create"); err == nil {
		t.Fatal("pre-Reload: doc-create should not exist yet")
	}

	// Simulate a user adding a new skill on disk.
	prov.addSkill("doc-create.md", minimalSKILL("doc-create"))

	if err := mgr.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	// Post-Reload: both skills visible.
	if _, err := mgr.GetSkill("test:/skill/flow-spec"); err != nil {
		t.Errorf("post-Reload GetSkill(flow-spec): %v", err)
	}
	if _, err := mgr.GetSkill("test:/skill/doc-create"); err != nil {
		t.Errorf("post-Reload GetSkill(doc-create): %v (the new skill should be visible)", err)
	}
}

// TestSkillManager_Reload_DropsRemovedSkill — Reload reflects deletions
// too. A skill that disappeared from the provider's loader should no
// longer resolve.
func TestSkillManager_Reload_DropsRemovedSkill(t *testing.T) {
	prov := newMutableProvider("test", map[string]string{
		"a.md": minimalSKILL("a"),
		"b.md": minimalSKILL("b"),
	})
	ctx := ctxWithProvider(t, prov)

	mgr, err := NewSkillManager(ctx, &SkillManagerConfig{})
	if err != nil {
		t.Fatalf("NewSkillManager: %v", err)
	}
	if _, err := mgr.GetSkill("test:/skill/a"); err != nil {
		t.Fatalf("pre-Reload GetSkill(a): %v", err)
	}

	// Drop "a" from disk.
	delete(prov.files, "a.md")

	if err := mgr.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if _, err := mgr.GetSkill("test:/skill/a"); err == nil {
		t.Error("post-Reload: a should no longer be findable (deleted from provider)")
	}
	if _, err := mgr.GetSkill("test:/skill/b"); err != nil {
		t.Errorf("post-Reload GetSkill(b): %v (untouched skill should still resolve)", err)
	}
}

// TestSkillManager_Reload_ConcurrentReadersDontRace — readers calling
// GetSkill concurrently with Reload must NOT panic or data-race. Run
// under `go test -race`; the RWMutex discipline is what saves us.
func TestSkillManager_Reload_ConcurrentReadersDontRace(t *testing.T) {
	prov := newMutableProvider("test", map[string]string{
		"s1.md": minimalSKILL("s1"),
	})
	ctx := ctxWithProvider(t, prov)

	mgr, err := NewSkillManager(ctx, &SkillManagerConfig{})
	if err != nil {
		t.Fatalf("NewSkillManager: %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Burst of concurrent readers.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = mgr.GetSkill("test:/skill/s1")
				}
			}
		}()
	}

	// Hammer Reload while readers run.
	for i := 0; i < 50; i++ {
		if err := mgr.Reload(ctx); err != nil {
			t.Errorf("Reload[%d]: %v", i, err)
		}
	}

	close(stop)
	wg.Wait()
}
