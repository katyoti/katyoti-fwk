// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// fakeProvider is a minimal in-memory ContributionProvider for testing.
type fakeProvider struct {
	provider, resType string
	entries           []contribapi.Contribution
	enumerateErr      error
}

func (f *fakeProvider) Type() string     { return f.resType }
func (f *fakeProvider) Provider() string { return f.provider }
func (f *fakeProvider) Enumerate(_ context.Context) ([]contribapi.Contribution, error) {
	if f.enumerateErr != nil {
		return nil, f.enumerateErr
	}
	return f.entries, nil
}
func (f *fakeProvider) Identity() contribapi.Identity { return contribapi.Identity{} }
func (f *fakeProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, f, id, layer)
}

// declaring returns a provider that declares `kinds`, each naming its schema.
//
// Builder tests need it because publishing a kind now requires a declaration:
// a test that omitted one would be exercising a path the product refuses.
func declaring(provider string, kinds ...string) contribapi.ContributionProvider {
	var out []contribapi.Contribution
	for _, k := range kinds {
		out = append(out, contribapi.Contribution{
			Provider: provider, Type: "contribution-type", ID: k,
			Layers: contribapi.InlineData(json.RawMessage(`{"name":"` + k + `","schema":"` + provider + `:/data-type/` + k + `"}`)),
		})
	}
	return &fakeProvider{provider: provider, resType: "contribution-type", entries: out}
}

func TestBuilder_AggregatesProviders(t *testing.T) {
	body := func(s string) json.RawMessage { return json.RawMessage(`"` + s + `"`) }
	providers := []contribapi.ContributionProvider{
		&fakeProvider{
			provider: "ktw-test", resType: "skill",
			entries: []contribapi.Contribution{
				{Provider: "ktw-test", Type: "skill", ID: "intro", Layers: contribapi.InlineData(body("intro-body"))},
				{Provider: "ktw-test", Type: "skill", ID: "advanced", Layers: contribapi.InlineData(body("adv-body"))},
			},
		},
		&fakeProvider{
			provider: "ktw-test", resType: "task",
			entries: []contribapi.Contribution{
				{Provider: "ktw-test", Type: "task", ID: "deploy", Layers: contribapi.InlineData(body("deploy-body"))},
			},
		},
		declaring("ktw-test", "skill", "task"),
	}

	b := NewBuilder("ktw-test", "1.0.0")
	m, err := b.BuildFromProviders(context.Background(), providers)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if m.Provider != "ktw-test" {
		t.Errorf("Provider = %q", m.Provider)
	}
	if m.Version != "1.0.0" {
		t.Errorf("Version = %q", m.Version)
	}
	// 3 content contributions + the 2 kind declarations they require: a kind
	// with no declaration cannot be published, so declarations are part of what
	// a binary emits rather than test scaffolding.
	if len(m.Contributions) != 5 {
		t.Fatalf("got %d resources, want 5 (3 content + 2 kind declarations)", len(m.Contributions))
	}
	if m.Hash == "" || m.Hash[:7] != "sha256:" {
		t.Errorf("Hash = %q", m.Hash)
	}
}

func TestBuilder_DeterministicHash(t *testing.T) {
	body := func(s string) json.RawMessage { return json.RawMessage(`"` + s + `"`) }
	mk := func() []contribapi.ContributionProvider {
		return []contribapi.ContributionProvider{
			&fakeProvider{
				provider: "ktw-test", resType: "skill",
				entries: []contribapi.Contribution{
					{Provider: "ktw-test", Type: "skill", ID: "b", Layers: contribapi.InlineData(body("b"))},
					{Provider: "ktw-test", Type: "skill", ID: "a", Layers: contribapi.InlineData(body("a"))},
				},
			},
			declaring("ktw-test", "skill"),
		}
	}

	b := NewBuilder("ktw-test", "1.0.0")
	m1, _ := b.BuildFromProviders(context.Background(), mk())
	m2, _ := b.BuildFromProviders(context.Background(), mk())
	if m1.Hash != m2.Hash {
		t.Errorf("hashes differ: %q vs %q", m1.Hash, m2.Hash)
	}
}

func TestBuilder_SortsResources(t *testing.T) {
	body := func(s string) json.RawMessage { return json.RawMessage(`"` + s + `"`) }
	providers := []contribapi.ContributionProvider{
		&fakeProvider{
			provider: "ktw-z", resType: "skill",
			entries: []contribapi.Contribution{
				{Provider: "ktw-z", Type: "skill", ID: "x", Layers: contribapi.InlineData(body("x"))},
			},
		},
		&fakeProvider{
			provider: "ktw-a", resType: "task",
			entries: []contribapi.Contribution{
				{Provider: "ktw-a", Type: "task", ID: "y", Layers: contribapi.InlineData(body("y"))},
			},
		},
		&fakeProvider{
			provider: "ktw-a", resType: "skill",
			entries: []contribapi.Contribution{
				{Provider: "ktw-a", Type: "skill", ID: "z", Layers: contribapi.InlineData(body("z"))},
				{Provider: "ktw-a", Type: "skill", ID: "a", Layers: contribapi.InlineData(body("a"))},
			},
		},
		declaring("ktw-z", "skill", "task"),
	}

	b := NewBuilder("test-binary", "dev")
	m, err := b.BuildFromProviders(context.Background(), providers)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// 4 content + 2 kind declarations.
	if len(m.Contributions) != 6 {
		t.Fatalf("got %d, want 6 (4 content + 2 kind declarations)", len(m.Contributions))
	}
	// Expected sort: (provider asc, type asc, id asc)
	// Sorted by provider, then kind, then id — kind declarations included,
	// since they are contributions like any other and sort among them.
	want := []struct{ Provider, Type, ID string }{
		{"ktw-a", "skill", "a"},
		{"ktw-a", "skill", "z"},
		{"ktw-a", "task", "y"},
		{"ktw-z", "contribution-type", "skill"},
		{"ktw-z", "contribution-type", "task"},
		{"ktw-z", "skill", "x"},
	}
	for i, w := range want {
		got := m.Contributions[i]
		if got.Provider != w.Provider || got.Type != w.Type || got.ID != w.ID {
			t.Errorf("Resources[%d] = (%s,%s,%s), want (%s,%s,%s)", i,
				got.Provider, got.Type, got.ID, w.Provider, w.Type, w.ID)
		}
	}
}

func TestBuilder_EnumerateError_PropagatesWithContext(t *testing.T) {
	providers := []contribapi.ContributionProvider{
		&fakeProvider{provider: "p", resType: "skill", enumerateErr: errors.New("boom")},
		declaring("p", "skill"),
	}
	b := NewBuilder("test", "dev")
	_, err := b.BuildFromProviders(context.Background(), providers)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := "enumerate p/skill: boom"
	if err.Error() != want {
		t.Errorf("err = %q, want %q", err.Error(), want)
	}
}

func TestBuilder_EmptyProviders_EmptyManifest(t *testing.T) {
	b := NewBuilder("empty-binary", "dev")
	m, err := b.BuildFromProviders(context.Background(), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if m.Provider != "empty-binary" || m.Version != "dev" {
		t.Errorf("identity wrong: %+v", m)
	}
	if len(m.Contributions) != 0 {
		t.Errorf("got %d resources, want 0", len(m.Contributions))
	}
	if m.Hash == "" {
		t.Errorf("even empty manifests should have a hash")
	}
}
