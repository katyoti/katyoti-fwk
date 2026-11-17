// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/store"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// A solution built by the publisher from two universes — the one snapshotted
// its main one, the other named beside it with its own seed — installs as two
// universes: the main one under the name given, the other beside it with its
// members and its seed laid out (over).
func TestASolutionPublishedFromTwoUniversesInstallsBoth(t *testing.T) {
	s := solutionFixture(t, "")
	// The seed's project goes to the person's projects folder: the test's own.
	person := t.TempDir()
	t.Setenv("USERPROFILE", person)
	t.Setenv("HOME", person)
	mkts, err := s.reg.List()
	if err != nil || len(mkts) != 1 {
		t.Fatalf("marketplaces %+v, %v", mkts, err)
	}
	catalogPath := mkts[0].URL

	// The publisher's two universes, each wiring a content member from the
	// versioned store (pinned at its store version), and the review's project.
	src := t.TempDir()
	universe := func(name, member string) string {
		folder, err := store.ContentFolderPath(member, "1.0")
		if err != nil {
			t.Fatal(err)
		}
		home := filepath.Join(src, name)
		if err := orchcatalog.SaveManifest(home, &orchcatalog.Manifest{Name: name, DisplayName: strings.ToUpper(name[:1]) + name[1:],
			Sidecars: []orchapi.SidecarSpec{{Name: member, Transport: "stdio", Binary: "ktw-host", Args: []string{"--folder", folder}, ContentRoot: folder}}}); err != nil {
			t.Fatal(err)
		}
		return home
	}
	studio, review := universe("studio", "notes"), universe("review", "cards")
	project := filepath.Join(src, "review-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Relecture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := authoring.New().AddSolution(studio, "2", "duo", catalogPath, "", authoring.SolutionSeed{}, authoring.SolutionSpec{
		Others: []authoring.OtherUniverse{{Name: "review", Home: review, Seed: authoring.SolutionSeed{Project: project}}},
	}); err != nil {
		t.Fatal(err)
	}

	// The universe service the seed's project is attached through.
	orch := &attachSpy{attached: map[string]string{}}
	app := application.New(application.Config())
	application.PutService[orchapi.OrchestratorService](app, "default", orch)
	ctx := application.WithApplication(context.Background(), app)

	out, err := s.InstallSolution(ctx, mktapi.InstallSolutionInput{Ref: mktapi.ItemRef{Name: "duo", Version: "2"}, UniverseHome: "team"})
	if err != nil {
		t.Fatal(err)
	}
	reg := registered(t)
	team, teamReview := reg["team"], reg["team-review"]
	if team == "" || teamReview == "" {
		t.Fatalf("registry %v: want team and team-review", reg)
	}
	if out.Main != "team" || len(out.Universes) != 2 || !out.Universes[0].Main || out.Universes[1].Name != "team-review" {
		t.Errorf("main %q, universes %+v; want team (main) then team-review", out.Main, out.Universes)
	}
	if got := members(t, team); strings.Join(got, ",") != "notes" {
		t.Errorf("team wires %v, want notes", got)
	}
	if got := members(t, teamReview); strings.Join(got, ",") != "cards" {
		t.Errorf("team-review wires %v, want cards", got)
	}
	p := out.Universes[1].Project
	if p == "" {
		t.Fatalf("team-review got no project: %+v", out.Universes[1])
	}
	if _, err := os.Stat(filepath.Join(p, "README.md")); err != nil {
		t.Errorf("team-review's seed is not laid out in %s: %v", p, err)
	}
	if out.Project != "" {
		t.Errorf("the main universe got the project %s; the seed is review's", out.Project)
	}
	if orch.attached["team-review"] != p || len(orch.attached) != 1 {
		t.Errorf("attached %v; want team-review's project alone", orch.attached)
	}
	// The seed laid it out on this PC: recorded as created here, through the
	// orchestrator, before it was attached.
	if len(orch.created) != 1 || orch.created[0] != p {
		t.Errorf("recorded as created on this PC %v; want team-review's project %s", orch.created, p)
	}
}

// attachSpy is the universe service as a seed's install uses it: every
// universe known, each project attached recorded (universe → project), each
// folder recorded as created on this PC listed.
type attachSpy struct {
	orchapi.OrchestratorService
	attached map[string]string
	created  []string
}

func (a *attachSpy) RecordCreatedProject(_ context.Context, in orchapi.RecordCreatedProjectInput) (orchapi.RecordCreatedProjectOutput, error) {
	a.created = append(a.created, in.ProjectDir)
	return orchapi.RecordCreatedProjectOutput{ProjectDir: in.ProjectDir, Recorded: true}, nil
}

func (a *attachSpy) GetUniverse(_ context.Context, in orchapi.GetUniverseInput) (orchapi.UniverseInfo, error) {
	return orchapi.UniverseInfo{}, nil
}

func (a *attachSpy) AttachProject(_ context.Context, in orchapi.AttachProjectInput) (orchapi.AttachProjectOutput, error) {
	a.attached[in.Universe] = in.ProjectDir
	return orchapi.AttachProjectOutput{}, nil
}
