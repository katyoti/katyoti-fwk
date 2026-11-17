// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// setupOrch records what the setup actions change.
type setupOrch struct {
	fakeOrch
	added    []string
	attached []string
	recorded []string
}

func (o *setupOrch) AddSidecar(_ context.Context, in api.AddSidecarInput) (api.UniverseInfo, error) {
	o.added = append(o.added, in.Sidecar.Name)
	return api.UniverseInfo{}, nil
}

func (o *setupOrch) RecordCreatedProject(_ context.Context, in api.RecordCreatedProjectInput) (api.RecordCreatedProjectOutput, error) {
	o.recorded = append(o.recorded, in.ProjectDir)
	return api.RecordCreatedProjectOutput{ProjectDir: in.ProjectDir, Recorded: true}, nil
}

func (o *setupOrch) ForgetCreatedProject(_ context.Context, in api.ForgetCreatedProjectInput) (api.ForgetCreatedProjectOutput, error) {
	return api.ForgetCreatedProjectOutput{ProjectDir: in.ProjectDir}, nil
}

func (o *setupOrch) AttachProject(_ context.Context, in api.AttachProjectInput) (api.AttachProjectOutput, error) {
	o.attached = append(o.attached, in.ProjectDir)
	return api.AttachProjectOutput{ProjectDir: in.ProjectDir, Universe: in.Universe}, nil
}

func TestUniverseSetup_EnsureAgentsAddsOnlyWhenMissing(t *testing.T) {
	o := &setupOrch{fakeOrch: fakeOrch{universes: map[string]api.UniverseInfo{
		"bare":  {Summary: api.UniverseSummary{Name: "bare"}},
		"ready": {Summary: api.UniverseSummary{Name: "ready"}, Sidecars: []api.SidecarSpec{{Name: "ktw-agents"}}},
	}}}
	s := SetupOver(o, func() (string, error) { return t.TempDir(), nil })
	ctx := context.Background()
	if _, err := s.EnsureAgents(ctx, guideapi.CheckInput{Universe: "ready"}); err != nil {
		t.Fatal(err)
	}
	if len(o.added) != 0 {
		t.Fatalf("added %v to a universe that has ktw-agents", o.added)
	}
	if r, err := s.EnsureAgents(ctx, guideapi.CheckInput{Universe: "bare"}); err != nil || !r.OK {
		t.Fatalf("bare: %+v %v", r, err)
	}
	if len(o.added) != 1 || o.added[0] != "ktw-agents" {
		t.Fatalf("added %v, want ktw-agents once", o.added)
	}
	if _, err := s.EnsureAgents(ctx, guideapi.CheckInput{}); err == nil {
		t.Error("no universe on screen: want an error")
	}
}

func TestUniverseSetup_SampleProjectWrittenAttachedReusedNeverOverwriting(t *testing.T) {
	o := &setupOrch{}
	home := t.TempDir()
	s := SetupOver(o, func() (string, error) { return home, nil })
	ctx := context.Background()

	r, err := s.CreateSampleProject(ctx, guideapi.CheckInput{Universe: "mine"})
	if err != nil || !r.OK {
		t.Fatalf("default folder: %+v %v", r, err)
	}
	dir := filepath.Join(home, "katyoti-sample")
	for _, f := range []string{"README.md", "modules/welcome/concepts.md", "modules/welcome/objects/welcome.cwm.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err != nil {
			t.Errorf("sample file %s: %v", f, err)
		}
	}
	// Its welcome module is of a type ktw-methods ships: the module type is
	// retired.
	if w, err := os.ReadFile(filepath.Join(dir, "modules", "welcome", "objects", "welcome.cwm.yaml")); err != nil || !strings.Contains(string(w), "\ntype: documentation\n") {
		t.Errorf("the sample's welcome module is not typed documentation: %v\n%s", err, w)
	}
	if len(o.attached) != 1 || o.attached[0] != dir {
		t.Fatalf("attached %v, want %s", o.attached, dir)
	}
	if len(o.recorded) != 1 || o.recorded[0] != dir {
		t.Fatalf("recorded %v, want %s once, through its orchestrator", o.recorded, dir)
	}
	// A second run reuses the sample it wrote.
	if r, err := s.CreateSampleProject(ctx, guideapi.CheckInput{Universe: "mine"}); err != nil || !r.OK {
		t.Fatalf("reuse: %+v %v", r, err)
	}
	if len(o.recorded) != 1 {
		t.Errorf("a reused sample was recorded again: %v", o.recorded)
	}
	// A folder holding something else is refused and left untouched.
	busy := filepath.Join(home, "busy")
	if err := os.MkdirAll(busy, 0o755); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(busy, "notes.txt")
	if err := os.WriteFile(mine, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSampleProject(ctx, guideapi.CheckInput{Universe: "mine", With: map[string]string{"folder": busy}}); err == nil {
		t.Fatal("a folder with other files was accepted")
	}
	if b, _ := os.ReadFile(mine); string(b) != "mine" {
		t.Error("the refused folder was changed")
	}
	if _, err := os.Stat(filepath.Join(busy, "README.md")); err == nil {
		t.Error("the sample was written into a refused folder")
	}
}
