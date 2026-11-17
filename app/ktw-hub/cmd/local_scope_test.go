// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// localEntry is what a member's SELF view carries and the aggregate never
// may: a `local:` service, binary-private by definition.
func localEntry() contribapi.Contribution {
	return contribapi.Contribution{Provider: contribapi.LocalProvider, Type: "service", ID: "sidecar"}
}

// localURIs lists every `local:` URI among entries — the finding, empty when
// the boundary held.
func localURIs(entries []contribapi.Contribution) []string {
	var out []string
	for _, c := range entries {
		if !contribapi.IsBusVisible(c.Provider) {
			out = append(out, c.ResolvedURI())
		}
	}
	return out
}

// TestHubNeverServesALocalEntry_EvenFromARecordThatCarriesOne — blind audit
// M3: a `local:` entry never crosses a process boundary, and the hub's list
// paths are that boundary for every consumer of the universe. The probe no
// longer asks a member for its local entries, but a tier-1 record written by
// an older hub into the SHARED cache still carries them — so a record is
// planted with one, for a member AND for the hub's own manifest, and every
// read path of the catalog service is asked: ListContributions, ListURIs,
// GetManifest, and the in-process by-type read. None may answer it.
func TestHubNeverServesALocalEntry_EvenFromARecordThatCarriesOne(t *testing.T) {
	cache := manifestlocal.NewCache(t.TempDir())
	bin := t.TempDir()
	sidecars := members(t, bin, "ktw-a")
	exe := sidecars[0].Binary

	// The member's record, as an older hub wrote it: its own entry plus a
	// local one, include_system false (a member record).
	m := &mbapi.Manifest{Provider: "ktw-a", Version: "1", Hash: "a-with-local"}
	m.Contributions = append(m.Contributions,
		contribapi.Contribution{Provider: "ktw-a", Type: "skill", ID: "own"}, localEntry())
	if err := cache.Save("ktw-a", m, exe, manifestlocal.SourceDigests{Binary: exe}); err != nil {
		t.Fatalf("plant the member record: %v", err)
	}

	// The hub's own record through the REAL writer, with a builder seam that
	// hands back a manifest carrying a local entry — what the ResourceService
	// would answer if it were asked with IncludeLocal left at true.
	hubExe := filepath.Join(bin, hubProvider+".exe")
	if err := os.WriteFile(hubExe, []byte(hubProvider), 0o644); err != nil {
		t.Fatal(err)
	}
	own := &mbapi.Manifest{Provider: hubProvider, Version: "1", Hash: "hub-with-local"}
	own.Contributions = append(own.Contributions,
		contribapi.Contribution{Provider: hubProvider, Type: "service", ID: "catalog-service"}, systemEntry(), localEntry())
	prevManifest, prevExe := hubOwnManifest, hubOwnExecutable
	hubOwnManifest = func(context.Context, bool) (*mbapi.Manifest, error) { return own, nil }
	hubOwnExecutable = func() (string, error) { return hubExe, nil }
	t.Cleanup(func() { hubOwnManifest, hubOwnExecutable = prevManifest, prevExe })
	if err := saveHubOwnManifest(context.Background(), cache); err != nil {
		t.Fatalf("save the hub's record: %v", err)
	}

	// The records DO carry the entry — the boundary is downstream of the
	// cache, and this is what the guard is falsified against.
	for _, member := range catalogMembers(sidecars) {
		rec, _, err := cache.Load(member.Name, member.Binary)
		if err != nil || rec == nil {
			t.Fatalf("load %s: %v (nil=%t)", member.Name, err, rec == nil)
		}
		if len(localURIs(rec.Contributions)) == 0 {
			t.Fatalf("%s record carries no local entry: the fixture does not plant what the test guards against", member.Name)
		}
	}

	cat, err := manifestlocal.NewCatalog(cache, catalogMembers(sidecars))
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	s := &catalogServiceImpl{cat: cat}
	ctx := context.Background()

	listed, err := s.ListContributions(ctx, mbapi.CatalogListContributionsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if bad := localURIs(listed.Contributions); len(bad) > 0 {
		t.Errorf("ListContributions serves local entries across the process boundary: %s", strings.Join(bad, ", "))
	}
	if len(listed.Contributions) != 3 { // ktw-a:/skill/own, ktw-hub:/service/catalog-service, system:/contribution-type/skill
		t.Errorf("ListContributions = %d entries, want the 3 universe-visible ones", len(listed.Contributions))
	}

	uris, err := s.ListURIs(ctx, mbapi.CatalogListURIsInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range uris.URIs {
		if !contribapi.IsBusVisible(u.Provider) {
			t.Errorf("ListURIs serves %s across the process boundary", u.String())
		}
	}

	for _, name := range []string{"ktw-a", hubProvider} {
		got, err := s.GetManifest(ctx, mbapi.GetManifestInput{Provider: name})
		if err != nil {
			t.Fatalf("GetManifest(%s): %v", name, err)
		}
		if bad := localURIs(got.Manifest.Contributions); len(bad) > 0 {
			t.Errorf("GetManifest(%s) serves local entries: %s", name, strings.Join(bad, ", "))
		}
		// The digest names what is served, not what the record carried.
		if got.Manifest.Hash == "a-with-local" || got.Manifest.Hash == "hub-with-local" {
			t.Errorf("GetManifest(%s).Hash = %q still names the record WITH the local entry", name, got.Manifest.Hash)
		}
	}

	byType, err := cat.GetContributionsByType(ctx, "service")
	if err != nil {
		t.Fatal(err)
	}
	if bad := localURIs(byType); len(bad) > 0 {
		t.Errorf("GetContributionsByType(service) holds local entries: %s", strings.Join(bad, ", "))
	}
}

// TestOlderRecordProbedWithLocalEntriesIsStale — the record's side of M3: a
// record of the previous format — one that may have been probed with local
// entries — is stale and re-probed rather than trusted, whatever binary stat
// it carries. The load-time narrowing above is the guard for a record the
// format cannot classify; this is the one that retires the old ones.
func TestOlderRecordProbedWithLocalEntriesIsStale(t *testing.T) {
	cache := manifestlocal.NewCache(t.TempDir())
	bin := t.TempDir()
	sidecars := members(t, bin, "ktw-a")
	exe := sidecars[0].Binary
	m := &mbapi.Manifest{Provider: "ktw-a", Version: "1", Hash: "a"}
	m.Contributions = append(m.Contributions, localEntry())
	if err := cache.Save("ktw-a", m, exe, manifestlocal.SourceDigests{Binary: exe}); err != nil {
		t.Fatal(err)
	}
	// Rewrite the record's format to the one that predates M3 — the record
	// lives at <Root>/<name>@<key>/metadata.yaml.
	metas, _ := filepath.Glob(filepath.Join(cache.Root, "ktw-a@*", "metadata.yaml"))
	if len(metas) != 1 {
		t.Fatalf("record folders for ktw-a: %v, want exactly one", metas)
	}
	metaPath := metas[0]
	b, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	old := strings.Replace(string(b), "format: 5", "format: 3", 1)
	if old == string(b) {
		t.Fatalf("metadata does not say format: 5:\n%s", b)
	}
	if err := os.WriteFile(metaPath, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	fresh, err := cacheIsFresh(cache, "ktw-a", exe, manifestlocal.SourceDigests{Binary: exe})
	if err != nil {
		t.Fatal(err)
	}
	if fresh {
		t.Error("a format-3 record (probed before members were asked without local entries) reads fresh; it must re-probe")
	}
}
