// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSidecarsYaml_LoadMissingReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sidecars.yaml")
	s, err := LoadSidecarsYaml(path)
	if err != nil {
		t.Fatalf("LoadSidecarsYaml(missing): %v", err)
	}
	if len(s.Sidecars) != 0 {
		t.Fatalf("expected empty sidecars, got %d", len(s.Sidecars))
	}
	if s.Version != 1 {
		t.Errorf("Version = %d, want 1", s.Version)
	}
}

// existingBinary places a file standing for an installed sidecar's binary under
// dir and answers its path. Loading sidecars.yaml forgets an entry whose files
// are gone (cenv.Vanished), and a rooted path such as /usr/local/bin/x is
// absolute on Linux: a fixture that must survive a reload names a real file.
func existingBinary(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, "bin", name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSidecarsYaml_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sidecars.yaml")
	want := &SidecarsYaml{
		Version: 1,
		Sidecars: []SidecarsYamlEntry{
			{Name: "ktw-docs", BinaryPath: existingBinary(t, dir, "ktw-docs"), Version: "1.0.0", Source: "PATH", LastSeen: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)},
			{Name: "ktw-example", BinaryPath: existingBinary(t, dir, "ktw-example"), Source: "PATH"},
		},
	}
	if err := SaveSidecarsYaml(path, want); err != nil {
		t.Fatalf("SaveSidecarsYaml: %v", err)
	}
	got, err := LoadSidecarsYaml(path)
	if err != nil {
		t.Fatalf("LoadSidecarsYaml: %v", err)
	}
	if len(got.Sidecars) != 2 {
		t.Fatalf("Sidecars: got %d entries, want 2", len(got.Sidecars))
	}
	if got.Sidecars[0].Name != "ktw-docs" {
		t.Errorf("Sidecars[0].Name = %q, want ktw-docs", got.Sidecars[0].Name)
	}
	if !got.Sidecars[0].LastSeen.Equal(want.Sidecars[0].LastSeen) {
		t.Errorf("LastSeen round-trip mismatch: got %v, want %v", got.Sidecars[0].LastSeen, want.Sidecars[0].LastSeen)
	}
}

func TestMergeInventory_NewEntries(t *testing.T) {
	existing := &SidecarsYaml{Version: 1}
	fresh := []SidecarsYamlEntry{
		{Name: "ktw-docs", BinaryPath: "/a/ktw-docs"},
		{Name: "ktw-example", BinaryPath: "/a/ktw-example"},
	}
	merged, newly := MergeInventory(existing, fresh)
	if len(merged.Sidecars) != 2 {
		t.Fatalf("merged size = %d, want 2", len(merged.Sidecars))
	}
	if len(newly) != 2 {
		t.Fatalf("newly count = %d, want 2", len(newly))
	}
	// Newly should be sorted alphabetically.
	if newly[0] != "ktw-docs" || newly[1] != "ktw-example" {
		t.Errorf("newly order: got %v, want [ktw-docs ktw-example]", newly)
	}
	// LastSeen must be set on all entries.
	for _, e := range merged.Sidecars {
		if e.LastSeen.IsZero() {
			t.Errorf("LastSeen zero for %s after merge", e.Name)
		}
	}
}

func TestMergeInventory_PreservesAbsentEntries(t *testing.T) {
	old := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	existing := &SidecarsYaml{
		Version: 1,
		Sidecars: []SidecarsYamlEntry{
			{Name: "ktw-old", BinaryPath: "/old/path", LastSeen: old},
		},
	}
	fresh := []SidecarsYamlEntry{
		{Name: "ktw-new", BinaryPath: "/new/path"},
	}
	merged, newly := MergeInventory(existing, fresh)
	if len(merged.Sidecars) != 2 {
		t.Fatalf("merged size = %d, want 2 (old + new preserved)", len(merged.Sidecars))
	}
	// Find old entry; LastSeen must NOT be bumped.
	var foundOld bool
	for _, e := range merged.Sidecars {
		if e.Name == "ktw-old" {
			foundOld = true
			if !e.LastSeen.Equal(old) {
				t.Errorf("ktw-old LastSeen bumped: got %v, want %v", e.LastSeen, old)
			}
		}
	}
	if !foundOld {
		t.Fatalf("ktw-old not preserved in merge")
	}
	if len(newly) != 1 || newly[0] != "ktw-new" {
		t.Errorf("newly = %v, want [ktw-new]", newly)
	}
}

func TestMergeInventory_BumpsLastSeenOnRescan(t *testing.T) {
	old := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	existing := &SidecarsYaml{
		Version: 1,
		Sidecars: []SidecarsYamlEntry{
			{Name: "ktw-docs", BinaryPath: "/old/path", LastSeen: old},
		},
	}
	fresh := []SidecarsYamlEntry{
		{Name: "ktw-docs", BinaryPath: "/new/path", Version: "1.1"},
	}
	merged, newly := MergeInventory(existing, fresh)
	if len(merged.Sidecars) != 1 {
		t.Fatalf("merged size = %d, want 1", len(merged.Sidecars))
	}
	if len(newly) != 0 {
		t.Errorf("newly = %v, want empty (re-scanned existing entry)", newly)
	}
	if merged.Sidecars[0].LastSeen.Equal(old) {
		t.Errorf("LastSeen NOT bumped on re-scan")
	}
	if merged.Sidecars[0].BinaryPath != "/new/path" {
		t.Errorf("BinaryPath = %q, want /new/path (fresh wins on re-scan)", merged.Sidecars[0].BinaryPath)
	}
}

func TestPersistInventory_FirstWriteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "sidecars.yaml")
	fresh := []SidecarsYamlEntry{{Name: "ktw-docs", BinaryPath: existingBinary(t, dir, "ktw-docs")}}
	merged, newly, err := PersistInventory(path, fresh)
	if err != nil {
		t.Fatalf("PersistInventory: %v", err)
	}
	if len(merged.Sidecars) != 1 {
		t.Errorf("merged size = %d, want 1", len(merged.Sidecars))
	}
	if len(newly) != 1 {
		t.Errorf("newly count = %d, want 1", len(newly))
	}
	// Reload from disk to confirm persistence.
	reloaded, err := LoadSidecarsYaml(path)
	if err != nil {
		t.Fatalf("LoadSidecarsYaml: %v", err)
	}
	if len(reloaded.Sidecars) != 1 {
		t.Errorf("reloaded size = %d, want 1", len(reloaded.Sidecars))
	}
}

// A live PATH scan must NEVER demote a marketplace install to a bare scan row —
// version/sha256/marketplace provenance survives (matches available.List's
// read-side "recorded wins"). Guards the MergeInventory clobber fix.
func TestMergeInventory_PreservesMarketplaceRow(t *testing.T) {
	existing := &SidecarsYaml{
		Version: 1,
		Sidecars: []SidecarsYamlEntry{
			{Name: "ktw-example", BinaryPath: "/store/ktw-example/1.4.2/ktw-example", Version: "1.4.2", Source: "marketplace", Marketplace: "team", Sha256: "abc123"},
		},
	}
	// A PATH scan rediscovers ktw-example as a bare binary (no provenance).
	fresh := []SidecarsYamlEntry{
		{Name: "ktw-example", BinaryPath: "/usr/local/bin/ktw-example", Source: "PATH"},
	}
	merged, newly := MergeInventory(existing, fresh)
	if len(newly) != 0 {
		t.Fatalf("newlyDiscovered = %v, want none (ktw-example already recorded)", newly)
	}
	if len(merged.Sidecars) != 1 {
		t.Fatalf("merged has %d rows, want 1", len(merged.Sidecars))
	}
	got := merged.Sidecars[0]
	if got.Source != "marketplace" || got.Version != "1.4.2" || got.Sha256 != "abc123" || got.Marketplace != "team" {
		t.Fatalf("marketplace provenance was demoted by a scan: %+v", got)
	}
	if got.BinaryPath != "/store/ktw-example/1.4.2/ktw-example" {
		t.Errorf("BinaryPath overwritten by scan path: %q", got.BinaryPath)
	}
	if got.LastSeen.IsZero() {
		t.Errorf("LastSeen should be refreshed on the preserved row")
	}
}

// The control plane is recorded with source "infra-channel", not "marketplace",
// and the scan that demotes it is the one the GUI triggers hub-side — inside
// ktw-univers, whose own folder IS the control plane. Any managed source must
// survive, not just the one string the first fix happened to check.
func TestMergeInventory_PreservesInfraChannelRow(t *testing.T) {
	existing := &SidecarsYaml{
		Version: 1,
		Sidecars: []SidecarsYamlEntry{
			{Name: "ktw-univers", BinaryPath: "/root/bin/store/ktw-univers/v0.2.0/ktw-univers", Version: "v0.2.0", Source: "infra-channel", Marketplace: "/market/infra", Sha256: "def456"},
			{Name: "ktw-hub", BinaryPath: "/root/bin/store/ktw-hub/v0.2.0/ktw-hub", Version: "v0.2.0", Source: "infra-channel", Marketplace: "/market/infra", Sha256: "0123ab"},
		},
	}
	fresh := []SidecarsYamlEntry{
		{Name: "ktw-univers", BinaryPath: "/root/control-plane/ktw-univers", Source: SourceExecutableDir},
		{Name: "ktw-hub", BinaryPath: "/root/control-plane/ktw-hub", Source: SourceExecutableDir},
	}
	merged, newly := MergeInventory(existing, fresh)
	if len(newly) != 0 {
		t.Fatalf("newlyDiscovered = %v, want none (both already recorded)", newly)
	}
	for _, got := range merged.Sidecars {
		if got.Source != "infra-channel" || got.Version != "v0.2.0" || got.Sha256 == "" || got.Marketplace != "/market/infra" {
			t.Fatalf("infra-channel provenance was demoted by a scan: %+v", got)
		}
		if IsScanSource(got.Source) {
			t.Fatalf("%s reads as a scan row after merge", got.Name)
		}
		if got.LastSeen.IsZero() {
			t.Errorf("%s: LastSeen should be refreshed on the preserved row", got.Name)
		}
	}
}

// A genuinely discovered row (scan source) IS refreshed by a later scan — the
// managed-row protection must not freeze dev binaries at their first path.
func TestMergeInventory_RefreshesScanRow(t *testing.T) {
	existing := &SidecarsYaml{Version: 1, Sidecars: []SidecarsYamlEntry{
		{Name: "ktw-weather", BinaryPath: "/old/ktw-weather", Source: SourcePATH},
	}}
	fresh := []SidecarsYamlEntry{{Name: "ktw-weather", BinaryPath: "/new/ktw-weather", Source: SourceExecutableDir}}
	merged, _ := MergeInventory(existing, fresh)
	if got := merged.Sidecars[0]; got.BinaryPath != "/new/ktw-weather" || got.Source != SourceExecutableDir {
		t.Fatalf("scan row was not refreshed: %+v", got)
	}
}

// UpsertSidecarEntry + RemoveSidecarEntry are the single locked write path shared
// by the marketplace-install and inventory-scan writers.
func TestUpsertRemoveSidecarEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sidecars.yaml")
	if err := UpsertSidecarEntry(path, SidecarsYamlEntry{Name: "ktw-notify", Version: "2.0.0", Source: "marketplace"}); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	// Upsert same name replaces.
	if err := UpsertSidecarEntry(path, SidecarsYamlEntry{Name: "ktw-notify", Version: "2.1.0", Source: "marketplace"}); err != nil {
		t.Fatalf("upsert replace: %v", err)
	}
	sy, _ := LoadSidecarsYaml(path)
	if len(sy.Sidecars) != 1 || sy.Sidecars[0].Version != "2.1.0" {
		t.Fatalf("upsert did not replace by name: %+v", sy.Sidecars)
	}
	// Remove with a non-matching source is a no-op.
	if removed, _ := RemoveSidecarEntry(path, "ktw-notify", "PATH"); removed {
		t.Fatalf("removed a marketplace row under source=PATH")
	}
	// Remove with the matching source drops it.
	if removed, err := RemoveSidecarEntry(path, "ktw-notify", "marketplace"); err != nil || !removed {
		t.Fatalf("remove marketplace: removed=%v err=%v", removed, err)
	}
	sy, _ = LoadSidecarsYaml(path)
	if len(sy.Sidecars) != 0 {
		t.Fatalf("entry not removed: %+v", sy.Sidecars)
	}
}

// : a program of an installation discovers the installation's
// programs only, and a program of no installation records nothing in an
// installation's registry.
func TestAnInstallationDiscoversOnlyItsOwnPrograms(t *testing.T) {
	inst := filepath.Join(t.TempDir(), "katyoti")
	dev := filepath.Join(inst, "bin", "dev")
	checkout := filepath.Join(t.TempDir(), "checkout", "bin")
	roots := ScanRootsIn(inst, dev, filepath.Join(inst, "control-plane"), []string{checkout, filepath.Join(inst, "tools"), ""})
	var dirs []string
	for _, r := range roots {
		dirs = append(dirs, r.Dir)
	}
	want := []string{filepath.Join(inst, "control-plane"), dev, filepath.Join(inst, "tools")}
	if strings.Join(dirs, "|") != strings.Join(want, "|") {
		t.Fatalf("roots %v, want %v", dirs, want)
	}
	if got := ScanRootsIn(inst, dev, checkout, nil); len(got) != 1 || got[0].Dir != dev {
		t.Fatalf("from a checkout's bin/: %+v", got)
	}

	rows := []SidecarsYamlEntry{
		{Name: "ktw-a", BinaryPath: filepath.Join(checkout, "ktw-a.exe"), Source: SourceExecutableDir},
		{Name: "ktw-b", BinaryPath: filepath.Join(dev, "ktw-b.exe"), Source: SourceExecutableDir},
		{Name: "ktw-c", BinaryPath: filepath.Join(t.TempDir(), "ktw-c.exe"), Source: "marketplace"},
		{Name: "ktw-d", BinaryPath: filepath.Join(checkout, "ktw-d.exe")},
	}
	var kept []string
	for _, r := range KeepScanRowsWithin(rows, inst) {
		kept = append(kept, r.Name)
	}
	if strings.Join(kept, ",") != "ktw-b,ktw-c" {
		t.Fatalf("kept %v", kept)
	}

	// This test binary belongs to no installation: an installation's registry
	// is not written by its scan.
	if err := os.MkdirAll(inst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inst, "katyoti.yml"), []byte("name: dev\nroots: {config: .}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(inst, "sidecars.yaml")
	if _, _, err := PersistInventory(path, rows[:1]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a program of no installation wrote the installation's registry (%v)", err)
	}
}
