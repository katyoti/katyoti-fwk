// SPDX-License-Identifier: Apache-2.0

package sidecarkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// projectWith lays out a project (id acme: contributions in
// modules/sys-contributions) carrying an overwrites tree for ktw-compose's
// greet skill and review task.
func projectWith(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "project.cwm.yaml"), "id: acme\n")
	over := filepath.Join(root, "modules", "sys-contributions", "overwrites", "ktw-compose", "embed")
	writeFile(t, filepath.Join(over, "skills", "greet.md"), "---\nname: greet\n---\nPROJECT body")
	writeFile(t, filepath.Join(over, "tasks", "review", "TASK.md"), "---\nname: review\n---\nPROJECT task")
	return root
}

// attachTrusted writes the universe's projects.yaml attaching root, trusted.
func attachTrusted(t *testing.T, configRoot, root string) {
	t.Helper()
	writeFile(t, filepath.Join(configRoot, "projects.yaml"), "projects:\n  - path: "+root+"\n    trusted: true\n")
}

func baseWithTask() loader.Loader {
	return loader.NewMapLoader(map[string]string{
		"embed/skills/greet.md":      "---\nname: greet\n---\nEMBED body",
		"embed/tasks/review/TASK.md": "---\nname: review\n---\nEMBED task",
	})
}

func read(t *testing.T, l loader.Loader, p string) string {
	t.Helper()
	b, err := l.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func originOf(l loader.Loader, p string) string {
	if o, ok := l.(loader.OriginSource); ok {
		return o.Origin(p)
	}
	return ""
}

// A project's overwrites tree is no longer served, even from a project the
// universe attaches and trusts: a project overrides a provider's content
// through override objects the catalog applies. The universe's tree, behind
// allow_overwrites, still is.
func TestAProjectOverwritesTreeIsNoLongerServedTheUniverseTreeStillIs(t *testing.T) {
	root := projectWith(t)

	home := writeHome(t, false, "", "")
	attachTrusted(t, home, root)
	l := buildOverlay(baseWithTask(), home, "ktw-compose")
	for _, p := range []string{"embed/skills/greet.md", "embed/tasks/review/TASK.md"} {
		if got := read(t, l, p); strings.Contains(got, "PROJECT") {
			t.Fatalf("%s: %q, a project overwrites tree was served", p, got)
		}
		if o := originOf(l, p); o != "" {
			t.Errorf("%s: origin = %q, want the embed", p, o)
		}
	}

	home = writeHome(t, true, "embed/skills/greet.md", "---\nname: greet\n---\nUNIVERSE body")
	attachTrusted(t, home, root)
	l = buildOverlay(baseWithTask(), home, "ktw-compose")
	if got := read(t, l, "embed/skills/greet.md"); !strings.Contains(got, "UNIVERSE body") {
		t.Fatalf("gate on: %q, want the universe's override", got)
	}
	if o := originOf(l, "embed/skills/greet.md"); o != contribapi.OriginUniverse {
		t.Errorf("origin = %q, want universe", o)
	}
	if got := read(t, l, "embed/tasks/review/TASK.md"); !strings.Contains(got, "EMBED task") {
		t.Fatalf("gate on: %q, want the embedded task (no project layer)", got)
	}
}
