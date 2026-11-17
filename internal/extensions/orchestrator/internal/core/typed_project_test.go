// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/secrets"
)

// A TYPED PROJECT: create-project with a type makes the
// folder a git repository whose store holds the seed of its project object -
// the type, name, title and groups given - for CWM to mint at the first
// opening; its store is ignored by git. Without a type the folder
// is made as before: no repository, no seed.
func TestATypedProjectIsARepositoryHoldingItsSeed(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	isolateDataDir(t)
	registry, _ := catalog.LoadRegistry(filepath.Join(home, "universes.yaml"))
	m := &Service{registry: registry}

	out, err := m.CreateProject(ctx, api.CreateProjectInput{
		Name: "dossier-si-rh", Type: "dossier-architecture", Title: "Dossier SI RH",
		Capabilities: map[string]map[string]any{"dossier": {"project_code": "SP7001", "project_name": "SI RH"}},
	})
	if err != nil || !out.Created {
		t.Fatalf("create a typed project: %+v %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(out.ProjectDir, ".git")); err != nil {
		t.Errorf("the typed project is no git repository: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out.ProjectDir, cwmapi.ProjectFileName)); err == nil {
		t.Error("the hub wrote the project object: only CWM mints it, from the seed")
	}
	store := application.EnvOf(out.ProjectDir).HomeDir()
	raw, err := os.ReadFile(filepath.Join(store, cwmapi.ProjectSeedFile))
	if err != nil {
		t.Fatalf("no seed in the project's store: %v", err)
	}
	var seed cwmapi.ProjectSeed
	if err := config.DecodeBytes(raw, marshal.YAML, &seed); err != nil {
		t.Fatal(err)
	}
	if seed.Name != "dossier-si-rh" || seed.Type != "dossier-architecture" || seed.Title != "Dossier SI RH" || seed.Capabilities["dossier"]["project_code"] != "SP7001" {
		t.Errorf("the seed: %+v", seed)
	}
	ignore, err := os.ReadFile(filepath.Join(out.ProjectDir, ".gitignore"))
	if err != nil || !strings.Contains(string(ignore), filepath.Base(store)+"/") {
		t.Errorf("the store is not ignored by git: %q %v", ignore, err)
	}

	plain, err := m.CreateProject(ctx, api.CreateProjectInput{Name: "plain"})
	if err != nil || !plain.Created {
		t.Fatalf("create a plain project: %+v %v", plain, err)
	}
	for _, rel := range []string{".git", ".gitignore", filepath.Join(filepath.Base(store), cwmapi.ProjectSeedFile)} {
		if _, err := os.Stat(filepath.Join(plain.ProjectDir, rel)); err == nil {
			t.Errorf("a project with no type has %s: it is made as before", rel)
		}
	}
}

// A CREATED PROJECT IGNORES EVERY SECRET PATH: the
// .gitignore a typed project is created with holds the store and every
// pattern of secrets.GitignorePatterns - the one list, never copied - so a
// secret file of each pattern seeded in the project is ignored by git, while
// an allowed example (a negated pattern) and an ordinary file stay untracked.
func TestACreatedProjectIgnoresEverySecretPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	isolateDataDir(t)
	registry, _ := catalog.LoadRegistry(filepath.Join(home, "universes.yaml"))
	m := &Service{registry: registry}

	out, err := m.CreateProject(ctx, api.CreateProjectInput{Name: "seeded-secrets", Type: "dossier-architecture"})
	if err != nil || !out.Created {
		t.Fatalf("create a typed project: %+v %v", out, err)
	}
	// One file per pattern: "**/" lies in a sub-folder, "*" is any name, a
	// folder pattern holds a file.
	fileOf := func(pattern string) string {
		p := strings.ReplaceAll(pattern, "**/", "sub/")
		p = strings.ReplaceAll(p, "*", "x")
		if strings.HasSuffix(p, "/") {
			p += "s"
		}
		return p
	}
	var ignored, kept []string
	for _, pattern := range secrets.GitignorePatterns() {
		if strings.HasPrefix(pattern, "!") {
			kept = append(kept, fileOf(strings.TrimPrefix(pattern, "!")))
		} else {
			ignored = append(ignored, fileOf(pattern))
		}
	}
	kept = append(kept, "README.md")
	for _, rel := range append(append([]string{}, ignored...), kept...) {
		full := filepath.Join(out.ProjectDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain", "--ignored=matching", "--untracked-files=all")
	cmd.Dir = out.ProjectDir
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	status := map[string]string{} // path -> "!!" or "??"
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if len(line) > 3 {
			status[strings.Trim(line[3:], `"`)] = line[:2]
		}
	}
	// covered says how git lists rel: by itself, or through a listed folder.
	covered := func(rel string) string {
		for p, st := range status {
			if p == rel || (strings.HasSuffix(p, "/") && strings.HasPrefix(rel, p)) {
				return st
			}
		}
		return ""
	}
	for _, rel := range ignored {
		if st := covered(rel); st != "!!" {
			t.Errorf("the secret %s is not ignored by git (status %q)", rel, st)
		}
	}
	for _, rel := range kept {
		if st := covered(rel); st != "??" {
			t.Errorf("%s should stay untracked, not ignored (status %q)", rel, st)
		}
	}
	if t.Failed() {
		t.Logf("git status:\n%s", raw)
	}
}
