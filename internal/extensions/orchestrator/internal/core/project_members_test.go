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
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// memberRepo makes a repository folder holding a project object of the id
// given ("" for none), recorded as created on this PC.
func memberRepo(t *testing.T, root, name, id string) string {
	t.Helper()
	dir := clonedRepo(t, root, name, id)
	markCreated(t, dir)
	return dir
}

// markCreated records dirs as created on this PC, in the record beside the
// default registry.
func markCreated(t *testing.T, dirs ...string) {
	t.Helper()
	path, err := createdRecordPath("")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		if err := recordCreated(path, d); err != nil {
			t.Fatal(err)
		}
	}
}

// clonedRepo makes a repository folder as memberRepo does, but no creation
// verb of this PC made it: a clone.
func clonedRepo(t *testing.T, root, name, id string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if id != "" {
		if err := os.WriteFile(filepath.Join(dir, "project.cwm.yaml"), []byte("id: "+id+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func attachTo(s *Service, uni, dir string) error {
	_, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: uni, ProjectDir: dir})
	return err
}

// SEVERAL REPOSITORIES ARE MEMBERS. The universe serves its
// objects member by member (one CWM store each), so a second
// repository is attached beside the first: both listed, in attach order, through the one reader every consumer
// shares (cenv.AttachedProjects), each pointing at the universe.
func TestAttachProject_ASecondRepositoryIsAMember(t *testing.T) {
	root := projectTestRoot(t)
	uni := "shared"
	seedUniverseFor(t, root, uni)
	a := memberRepo(t, root, "repo-a", "alpha")
	b := memberRepo(t, root, "repo-b", "beta")
	s := &Service{}
	for _, d := range []string{a, b} {
		if err := attachTo(s, uni, d); err != nil {
			t.Fatalf("attach %s: %v", d, err)
		}
	}
	got := cenv.AttachedProjects(filepath.Dir(s.universeProjectsPath(uni)))
	if len(got) != 2 || !samePath(got[0], a) || !samePath(got[1], b) {
		t.Fatalf("members = %v, want [%s %s]", got, a, b)
	}
	for _, d := range []string{a, b} {
		if projectUniverse(d) != uni {
			t.Errorf("%s does not point at universe %q", d, uni)
		}
	}
	// Re-attaching a member changes nothing.
	if err := attachTo(s, uni, b); err != nil {
		t.Fatalf("re-attach: %v", err)
	}
	if out, _ := s.ListProjects(context.Background(), api.ListProjectsInput{Universe: uni}); len(out.Projects) != 2 {
		t.Errorf("after a re-attach: %v", out.Projects)
	}
}

// A MEMBER BESIDE OTHERS IS ADDRESSABLE. The store set opens one store per
// member, keyed by cwmapi.ProjectKeyOf: its project object's id; before its
// first opening, its seed's name (CWM mints the object from it before the
// store set joins it); else its folder's name. A repository with
// no project object joins under that key; one of a key another member holds -
// by its object, its seed or its folder - is refused before anything is
// written.
func TestAttachProject_AMemberBesideOthersNeedsAKeyOfItsOwn(t *testing.T) {
	root := projectTestRoot(t)
	uni := "shared"
	seedUniverseFor(t, root, uni)
	a := memberRepo(t, root, "repo-a", "alpha")
	s := &Service{}
	if err := attachTo(s, uni, a); err != nil {
		t.Fatal(err)
	}
	refused := []struct {
		name, id, seed string
	}{
		{"same-key", "alpha", ""},
		{"alpha", "", ""},
		{"seeded-alpha", "", "alpha"},
	}
	for _, c := range refused {
		dir := memberRepo(t, filepath.Join(root, "refused"), c.name, c.id)
		if c.seed != "" {
			seedRepo(t, dir, c.seed)
		}
		err := attachTo(s, uni, dir)
		if err == nil || !strings.Contains(err.Error(), `already attaches a project "alpha"`) {
			t.Errorf("%s: attach = %v; want refused as the key alpha a member holds", c.name, err)
		}
		if _, err := os.Stat(filepath.Join(application.EnvOf(dir).HomeDir(), "project.yaml")); err == nil {
			t.Errorf("%s: the refused repository was left pointing at the universe", c.name)
		}
	}
	// No project object yet: joins under its seed's name, else its folder's.
	seeded := memberRepo(t, root, "new-folder", "")
	seedRepo(t, seeded, "delta")
	for _, dir := range []string{memberRepo(t, root, "no-object", ""), seeded} {
		if err := attachTo(s, uni, dir); err != nil {
			t.Errorf("a member with no project object yet refused beside others: %v", err)
		}
	}
	if err := attachTo(s, uni, memberRepo(t, filepath.Join(root, "refused"), "repo-d", "delta")); err == nil || !strings.Contains(err.Error(), `"delta"`) {
		t.Errorf("attach of the key a seeded member holds = %v; want refused", err)
	}

	// The first member has no project object: a second joins beside it.
	uni2 := "other"
	seedUniverseFor(t, root, uni2)
	bare := memberRepo(t, root, "bare", "")
	if err := attachTo(s, uni2, bare); err != nil {
		t.Fatalf("a first member needs no project object: %v", err)
	}
	if err := attachTo(s, uni2, memberRepo(t, root, "repo-c", "gamma")); err != nil {
		t.Errorf("attach beside a member with no project object = %v; want accepted, keyed by its folder", err)
	}
}

// seedRepo leaves in dir's store the seed of a project object named name, as
// the hub does for a project it creates.
func seedRepo(t *testing.T, dir, name string) {
	t.Helper()
	store := application.EnvOf(dir).HomeDir()
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, cwmapi.ProjectSeedFile), []byte("name: "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TWO MEMBERS NEVER PUBLISH UNDER ONE TOKEN: the second holding a
// contributions folder under a token a member publishes under is refused.
func TestAttachProject_TwoMembersNeverShareAToken(t *testing.T) {
	root := projectTestRoot(t)
	uni := "shared"
	home := filepath.Join(root, "universes", uni)
	seedUniverseAt(t, root, uni, home)
	cfg := filepath.Join(application.EnvOf(home).HomeDir(), "config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "Univers.yaml"), []byte("version: 1\nname: shared\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := memberRepo(t, root, "repo-a", "alpha")
	b := memberRepo(t, root, "repo-b", "beta")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(filepath.Join(d, "modules", "sys-contributions", "skills"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(b, "project.cwm.yaml"), []byte("id: beta\ncapabilities:\n  contributions:\n    provider: alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Service{}
	if err := attachTo(s, uni, a); err != nil {
		t.Fatal(err)
	}
	err := attachTo(s, uni, b)
	if err == nil || !strings.Contains(err.Error(), `token "alpha"`) {
		t.Fatalf("attach = %v; want refused naming the shared token", err)
	}
	// Under a token of its own it joins.
	if err := os.WriteFile(filepath.Join(b, "project.cwm.yaml"), []byte("id: beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := attachTo(s, uni, b); err != nil {
		t.Fatalf("a member of its own token: %v", err)
	}
}
