// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// : an installed item whose files are gone is forgotten - never
// listed (so never config-checked) and, once pruned, no longer in
// sidecars.yaml - while an item whose files are there, one found on PATH by
// its bare name, and one on a drive that is not there are kept.
func TestAnInstalledItemWhoseFilesAreGoneIsForgotten(t *testing.T) {
	dir := t.TempDir()
	place := func(p string) string {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	kept := place(filepath.Join(dir, "store", "ktw-methods", "v2", "ktw-methods.exe"))
	content := filepath.Join(dir, "store-content", "ktw-guide", "v1")
	if err := os.MkdirAll(content, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sidecars.yaml")
	rows := []SidecarsYamlEntry{
		{Name: "ktw-methods", BinaryPath: kept, Version: "v2", Source: "marketplace"},
		{Name: "ktw-sync", BinaryPath: filepath.Join(dir, "store", "ktw-sync", "v0.2.0-test.5", "ktw-sync.exe"), Version: "v0.2.0-test.5", Source: "marketplace"},
		{Name: "ktw-guide", Kind: "content", FolderPath: content, Version: "v1", Source: "marketplace"},
		{Name: "ktw-notes", Kind: "content", FolderPath: filepath.Join(dir, "store-content", "ktw-notes", "v0"), Version: "v0", Source: "marketplace"},
		{Name: "ktw-host", BinaryPath: "ktw-host", Source: "PATH"},
	}
	if filepath.Separator == '\\' {
		for _, d := range "QRSTUVWXYZ" {
			if _, err := os.Stat(string(d) + `:\`); err != nil {
				rows = append(rows, SidecarsYamlEntry{Name: "ktw-offline", BinaryPath: string(d) + `:\katyoti\ktw-offline.exe`, Source: "marketplace"})
				break
			}
		}
	}
	if err := SaveSidecarsYaml(path, &SidecarsYaml{Version: 1, Sidecars: rows}); err != nil {
		t.Fatal(err)
	}
	want := "ktw-methods,ktw-guide,ktw-host"
	if len(rows) == 6 {
		want += ",ktw-offline"
	}

	s, err := LoadSidecarsYaml(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := entryNames(s.Sidecars); got != want {
		t.Fatalf("listed %s, want %s", got, want)
	}
	gone, err := PruneVanishedSidecars(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := entryNames(gone); got != "ktw-sync,ktw-notes" {
		t.Fatalf("pruned %s, want ktw-sync,ktw-notes", got)
	}
	raw, err := readSidecarsYaml(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := entryNames(raw.Sidecars); got != want {
		t.Fatalf("sidecars.yaml holds %s after the prune, want %s", got, want)
	}
}

func entryNames(rows []SidecarsYamlEntry) string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return strings.Join(out, ",")
}
