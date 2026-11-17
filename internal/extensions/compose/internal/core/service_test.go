// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureService builds a Service over the REAL canonical templates and a
// fake repo root (go.mod + app/ + one target sidecar with embed.go).
func fixtureService(t *testing.T) (*Service, string) {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	tmplDir := filepath.Join(repoRoot, "app", "ktw-compose", "embed", "templates")
	if _, err := os.Stat(tmplDir); err != nil {
		t.Fatalf("canonical templates not found: %v", err)
	}

	fakeRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeRoot, "go.mod"), []byte("module fake\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(fakeRoot, "app", "ktw-target")
	if err := os.MkdirAll(filepath.Join(target, "embed", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	embedGo := "package main\n\nimport \"embed\"\n\n//go:embed embed/skills embed/tasks\nvar embeddedFS embed.FS\n"
	if err := os.WriteFile(filepath.Join(target, "embed.go"), []byte(embedGo), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(os.DirFS(tmplDir))
	s.repoRoot = fakeRoot
	return s, fakeRoot
}

func TestSkillNew_EmitsAndRefusesOverwrite(t *testing.T) {
	s, root := fixtureService(t)
	out, err := s.CreateSkill(context.Background(), apiSkill("ktw-target", "my-skill"))
	if err != nil {
		t.Fatalf("SkillNew: %v", err)
	}
	raw, err := os.ReadFile(out.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "name: my-skill") || !strings.Contains(string(raw), "advertised: false") {
		t.Fatalf("rendered skill wrong:\n%s", raw)
	}
	if !strings.HasPrefix(out.Path, filepath.Join(root, "app", "ktw-target")) {
		t.Fatalf("wrong target: %s", out.Path)
	}
	if _, err := s.CreateSkill(context.Background(), apiSkill("ktw-target", "my-skill")); err == nil ||
		!strings.Contains(err.Error(), "never overwrites") {
		t.Fatalf("expected overwrite refusal, got %v", err)
	}
}

func TestTaskNew_FlatShape_AndEmbedWarning(t *testing.T) {
	s, _ := fixtureService(t)
	out, err := s.CreateTask(context.Background(), apiTask("ktw-target", "my-task"))
	if err != nil {
		t.Fatalf("TaskNew: %v", err)
	}
	if filepath.Base(out.Path) != "my-task.md" {
		t.Fatalf("task must be a FLAT file, got %s", out.Path)
	}
	raw, _ := os.ReadFile(out.Path)
	if !strings.Contains(string(raw), "type: agent") || !strings.Contains(string(raw), "config:") {
		t.Fatalf("task shape wrong:\n%s", raw)
	}
	// embed.go includes embed/tasks → no warning expected.
	if strings.Contains(out.Note, "WARNING") {
		t.Fatalf("unexpected embed warning: %s", out.Note)
	}

	// Personas dir is NOT in the fixture embed.go → warning expected.
	pOut, err := s.CreatePersona(context.Background(), apiPersona("ktw-target", "my-persona"))
	if err != nil {
		t.Fatalf("PersonaNew: %v", err)
	}
	if !strings.Contains(pOut.Note, "WARNING") || !strings.Contains(pOut.Note, "embed/personas") {
		t.Fatalf("expected go:embed warning for personas, got: %s", pOut.Note)
	}
}

func TestRefNew_PathSafety(t *testing.T) {
	s, _ := fixtureService(t)
	if _, err := s.CreateReference(context.Background(), apiRef("ktw-target", "../escape.md")); err == nil {
		t.Fatal("expected path-traversal rejection")
	}
	out, err := s.CreateReference(context.Background(), apiRef("ktw-target", "topic/notes.md"))
	if err != nil {
		t.Fatalf("RefNew: %v", err)
	}
	raw, _ := os.ReadFile(out.Path)
	if !strings.Contains(string(raw), "# notes") {
		t.Fatalf("title not derived: %s", raw)
	}
}

func TestSidecarNew_ScaffoldsAndRefusesDuplicates(t *testing.T) {
	s, root := fixtureService(t)
	out, err := s.CreateSidecar(context.Background(), apiSidecar("ktw-fresh", "skills,references"))
	if err != nil {
		t.Fatalf("SidecarNew: %v", err)
	}
	for _, rel := range []string{"main.go", "embed.go", "cmd/root.go", "cmd/bootstrap.go",
		"embed/skills/usage.md", "embed/references/README.md"} {
		if _, err := os.Stat(filepath.Join(root, "app", "ktw-fresh", filepath.FromSlash(rel))); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
	// No README in embed/skills — it would fail the strict skill corpus;
	// usage.md is the dir's embeddable file.
	if _, err := os.Stat(filepath.Join(root, "app", "ktw-fresh", "embed", "skills", "README.md")); err == nil {
		t.Error("embed/skills/README.md must NOT be scaffolded")
	}
	raw, _ := os.ReadFile(filepath.Join(root, "app", "ktw-fresh", "embed.go"))
	if !strings.Contains(string(raw), "embed/references embed/skills") {
		t.Fatalf("includes wrong: %s", raw)
	}
	if !strings.Contains(out.Note, "APPS") {
		t.Fatalf("makefile note missing: %s", out.Note)
	}
	if _, err := s.CreateSidecar(context.Background(), apiSidecar("ktw-fresh", "")); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate refusal, got %v", err)
	}
	if _, err := s.CreateSidecar(context.Background(), apiSidecar("ktw-bad", "nope")); err == nil ||
		!strings.Contains(err.Error(), "unknown include") {
		t.Fatalf("expected unknown-include error, got %v", err)
	}
}

func TestInRepoOnly(t *testing.T) {
	s, _ := fixtureService(t)
	s.repoRoot = t.TempDir() // no go.mod, no app/
	if _, err := s.CreateSidecar(context.Background(), apiSidecar("ktw-x", "")); err == nil ||
		!strings.Contains(err.Error(), "in-repo only") {
		t.Fatalf("expected in-repo-only error, got %v", err)
	}
}
