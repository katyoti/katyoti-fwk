// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"
	"testing"

	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// fakeOrch is a minimal in-memory OrchestratorService for SolutionApply
// unit tests (the real one is exercised by #10's suite + the live smoke).
type fakeOrch struct {
	universes map[string][]orchapi.SidecarSpec
	archived  []string
}

func newFakeOrch() *fakeOrch { return &fakeOrch{universes: map[string][]orchapi.SidecarSpec{}} }

func (f *fakeOrch) ListUniverses(ctx context.Context, in orchapi.ListUniversesInput) (orchapi.ListUniversesOutput, error) {
	return orchapi.ListUniversesOutput{}, nil
}
func (f *fakeOrch) GetUniverse(ctx context.Context, in orchapi.GetUniverseInput) (orchapi.UniverseInfo, error) {
	if scs, ok := f.universes[in.Name]; ok {
		return orchapi.UniverseInfo{Sidecars: scs}, nil
	}
	return orchapi.UniverseInfo{}, fmt.Errorf("universe %q not found", in.Name)
}
func (f *fakeOrch) CreateUniverse(ctx context.Context, in orchapi.CreateUniverseInput) (orchapi.UniverseInfo, error) {
	if _, ok := f.universes[in.Name]; ok {
		return orchapi.UniverseInfo{}, fmt.Errorf("universe %q already exists", in.Name)
	}
	if in.DisplayName == "" {
		return orchapi.UniverseInfo{}, fmt.Errorf("display_name is required")
	}
	f.universes[in.Name] = nil
	return orchapi.UniverseInfo{}, nil
}
func (f *fakeOrch) DeleteUniverse(ctx context.Context, in orchapi.DeleteUniverseInput) (orchapi.DeleteUniverseOutput, error) {
	delete(f.universes, in.Name)
	f.archived = append(f.archived, in.Name)
	return orchapi.DeleteUniverseOutput{Archived: true}, nil
}
func (f *fakeOrch) CreateProject(ctx context.Context, in orchapi.CreateProjectInput) (orchapi.CreateProjectOutput, error) {
	return orchapi.CreateProjectOutput{}, nil
}

func (f *fakeOrch) ScanInventory(ctx context.Context, in orchapi.ScanInventoryInput) (orchapi.ScanInventoryOutput, error) {
	return orchapi.ScanInventoryOutput{}, nil
}
func (f *fakeOrch) AddSidecar(ctx context.Context, in orchapi.AddSidecarInput) (orchapi.UniverseInfo, error) {
	scs, ok := f.universes[in.Universe]
	if !ok {
		return orchapi.UniverseInfo{}, fmt.Errorf("universe %q not in registry", in.Universe)
	}
	scs = append(scs, orchapi.SidecarSpec{Name: in.Sidecar.Name, Binary: in.Sidecar.BinaryPath})
	f.universes[in.Universe] = scs
	return orchapi.UniverseInfo{Sidecars: scs}, nil
}
func (f *fakeOrch) RemoveSidecar(ctx context.Context, in orchapi.RemoveSidecarInput) (orchapi.UniverseInfo, error) {
	return orchapi.UniverseInfo{}, nil
}

// The methods below exist only to satisfy OrchestratorService — SolutionApply
// does not exercise them. They grew with the interface (universe edit + project
// attach/detach); keeping them as explicit no-ops makes a future interface
// addition a compile error here rather than a silent gap.
func (f *fakeOrch) UpdateUniverse(ctx context.Context, in orchapi.UpdateUniverseInput) (orchapi.UniverseInfo, error) {
	return orchapi.UniverseInfo{}, nil
}
func (f *fakeOrch) AddExistingUniverse(ctx context.Context, in orchapi.AddExistingUniverseInput) (orchapi.UniverseInfo, error) {
	return orchapi.UniverseInfo{}, nil
}
func (f *fakeOrch) SetUniverseSolution(ctx context.Context, in orchapi.SetUniverseSolutionInput) (orchapi.UniverseInfo, error) {
	return orchapi.UniverseInfo{}, nil
}
func (f *fakeOrch) RecordCreatedProject(ctx context.Context, in orchapi.RecordCreatedProjectInput) (orchapi.RecordCreatedProjectOutput, error) {
	return orchapi.RecordCreatedProjectOutput{}, nil
}
func (f *fakeOrch) ForgetCreatedProject(ctx context.Context, in orchapi.ForgetCreatedProjectInput) (orchapi.ForgetCreatedProjectOutput, error) {
	return orchapi.ForgetCreatedProjectOutput{}, nil
}
func (f *fakeOrch) AttachProject(ctx context.Context, in orchapi.AttachProjectInput) (orchapi.AttachProjectOutput, error) {
	return orchapi.AttachProjectOutput{}, nil
}
func (f *fakeOrch) ResolveProject(ctx context.Context, in orchapi.ResolveProjectInput) (orchapi.ResolveProjectOutput, error) {
	return orchapi.ResolveProjectOutput{}, nil
}
func (f *fakeOrch) DetachProject(ctx context.Context, in orchapi.DetachProjectInput) (orchapi.DetachProjectOutput, error) {
	return orchapi.DetachProjectOutput{}, nil
}
func (f *fakeOrch) SwitchProject(ctx context.Context, in orchapi.SwitchProjectInput) (orchapi.SwitchProjectOutput, error) {
	return orchapi.SwitchProjectOutput{}, nil
}
func (f *fakeOrch) ListProjects(ctx context.Context, in orchapi.ListProjectsInput) (orchapi.ListProjectsOutput, error) {
	return orchapi.ListProjectsOutput{}, nil
}

// Compile-time proof the stub stays complete.
var _ orchapi.OrchestratorService = (*fakeOrch)(nil)

var _ orchapi.OrchestratorService = (*fakeOrch)(nil)

func TestSolutionApply_HappyPath_WithNewSidecar(t *testing.T) {
	s, _ := fixtureService(t)
	s.orchOverride = newFakeOrch()

	out, err := s.ApplyUniverse(context.Background(), api.ApplyUniverseInput{Spec: api.SolutionSpec{
		Name:     "sol-test",
		Sidecars: []api.SolutionSidecar{{Name: "ktw-target"}},
		NewSidecar: &api.SolutionNewSidecar{
			Name:    "ktw-sol-new",
			Include: "skills,tasks",
		},
		SkipBuild: true, // fake repo root has no real module to build
	}})
	if err != nil {
		t.Fatalf("SolutionApply: %v\nsteps: %v", err, out.Steps)
	}
	if out.Scaffolded == nil {
		t.Fatal("expected scaffold output")
	}
	if len(out.Universe.Sidecars) != 2 {
		t.Fatalf("expected 2 members, got %+v", out.Universe.Sidecars)
	}
	joined := strings.Join(out.Steps, "; ")
	for _, want := range []string{"scaffolded app/ktw-sol-new", "created universe", "added sidecar \"ktw-target\"", "added sidecar \"ktw-sol-new\""} {
		if !strings.Contains(joined, want) {
			t.Errorf("steps missing %q: %s", want, joined)
		}
	}
}

func TestSolutionApply_IdempotencyAndReplace(t *testing.T) {
	s, _ := fixtureService(t)
	fake := newFakeOrch()
	s.orchOverride = fake
	fake.universes["sol-dup"] = nil

	_, err := s.ApplyUniverse(context.Background(), api.ApplyUniverseInput{Spec: api.SolutionSpec{
		Name: "sol-dup", Sidecars: []api.SolutionSidecar{{Name: "ktw-target"}},
	}})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already-exists error, got %v", err)
	}

	out, err := s.ApplyUniverse(context.Background(), api.ApplyUniverseInput{Spec: api.SolutionSpec{
		Name: "sol-dup", Sidecars: []api.SolutionSidecar{{Name: "ktw-target"}}, Replace: true,
	}})
	if err != nil {
		t.Fatalf("replace path: %v", err)
	}
	if len(fake.archived) != 1 || fake.archived[0] != "sol-dup" {
		t.Fatalf("expected archive of sol-dup, got %v", fake.archived)
	}
	if !strings.Contains(strings.Join(out.Steps, ";"), "archived existing universe") {
		t.Fatalf("steps missing archive record: %v", out.Steps)
	}
}

func TestSolutionApply_EmptySpecRejected(t *testing.T) {
	s, _ := fixtureService(t)
	s.orchOverride = newFakeOrch()
	if _, err := s.ApplyUniverse(context.Background(), api.ApplyUniverseInput{Spec: api.SolutionSpec{Name: "x"}}); err == nil ||
		!strings.Contains(err.Error(), "nothing to compose") {
		t.Fatalf("expected nothing-to-compose error, got %v", err)
	}
}
