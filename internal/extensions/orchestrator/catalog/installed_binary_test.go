// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

// An installation keeps each program in its own store folder: the registry's
// managed row names it, a scan row or a vanished file does not.
func TestInstalledBinaryIn(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "store", "ktw-sync", "1.0", "ktw-sync.exe")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	reg := filepath.Join(dir, "sidecars.yaml")
	if err := SaveSidecarsYaml(reg, &SidecarsYaml{Sidecars: []SidecarsYamlEntry{
		{Name: "ktw-docs", BinaryPath: bin, Source: SourcePATH},
		{Name: "ktw-sync", BinaryPath: bin, Source: "marketplace"},
	}}); err != nil {
		t.Fatal(err)
	}
	if got := installedBinaryIn(reg, "ktw-sync"); got != bin {
		t.Fatalf("ktw-sync: got %q, want %q", got, bin)
	}
	if got := installedBinaryIn(reg, "ktw-docs"); got != "" {
		t.Fatalf("a scan row is not an installed binary: got %q", got)
	}
	if got := installedBinaryIn(reg, "ktw-kb"); got != "" {
		t.Fatalf("no row: got %q", got)
	}
	if got := installedBinaryIn(filepath.Join(dir, "none.yaml"), "ktw-sync"); got != "" {
		t.Fatalf("no registry: got %q", got)
	}
}
