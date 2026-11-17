// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"os"
	"path/filepath"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

func writeIn(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A served folder's declaration is read once, for the host and the hub's
// folder member alike: its identity and its `dirs:`, the rest of the file
// (its `config:` block) left to its reader; a folder without one declares
// nothing and is no error.
func TestAFolderDeclarationIsReadOnce(t *testing.T) {
	root := t.TempDir()
	if d, found, err := ReadFolderDecl(loader.NewFSLoader(root)); err != nil || found || d.Name != "" {
		t.Fatalf("a folder with no declaration: %+v %v %v", d, found, err)
	}
	writeIn(t, root, FolderManifestFile, "name: acme\nversion: 1.2.0\ndirs:\n  references: docs/refs\n  cwm: \"\"\nconfig:\n  apiKeys:\n    - envName: ACME_KEY\n")
	d, found, err := ReadFolderDecl(loader.NewFSLoader(root))
	if err != nil || !found || d.Name != "acme" || d.Version != "1.2.0" || d.Dirs["references"] != "docs/refs" {
		t.Fatalf("decl = %+v %v %v", d, found, err)
	}
}

// A host folder (cwm, scripts) is read from its conventional folder, from
// the folder `dirs:` names, from content the pack records under it when the
// folder itself is gone (bulk), and never when `dirs:` switches it off.
func TestAHostFolderIsReadFromDiskOrFromWhatThePackRecords(t *testing.T) {
	root := t.TempDir()
	writeIn(t, root, "scripts/run.sh", "echo hi\n")
	var d FolderDecl
	if got := d.HostDir(root, "scripts", contribapi.LayerManifest{}); got != "scripts" {
		t.Errorf("conventional scripts/ = %q", got)
	}
	if got := d.HostDir(root, "cwm", contribapi.LayerManifest{}); got != "" {
		t.Errorf("an absent cwm/ = %q, want none", got)
	}
	recorded := contribapi.LayerManifest{Layers: map[string]contribapi.LayerRef{"cwm/object-types/big.yaml": {}}}
	if got := d.HostDir(root, "cwm", recorded); got != "cwm" {
		t.Errorf("cwm/ recorded but not on disk = %q, want cwm", got)
	}
	d.Dirs = map[string]string{"scripts": ""}
	if got := d.HostDir(root, "scripts", contribapi.LayerManifest{}); got != "" {
		t.Errorf("scripts switched off = %q", got)
	}
	d.Dirs = map[string]string{"scripts": "tools/scripts"}
	writeIn(t, root, "tools/scripts/a.sh", "echo a\n")
	if got := d.HostDir(root, "scripts", contribapi.LayerManifest{}); got != "tools/scripts" {
		t.Errorf("scripts remapped = %q", got)
	}
}
