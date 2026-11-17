// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// membersOrch records the project verbs an install calls, in order, and
// refuses the attach of the folder named refuse.
type membersOrch struct {
	orchapi.OrchestratorService
	calls  []string
	refuse string
}

func (o *membersOrch) AttachProject(_ context.Context, in orchapi.AttachProjectInput) (orchapi.AttachProjectOutput, error) {
	o.calls = append(o.calls, "attach "+filepath.Base(in.ProjectDir)+" to "+in.Universe)
	if in.ProjectDir == o.refuse {
		return orchapi.AttachProjectOutput{}, errors.New("not created on this PC")
	}
	return orchapi.AttachProjectOutput{}, nil
}

func (o *membersOrch) RecordCreatedProject(_ context.Context, in orchapi.RecordCreatedProjectInput) (orchapi.RecordCreatedProjectOutput, error) {
	o.calls = append(o.calls, "record "+filepath.Base(in.ProjectDir))
	return orchapi.RecordCreatedProjectOutput{ProjectDir: in.ProjectDir, Recorded: true}, nil
}

func (o *membersOrch) ForgetCreatedProject(_ context.Context, in orchapi.ForgetCreatedProjectInput) (orchapi.ForgetCreatedProjectOutput, error) {
	o.calls = append(o.calls, "forget "+filepath.Base(in.ProjectDir))
	return orchapi.ForgetCreatedProjectOutput{ProjectDir: in.ProjectDir, Forgotten: true}, nil
}

// repositoriesCase is a case folder whose project and repository walk-b each
// hold a module sales.
func repositoriesCase(t *testing.T, repos ...api.CaseRepository) *Engine {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"project/modules/sales/README.md", "walk-b/modules/sales/README.md"} {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# sales\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c := &api.Case{Name: "two-repos", Dir: dir,
		Project:      api.CaseProject{From: "project", Name: "walk-a", Type: "software-project"},
		Repositories: repos}
	return NewEngine(c, t.TempDir(), "", io.Discard)
}

// A CASE DECLARING A SECOND REPOSITORY GETS A UNIVERSE SERVING BOTH
// : the install attaches the project, then lays the repository
// out under repositories/<name> with its content and a seed naming its key,
// records it as created on this PC BEFORE attaching it beside the project
// (refuses an unrecorded repository beside other members), and
// attaches it to the same universe. A case with its project alone keeps
// today's install: one attach, nothing recorded.
func TestACaseWithTwoRepositoriesAttachesBothAsMembers(t *testing.T) {
	e := repositoriesCase(t, api.CaseRepository{Name: "walk-b", From: "walk-b", Type: "software-project", Title: "Walk B"})
	o := &membersOrch{}
	if err := e.installProjects(context.Background(), o, "scn-two-repos"); err != nil {
		t.Fatal(err)
	}
	want := []string{"attach project to scn-two-repos", "record walk-b", "attach walk-b to scn-two-repos"}
	if !slices.Equal(o.calls, want) {
		t.Fatalf("calls = %q; want %q", o.calls, want)
	}
	b := e.RT.Repository("walk-b")
	if _, err := os.Stat(filepath.Join(b, "modules", "sales", "README.md")); err != nil {
		t.Errorf("the repository's content is not under repositories/walk-b: %v", err)
	}
	if k := cwmapi.ProjectKeyOf(b); k != "walk-b" {
		t.Errorf("the repository's key = %q; want walk-b", k)
	}
	if k := cwmapi.ProjectKeyOf(e.RT.Project()); k != "walk-a" {
		t.Errorf("the project's key = %q; want walk-a (its seed's name)", k)
	}

	one := repositoriesCase(t)
	o = &membersOrch{}
	if err := one.installProjects(context.Background(), o, "scn-two-repos"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"attach project to scn-two-repos"}; !slices.Equal(o.calls, want) {
		t.Fatalf("a case with one project: calls = %q; want %q", o.calls, want)
	}
	if _, err := os.Stat(filepath.Join(one.RT.Dir, "repositories")); err == nil {
		t.Error("a case with one project has a repositories folder")
	}
}

// A repository whose attach is refused is forgotten again: the creation
// record never names a folder no member serves.
func TestARefusedRepositoryIsForgotten(t *testing.T) {
	e := repositoriesCase(t, api.CaseRepository{Name: "walk-b", From: "walk-b"})
	o := &membersOrch{refuse: e.RT.Repository("walk-b")}
	err := e.installProjects(context.Background(), o, "scn-two-repos")
	if err == nil || !strings.Contains(err.Error(), "walk-b") {
		t.Fatalf("install = %v; want refused naming walk-b", err)
	}
	if want := []string{"attach project to scn-two-repos", "record walk-b", "attach walk-b to scn-two-repos", "forget walk-b"}; !slices.Equal(o.calls, want) {
		t.Fatalf("calls = %q; want %q", o.calls, want)
	}
}

// The case file refuses a repository name that is no project key, one named
// twice and one named as the project's key.
func TestACaseRefusesARepositoryNameThatIsNoKey(t *testing.T) {
	for body, want := range map[string]string{
		"repositories: [{name: Walk_B}]":                                     "lowercase letters",
		"repositories: [{name: walk-b}, {name: walk-b}]":                     "served twice",
		"project: {name: walk-a}\nrepositories: [{name: walk-a}]":            "served twice",
		"project: {name: walk-a}\nrepositories: [{name: walk-b, git: true}]": "",
	} {
		dir := t.TempDir()
		yaml := "name: two\nuniverse: {sidecars: [ktw-agents]}\n" + body + "\nstages: [{id: s1}]\n"
		if err := os.WriteFile(filepath.Join(dir, api.CaseFile), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
		c, err := api.Load(dir)
		switch {
		case want == "" && err != nil:
			t.Errorf("%s: %v; want loaded", body, err)
		case want == "" && (len(c.Repositories) != 1 || !c.Repositories[0].Git || c.Project.Name != "walk-a"):
			t.Errorf("%s: read %+v %+v", body, c.Project, c.Repositories)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
			t.Errorf("%s: %v; want refused saying %q", body, err, want)
		}
	}
}
