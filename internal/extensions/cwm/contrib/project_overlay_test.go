// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// edge writes one edge type, `rel`, whose description names the layer.
func edge(t *testing.T, dir, layer string) {
	t.Helper()
	d := filepath.Join(dir, "edge-types")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "name: rel\nfrom: any\nto: any\ndescription: " + layer + "\n"
	if err := os.WriteFile(filepath.Join(d, "rel.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// project writes a project whose contributions live under packs/acme and
// overrides ktw-foo's `rel`.
func project(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	obj := "id: acme\ntype: project\ncapabilities:\n  contributions:\n    provider: acme\n    root: packs/acme\n"
	if err := os.WriteFile(filepath.Join(root, "project.cwm.yaml"), []byte(obj), 0o644); err != nil {
		t.Fatal(err)
	}
	edge(t, filepath.Join(root, "packs", "acme", "overwrites", "ktw-foo", "embed", "cwm"), "project")
	return root
}

func relIn(t *testing.T, home string) string {
	t.Helper()
	embedded := fstest.MapFS{"edge-types/rel.yaml": {Data: []byte("name: rel\nfrom: any\nto: any\ndescription: embedded\n")}}
	raw, err := lastWins(append([]fs.FS{embedded}, FloorOverlays(home, "ktw-foo")...), "edge-types/rel.yaml")
	if err != nil || raw == nil {
		t.Fatalf("rel is not declared: %v", err)
	}
	var e struct {
		Description string `yaml:"description"`
	}
	if err := marshal.Unmarshal(raw, marshal.YAML, &e); err != nil {
		t.Fatal(err)
	}
	return e.Description
}

// TestAProjectTreeNoLongerOverridesTheFloor: a project's per-file override
// tree of a provider's own CWM types is not layered - its overrides are
// override entries the catalog applies, the tree retires through the data
// migration. The universe's tree, gated by allow_overwrites, stays the
// operator's last word.
func TestAProjectTreeNoLongerOverridesTheFloor(t *testing.T) {
	// A project tree that once overrode rel is there, and is not read.
	project(t)

	// No universe overlay (the gate off, though the folder exists): the embedded type.
	off := cwmHome(t, false, false)
	edge(t, filepath.Join(cenv.OverwritesDir(off, "ktw-foo"), "embed", "cwm"), "universe")
	if got := relIn(t, off); got != "embedded" {
		t.Fatalf("without allow_overwrites the embedded type stays: got %q", got)
	}

	// The universe overlay on: it wins.
	on := cwmHome(t, true, false)
	edge(t, filepath.Join(cenv.OverwritesDir(on, "ktw-foo"), "embed", "cwm"), "universe")
	if got := relIn(t, on); got != "universe" {
		t.Fatalf("the universe's override must be the last word: got %q", got)
	}
}

// lastWins reads a file as the layered pack does: the last source holding it.
func lastWins(sources []fs.FS, name string) ([]byte, error) {
	for i := len(sources) - 1; i >= 0; i-- {
		if raw, err := fs.ReadFile(sources[i], name); err == nil {
			return raw, nil
		}
	}
	return nil, fs.ErrNotExist
}
