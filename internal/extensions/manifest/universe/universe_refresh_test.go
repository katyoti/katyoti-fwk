// SPDX-License-Identifier: Apache-2.0

package universe_test

import (
	"context"
	"errors"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	manifestuniverse "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/universe"
)

// These exercise the event-driven incremental-refresh dispatcher
// (RefreshOnContributionsChanged) in isolation — no live hub, no Application.
// Part of contribution-event-incremental-refresh (ST-8 verification).

func manifest(provider string, ids ...string) *mbapi.Manifest {
	m := &mbapi.Manifest{Provider: provider}
	for _, id := range ids {
		m.Contributions = append(m.Contributions, contribapi.Contribution{
			Provider: provider, Type: "node-type", ID: id,
		})
	}
	return m
}

func TestRefresh_PerProvider_PatchesOnlyThatEntry(t *testing.T) {
	ctx := context.Background()
	local := manifest("ktw-agents", "workspace")
	cat := manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{
		local,
		manifest("ktw-methods", "old"),
		manifest("ktw-code", "untouched"),
	}, nil)

	// The hub now reports ktw-methods with a NEW contribution.
	fetchAll := func(context.Context) ([]*mbapi.Manifest, error) {
		return []*mbapi.Manifest{
			manifest("ktw-methods", "fresh"),
			manifest("ktw-code", "untouched"),
		}, nil
	}

	manifestuniverse.RefreshOnContributionsChanged(ctx, fetchAll, cat, []string{"ktw-methods", "ktw-code"}, local, nil,
		contribapi.ContributionsChangedEvent{Provider: "ktw-methods", KindsChanged: []string{"node-type"}})

	// ktw-methods patched.
	if got := idsFor(t, cat, "ktw-methods"); len(got) != 1 || got[0] != "fresh" {
		t.Errorf("ktw-methods = %v, want [fresh]", got)
	}
	// ktw-code untouched, ktw-agents (local) still present.
	if got := idsFor(t, cat, "ktw-code"); len(got) != 1 || got[0] != "untouched" {
		t.Errorf("ktw-code = %v, want [untouched]", got)
	}
	if _, err := cat.Get(ctx, "ktw-agents"); err != nil {
		t.Errorf("local manifest lost after per-provider refresh: %v", err)
	}
}

func TestRefresh_Whole_ReplacesAndPreservesLocal(t *testing.T) {
	ctx := context.Background()
	local := manifest("ktw-agents", "workspace")
	cat := manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{local, manifest("ktw-methods", "old")}, nil)

	// Hub reports a brand-new sidecar + drops ktw-methods.
	fetchAll := func(context.Context) ([]*mbapi.Manifest, error) {
		return []*mbapi.Manifest{manifest("ktw-newpack", "shiny")}, nil
	}

	manifestuniverse.RefreshOnContributionsChanged(ctx, fetchAll, cat, []string{"ktw-newpack"}, local, nil,
		contribapi.ContributionsChangedEvent{Provider: "", KindsChanged: []string{"*"}})

	if got := idsFor(t, cat, "ktw-newpack"); len(got) != 1 || got[0] != "shiny" {
		t.Errorf("ktw-newpack = %v, want [shiny]", got)
	}
	// Local manifest re-appended on whole Replace.
	if _, err := cat.Get(ctx, "ktw-agents"); err != nil {
		t.Errorf("local manifest not preserved on whole Replace: %v", err)
	}
	// Dropped provider gone.
	if _, err := cat.Get(ctx, "ktw-methods"); !errors.Is(err, mbapi.ErrSidecarNotInCatalog) {
		t.Errorf("ktw-methods should be gone after whole Replace; got %v", err)
	}
}

func TestRefresh_SelfProvider_RePutsLocalWithoutFetch(t *testing.T) {
	ctx := context.Background()
	local := manifest("ktw-agents", "workspace")
	cat := manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{local}, nil)

	fetchCalled := false
	fetchAll := func(context.Context) ([]*mbapi.Manifest, error) {
		fetchCalled = true
		return nil, nil
	}
	manifestuniverse.RefreshOnContributionsChanged(ctx, fetchAll, cat, nil, local, nil,
		contribapi.ContributionsChangedEvent{Provider: "ktw-agents", KindsChanged: []string{"skill"}})

	if fetchCalled {
		t.Error("self-provider event should NOT hit the hub fetch")
	}
	if _, err := cat.Get(ctx, "ktw-agents"); err != nil {
		t.Errorf("local manifest missing after self re-Put: %v", err)
	}
}

func TestRefresh_HookFanout_KindFiltered(t *testing.T) {
	ctx := context.Background()
	cat := manifestlocal.NewMemoryCatalog(nil, nil)
	fetchAll := func(context.Context) ([]*mbapi.Manifest, error) { return nil, nil }

	var fired []string
	hooks := []mbapi.RefreshHook{
		{Name: "skills", Kinds: []string{"skill"}, Refresh: func(context.Context, contribapi.ContributionsChangedEvent) error {
			fired = append(fired, "skills")
			return nil
		}},
		{Name: "cwm", Kinds: []string{"node-type"}, Refresh: func(context.Context, contribapi.ContributionsChangedEvent) error {
			fired = append(fired, "cwm")
			return nil
		}},
		{Name: "boom", Kinds: []string{"node-type"}, Refresh: func(context.Context, contribapi.ContributionsChangedEvent) error {
			fired = append(fired, "boom")
			return errors.New("hook error must not abort others")
		}},
	}

	manifestuniverse.RefreshOnContributionsChanged(ctx, fetchAll, cat, nil, nil, hooks,
		contribapi.ContributionsChangedEvent{Provider: "ktw-methods", KindsChanged: []string{"node-type"}})

	// Only node-type hooks fire; the skill hook is filtered out; a hook error
	// doesn't stop the rest.
	if len(fired) != 2 || fired[0] != "cwm" || fired[1] != "boom" {
		t.Errorf("fired hooks = %v, want [cwm boom]", fired)
	}
}

func idsFor(t *testing.T, cat *manifestlocal.MemoryCatalog, provider string) []string {
	t.Helper()
	m, err := cat.Get(context.Background(), provider)
	if err != nil {
		return nil
	}
	var out []string
	for _, c := range m.Contributions {
		out = append(out, c.ID)
	}
	return out
}
