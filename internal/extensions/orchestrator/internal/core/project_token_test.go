// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// THE TOKEN RULE AT ATTACH. A project holding a contributions folder is
// published under its token, which no member of the universe may answer to:
// the attach refuses it and writes nothing. A project with no such folder
// contributes nothing and attaches whatever its token.
func TestAttachProject_RefusesATokenAMemberAnswersTo(t *testing.T) {
	root := projectTestRoot(t)
	uni := "shared"
	home := filepath.Join(root, "universes", uni)
	seedUniverseAt(t, root, uni, home)
	cfg := filepath.Join(application.EnvOf(home).HomeDir(), "config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "Univers.yaml"), []byte("version: 1\nname: shared\nsidecars:\n  - name: acme-shop\n    binary: ktw-acme.exe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "acme")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "project.cwm.yaml"), []byte("id: Acme Shop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Service{}
	attach := func() error {
		_, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: uni, ProjectDir: project})
		return err
	}

	// The token acme-shop is a member's name, but the project contributes
	// nothing: attached.
	if err := attach(); err != nil {
		t.Fatalf("a project with no contributions folder: %v", err)
	}
	if _, err := s.DetachProject(context.Background(), api.DetachProjectInput{ProjectDir: project}); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(project, "modules", "sys-contributions", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := attach()
	if err == nil || !strings.Contains(err.Error(), `"acme-shop"`) || !strings.Contains(err.Error(), "member acme-shop") {
		t.Fatalf("attach = %v; want refused, naming the token and the member", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".katyoti", "project.yaml")); err == nil {
		t.Error("the refused project was left pointing at the universe")
	}
}

// Attaching or detaching rewrites the universe's projects.yaml; the
// operator's `trusted: true` on a project survives the rewrite.
func TestARewriteOfTheProjectsKeepsTheirTrust(t *testing.T) {
	root := projectTestRoot(t)
	uni := "shared"
	home := filepath.Join(root, "universes", uni)
	seedUniverseAt(t, root, uni, home)
	s := &Service{}
	path := s.universeProjectsPath(uni)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	trusted := filepath.Join(root, "trusted-one")
	if err := os.WriteFile(path, []byte("projects:\n  - path: "+trusted+"\n    trusted: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.registerProject(uni, filepath.Join(root, "other")); err != nil {
		t.Fatal(err)
	}
	if !cenv.ProjectTrusted(filepath.Dir(path), trusted) {
		raw, _ := os.ReadFile(path)
		t.Fatalf("the rewrite dropped the trust:\n%s", raw)
	}
}
