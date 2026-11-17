// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"embed"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	connmgrlocal "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/local"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// TestUniverseListsTheHubsOwnOfferings — e24 / N11: the hub's own
// `ktw-hub:/service/*` entries are in the universe view. The hub application is
// built from exactly the extension list that ships; its own manifest is
// recorded in the tier-1 cache the way the catalog factory records it, and the
// catalog over that cache and the hub's membership lists the hub's services at
// universe reach. The hub's record carries the system set (e26): the universe
// here has no binary member at all and still resolves every kind.
func TestUniverseListsTheHubsOwnOfferings(t *testing.T) {
	t.Setenv("KTW_WORKSPACE", t.TempDir())
	t.Setenv("KTW_DATA", t.TempDir())
	t.Setenv("KTW_CACHE", t.TempDir())

	var efs embed.FS
	app := application.New(application.Config(), buildExtensions(efs)...)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	ctx := application.WithApplication(context.Background(), app)

	cache := manifestlocal.NewCache(t.TempDir())
	if err := saveHubOwnManifest(ctx, cache); err != nil {
		t.Fatalf("saveHubOwnManifest: %v", err)
	}
	hubExe, _ := hubOwnExecutable()
	own, meta, err := cache.Load(hubProvider, hubExe)
	if err != nil || own == nil || meta == nil {
		t.Fatalf("the hub's own record is not in the cache: manifest=%v meta=%v err=%v", own, meta, err)
	}
	if !meta.IncludeSystem {
		t.Error("the hub's own record says include_system: false; the hub is the universe's one carrier of the system set")
	}
	systemKinds := map[string]int{}
	for _, c := range own.Contributions {
		if c.Provider == "system" {
			systemKinds[c.Type]++
		}
	}
	if systemKinds["contribution-type"] == 0 || systemKinds["data-type"] == 0 {
		t.Fatalf("the hub's own manifest carries system entries %v; want the system set (contribution-type and data-type)", systemKinds)
	}

	// A member universe: the hub's name is in the catalog's membership.
	sidecars := []appcore.SidecarSpec{{Name: "host-only"}}
	names := catalogMembers(sidecars)
	if names[len(names)-1].Name != hubProvider {
		t.Fatalf("catalogMembers = %v, want the hub last", names)
	}
	cat, err := manifestlocal.NewCatalog(cache, names)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	services, err := mbapi.Query(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: "service", Catalog: cat})
	if err != nil {
		t.Fatalf("Query services: %v", err)
	}
	var uris []string
	found := false
	for _, s := range services {
		uris = append(uris, s.ResolvedURI())
		if s.ResolvedURI() == "ktw-hub:/service/catalog-service" {
			found = true
		}
	}
	if !found {
		t.Fatalf("universe view services = %v, want ktw-hub:/service/catalog-service", uris)
	}

	// Kinds resolve at universe reach from the hub member: every kind it
	// declares - the system set and the pack kinds it publishes (skill,
	// task, persona) - is found, with nothing else in the universe.
	kinds, err := mbapi.KindsAt(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat})
	if err != nil {
		t.Fatalf("KindsAt: %v", err)
	}
	declared := 0
	for _, c := range own.Contributions {
		if c.Type != "contribution-type" || (c.Provider != "system" && c.Provider != hubProvider) {
			continue
		}
		declared++
		if _, ok := kinds.Get(c.ID); !ok {
			t.Errorf("kind %q does not resolve at universe reach", c.ID)
		}
	}
	for _, k := range []string{"skill", "task", "persona"} {
		if _, ok := kinds.Get(k); !ok {
			t.Errorf("pack kind %q does not resolve at universe reach with the hub alone", k)
		}
	}
	if kinds.Len() != declared {
		t.Errorf("universe reach resolves %d kinds, the hub declares %d", kinds.Len(), declared)
	}

	// The catalog forwards a read of an entry to its host; for the hub's own
	// entries the host is the hub, which the connection manager answers in
	// process instead of reporting "not in universe".
	mgr, err := withHubSelf(connmgrlocal.NewManager(nil))(app)
	if err != nil {
		t.Fatalf("connection manager: %v", err)
	}
	sender, err := mgr.Acquire(ctx, hubProvider)
	if err != nil {
		t.Fatalf("Acquire(%s): %v — the hub's own entries would be unreadable through the catalog", hubProvider, err)
	}
	var dataType string
	for _, c := range own.Contributions {
		if c.Type == "data-type" {
			dataType = c.ResolvedURI()
			break
		}
	}
	if dataType == "" {
		t.Fatal("the hub's own manifest carries no data-type to read")
	}
	raw, err := sender.Call(ctx, "resources/read", mbapi.ContributionReadInput{URI: dataType})
	if err != nil {
		t.Fatalf("resources/read %s on the hub itself: %v", dataType, err)
	}
	var read mbapi.ContributionReadOutput
	if err := json.Unmarshal(raw, &read); err != nil || len(read.Content) == 0 {
		t.Fatalf("resources/read %s: content=%q err=%v", dataType, read.Content, err)
	}

	// A second boot with nothing changed does not rewrite the record.
	before := meta.IndexedAt
	if err := saveHubOwnManifest(ctx, cache); err != nil {
		t.Fatalf("saveHubOwnManifest (again): %v", err)
	}
	if _, again, _ := cache.Load(hubProvider, hubExe); again == nil || !again.IndexedAt.Equal(before) {
		t.Error("an unchanged hub manifest was rewritten — every hub command would churn the cache and wake its watchers")
	}
}
