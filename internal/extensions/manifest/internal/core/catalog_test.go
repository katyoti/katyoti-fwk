// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

func TestCatalog_EmptyMembers(t *testing.T) {
	cache := NewCache(t.TempDir())
	c, err := NewCatalog(cache, nil)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	ms, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ms) != 0 {
		t.Errorf("List = %d, want 0", len(ms))
	}
	names, _ := c.Names(context.Background())
	if len(names) != 0 {
		t.Errorf("Names = %d, want 0", len(names))
	}
	_, err = c.Get(context.Background(), "ktw-docs")
	if !errors.Is(err, mbapi.ErrSidecarNotInCatalog) {
		t.Errorf("Get unknown should return ErrSidecarNotInCatalog, got %v", err)
	}
}

func TestCatalog_LoadsCachedManifest(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)

	// Seed the inventory with one manifest for ktw-docs.
	docs := makeManifest("ktw-docs", "1.0.0", "sha256:abc")
	docs.Contributions = []contribapi.Contribution{
		{Provider: "ktw-docs", Type: "skill", ID: "doc-create"},
		{Provider: "ktw-docs", Type: "skill", ID: "doc-translate"},
		{Provider: "ktw-docs", Type: "config-program", ID: "pandoc"},
	}
	if err := SeedCacheSave(t, cache, docs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Membership lists ktw-docs and ktw-example; only ktw-docs has a cache.
	c, err := NewCatalog(cache, MembersOf(cache, "ktw-docs", "ktw-example"))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	// Names returns full membership.
	names, _ := c.Names(context.Background())
	if len(names) != 2 || names[0] != "ktw-docs" || names[1] != "ktw-example" {
		t.Errorf("Names = %v, want [ktw-docs ktw-example]", names)
	}

	// List returns only loaded manifests.
	ms, _ := c.List(context.Background())
	if len(ms) != 1 || ms[0].Provider != "ktw-docs" {
		t.Errorf("List = %v, want one ktw-docs manifest", ms)
	}

	// Get(ktw-docs) succeeds.
	m, err := c.Get(context.Background(), "ktw-docs")
	if err != nil {
		t.Fatalf("Get(ktw-docs): %v", err)
	}
	if m.Hash != "sha256:abc" {
		t.Errorf("Hash = %q, want sha256:abc", m.Hash)
	}

	// Get(ktw-example) returns ErrSidecarNotInCatalog.
	_, err = c.Get(context.Background(), "ktw-example")
	if !errors.Is(err, mbapi.ErrSidecarNotInCatalog) {
		t.Errorf("Get(ktw-example): expected ErrSidecarNotInCatalog, got %v", err)
	}

	// GetContributionsByType(skill) returns 2 entries.
	skills, _ := c.GetContributionsByType(context.Background(), "skill")
	if len(skills) != 2 {
		t.Errorf("GetContributionsByType(skill) = %d, want 2", len(skills))
	}

	// GetContributionsByType(config-program) returns 1 entry.
	progs, _ := c.GetContributionsByType(context.Background(), "config-program")
	if len(progs) != 1 || progs[0].ID != "pandoc" {
		t.Errorf("GetContributionsByType(config-program) = %v", progs)
	}
}

func TestCatalog_GetResources_TypedDecode(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)

	type configFile struct {
		HasDefault bool `json:"has_default"`
	}
	body, _ := json.Marshal(configFile{HasDefault: true})

	docs := makeManifest("ktw-docs", "1.0.0", "sha256:abc")
	docs.Contributions = []contribapi.Contribution{
		{Provider: "ktw-docs", Type: "config-file", ID: "DocService.yaml", Layers: contribapi.InlineData(body)},
	}
	if err := SeedCacheSave(t, cache, docs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	c, err := NewCatalog(cache, MembersOf(cache, "ktw-docs"))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	decoded, err := mbapi.QueryTyped[configFile](context.Background(),
		mbapi.Q{Reach: mbapi.ReachUniverse, Kind: "config-file", Catalog: c})
	if err != nil {
		t.Fatalf("QueryTyped: %v", err)
	}
	if len(decoded) != 1 || !decoded[0].Body.HasDefault {
		t.Errorf("QueryTyped = %+v", decoded)
	}
}

func TestCatalog_Reload_PicksUpNewFile(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)

	c, err := NewCatalog(cache, MembersOf(cache, "ktw-docs"))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	// Initially nothing.
	ms, _ := c.List(context.Background())
	if len(ms) != 0 {
		t.Fatalf("expected empty catalog, got %d", len(ms))
	}

	// Drop a file then reload.
	if err := SeedCacheSave(t, cache, makeManifest("ktw-docs", "1.0.0", "sha256:fresh")); err != nil {
		t.Fatal(err)
	}
	if err := c.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	ms, _ = c.List(context.Background())
	if len(ms) != 1 {
		t.Errorf("after Reload: List = %d, want 1", len(ms))
	}
}

func TestCatalog_SetMembers(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)
	if err := SeedCacheSave(t, cache, makeManifest("ktw-docs", "1.0.0", "sha256:a")); err != nil {
		t.Fatal(err)
	}
	if err := SeedCacheSave(t, cache, makeManifest("ktw-example", "1.0.0", "sha256:b")); err != nil {
		t.Fatal(err)
	}

	c, err := NewCatalog(cache, MembersOf(cache, "ktw-docs"))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	if ms, _ := c.List(context.Background()); len(ms) != 1 {
		t.Errorf("List with one member = %d, want 1", len(ms))
	}

	if err := c.SetMembers(MembersOf(cache, "ktw-docs", "ktw-example")); err != nil {
		t.Fatalf("SetMembers: %v", err)
	}
	ms, _ := c.List(context.Background())
	if len(ms) != 2 {
		t.Errorf("After SetMembers: List = %d, want 2", len(ms))
	}
}
