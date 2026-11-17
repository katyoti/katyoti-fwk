// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// THE OBJECT CORPUS AND THE WORKSPACE STATE HAVE DIFFERENT ROOTS.
//
// Objects are authored and COMMITTED beside the code they describe (G2:
// "version-bound content lives in the committed git repository"). Node
// storage, the graph and the type store are recreatable state under the
// config root, which is gitignored. Joining `modules/` onto the config root
// meant the committed corpus could never be found — and could never be MOVED
// somewhere it would be found, because everything under the config root is
// ignored by git.
func TestObjectsAreReadFromTheProjectRoot(t *testing.T) {
	project := t.TempDir()
	// Deliberately NOT named like a config root: this test is about the split
	// being honoured when it is stated, not about deriving it.
	cfgRoot := filepath.Join(project, "state")
	if err := os.MkdirAll(cfgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteWorkspaceRaw(cfgRoot, []byte(`version: "1"`)); err != nil {
		t.Fatalf("workspace.yaml: %v", err)
	}
	// An authored object, where a committed corpus actually lives.
	dir := filepath.Join(project, "modules", "demo", "objects")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	obj := "id: demo.x\ntype: goal\ncapabilities:\n  content:\n    title: An authored goal\n"
	if err := os.WriteFile(filepath.Join(dir, "x.cwm.yaml"), []byte(obj), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, err := New(stdctx.Background(), &CWMServiceConfig{
		WorkspaceRoot: cfgRoot, ProjectRoot: project,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	out, err := svc.ObjectList(stdctx.Background(), api.ObjectListInput{Module: "demo"})
	if err != nil {
		t.Fatalf("ObjectList: %v", err)
	}
	if out.Total != 1 || len(out.Objects) != 1 || out.Objects[0].ID != "demo.x" {
		t.Fatalf("ObjectList found %d object(s) %v — the authored corpus under "+
			"<project>/modules/ must be visible to the service", out.Total, out.Objects)
	}
}

// WITHOUT ProjectRoot, THE OLD BEHAVIOUR IS PRESERVED EXACTLY.
//
// Every substrate test constructs this config with a bare root and no
// ProjectRoot. The fallback is what keeps those working, so it is asserted
// rather than assumed — a silent change here would move every test's object
// store without anything failing.
func TestObjectsRootFallsBackToTheWorkspaceRoot(t *testing.T) {
	c := &CWMServiceConfig{WorkspaceRoot: "/w"}
	if got := c.ObjectsRoot(); got != "/w" {
		t.Errorf("ObjectsRoot() = %q, want the workspace root when ProjectRoot is unset", got)
	}
	c.ProjectRoot = "/p"
	if got := c.ObjectsRoot(); got != "/p" {
		t.Errorf("ObjectsRoot() = %q, want ProjectRoot when set", got)
	}
}
