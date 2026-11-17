// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/internal/core"
)

// stubProvider is a minimal ContributionProvider for Builder testing.
type stubProvider struct {
	provider string
	rtype    string
	entries  []contribapi.Contribution
}

func (p *stubProvider) Provider() string { return p.provider }
func (p *stubProvider) Type() string     { return p.rtype }
func (p *stubProvider) Enumerate(_ context.Context) ([]contribapi.Contribution, error) {
	return p.entries, nil
}
func (p *stubProvider) Identity() contribapi.Identity { return contribapi.Identity{} }
func (p *stubProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}

// declaring returns a provider that declares `kinds`, so this test exercises the
// real contract: publishing a kind requires a declaration naming its schema.
func declaring(provider string, kinds ...string) contribapi.ContributionProvider {
	var out []contribapi.Contribution
	for _, k := range kinds {
		out = append(out, contribapi.Contribution{
			Provider: provider, Type: "contribution-type", ID: k,
			Layers: contribapi.InlineData(json.RawMessage(`{"name":"` + k + `","schema":"` + provider + `:/data-type/` + k + `"}`)),
		})
	}
	return &stubProvider{provider: provider, rtype: "contribution-type", entries: out}
}

// TestBuilder_populatesContributions asserts that BuildFromProviders
// populates Contributions[] from every contributing provider, with URI
// derived from the producer's identity fields when unset.
func TestBuilder_populatesContributions(t *testing.T) {
	ctx := context.Background()
	providers := []contribapi.ContributionProvider{
		&stubProvider{
			provider: "test",
			rtype:    "skill",
			entries: []contribapi.Contribution{
				{Provider: "test", Type: "skill", ID: "alpha", Layers: contribapi.InlineData(json.RawMessage(`{"name":"Alpha"}`))},
				{Provider: "test", Type: "skill", ID: "beta", Layers: contribapi.InlineData(json.RawMessage(`{"name":"Beta"}`))},
			},
		},
		&stubProvider{
			provider: "test",
			rtype:    "task",
			entries: []contribapi.Contribution{
				{Provider: "test", Type: "task", ID: "do-thing", Layers: contribapi.InlineData(json.RawMessage(`{"type":"agent"}`))},
			},
		},
		declaring("test", "skill", "task"),
	}

	b := core.NewBuilder("test", "0.1.0")
	m, err := b.BuildFromProviders(ctx, providers)
	if err != nil {
		t.Fatalf("BuildFromProviders: %v", err)
	}

	// 3 content + the 2 kind declarations they require.
	if got, want := len(m.Contributions), 5; got != want {
		t.Fatalf("len(Contributions) = %d, want %d", got, want)
	}
	for i, c := range m.Contributions {
		// Builder derives URI when producer left it empty.
		if c.URI == "" {
			t.Errorf("index %d: URI empty (Builder should derive)", i)
		}
		if c.URI != c.ResolvedURI() {
			t.Errorf("index %d: URI %q != ResolvedURI %q", i, c.URI, c.ResolvedURI())
		}
	}
}

// TestBuilder_hashStable asserts that BuildFromProviders produces a
// deterministic hash across runs (canonical JSON ordering).
func TestBuilder_hashStable(t *testing.T) {
	ctx := context.Background()
	providers := []contribapi.ContributionProvider{
		&stubProvider{
			provider: "test",
			rtype:    "skill",
			entries: []contribapi.Contribution{
				{Provider: "test", Type: "skill", ID: "alpha", Layers: contribapi.InlineData(json.RawMessage(`{}`))},
			},
		},
		declaring("test", "skill"),
	}

	b := core.NewBuilder("test", "0.1.0")
	m1, _ := b.BuildFromProviders(ctx, providers)
	m2, _ := b.BuildFromProviders(ctx, providers)

	if m1.Hash == "" {
		t.Errorf("Hash empty")
	}
	if m1.Hash != m2.Hash {
		t.Errorf("Hash unstable: %q vs %q", m1.Hash, m2.Hash)
	}
}

// TestBuilder_hashChangesOnMetadataMutation asserts that mutating a
// Contribution's Metadata changes the manifest hash. Confirms
// Contributions (including Metadata) is in the canonical hash input.
func TestBuilder_hashChangesOnMetadataMutation(t *testing.T) {
	ctx := context.Background()
	providers := []contribapi.ContributionProvider{
		&stubProvider{
			provider: "test",
			rtype:    "skill",
			entries:  []contribapi.Contribution{{Provider: "test", Type: "skill", ID: "alpha", Layers: contribapi.InlineData(json.RawMessage(`{}`))}},
		},
		declaring("test", "skill"),
	}
	b := core.NewBuilder("test", "0.1.0")
	m, _ := b.BuildFromProviders(ctx, providers)
	originalHash := m.Hash

	yes := true
	m.Contributions[0].Metadata = &contribapi.ContributionMeta{Name: "Richer", Advertised: &yes}
	core.ReHash(m)
	if m.Hash == originalHash {
		t.Errorf("expected hash to change after mutating Contribution.Metadata, got same: %q", originalHash)
	}
}

// TestCatalog_bootProtocol exercises the Phase 1 → Phase 2 → ready
// sequence per the hub-boot-order ADR. Uses a temp Inventory.
func TestCatalog_bootProtocol(t *testing.T) {
	dir := t.TempDir()
	cache := core.NewCache(dir)

	// Phase 1: catalog created with no members + no schema.
	cat, err := core.NewCatalog(cache, nil)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	if cat.Ready() {
		t.Errorf("catalog should not be Ready before MarkReady")
	}

	// Reindex pre-SetSchema is a no-op (Phase 1 state).
	if err := cat.Reindex(context.Background(), nil); err != nil {
		t.Errorf("Reindex pre-SetSchema returned error: %v", err)
	}

	// Phase 2: attach schema, reindex, mark ready.
	stub := &stubSchema{}
	if err := cat.SetSchema(stub); err != nil {
		t.Fatalf("SetSchema: %v", err)
	}
	if err := cat.Reindex(context.Background(), nil); err != nil {
		t.Errorf("Reindex post-SetSchema returned error: %v", err)
	}
	if err := cat.MarkReady(context.Background()); err != nil {
		t.Errorf("MarkReady: %v", err)
	}
	if !cat.Ready() {
		t.Errorf("catalog should be Ready after MarkReady")
	}

	// Refresh path: re-Reindex with explicit URIs.
	if err := cat.Reindex(context.Background(), []string{"test:/skill/foo"}); err != nil {
		t.Errorf("Reindex partial returned error: %v", err)
	}
}

// stubSchema satisfies mbapi.CatalogSchemaService for tests.
type stubSchema struct{}

func (*stubSchema) GetDataTypeByURI(_ context.Context, _ string) (json.RawMessage, error) {
	return nil, nil
}
func (*stubSchema) ValidateBody(_ context.Context, _ string, _ json.RawMessage) error { return nil }

// TestCatalog_getContributionsByType asserts the universe-aggregation
// method returns Contributions matching the type filter across loaded
// manifests.
func TestCatalog_getContributionsByType(t *testing.T) {
	dir := t.TempDir()
	cache := core.NewCache(dir)

	m := &mbapi.Manifest{
		Provider: "test",
		Version:  "0.1.0",
		Contributions: []contribapi.Contribution{
			{Provider: "test", Type: "skill", ID: "alpha", URI: "test:/skill/alpha"},
		},
	}
	core.ReHash(m)
	if err := core.SeedCacheSave(t, cache, m); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	cat, err := core.NewCatalog(cache, core.MembersOf(cache, "test"))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	got, err := cat.GetContributionsByType(context.Background(), "skill")
	if err != nil {
		t.Fatalf("GetContributionsByType: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d Contributions, want 1", len(got))
	}
	if got[0].ID != "alpha" {
		t.Errorf("got Contribution.ID = %q, want %q", got[0].ID, "alpha")
	}
}
