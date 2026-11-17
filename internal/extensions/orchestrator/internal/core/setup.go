// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

//go:embed embed/sample
var sampleFS embed.FS

// sampleMarker is the file that says a folder holds the sample project, so a
// second run reuses it instead of refusing a folder that is not empty.
const sampleMarker = "README.md"

// Setup does the guide's actions on a universe through the orchestrator.
type Setup struct {
	orch api.OrchestratorService
	home func() (string, error)
}

// NewSetup builds the setup actions over the OrchestratorService of the context.
func NewSetup(ctx context.Context) (api.UniverseSetup, error) {
	orch, err := application.UseService[api.OrchestratorService](ctx)
	if err != nil {
		return nil, fmt.Errorf("universe setup: %w", err)
	}
	return SetupOver(orch, os.UserHomeDir), nil
}

// SetupOver builds the setup actions over a given orchestrator and home.
func SetupOver(orch api.OrchestratorService, home func() (string, error)) api.UniverseSetup {
	return &Setup{orch: orch, home: home}
}

var _ api.UniverseSetup = (*Setup)(nil)

func (s *Setup) EnsureAgents(ctx context.Context, in guideapi.CheckInput) (guideapi.CheckResult, error) {
	if in.Universe == "" {
		return guideapi.CheckResult{}, fmt.Errorf("no universe is on screen")
	}
	info, err := s.orch.GetUniverse(ctx, api.GetUniverseInput{Name: in.Universe})
	if err != nil {
		return guideapi.CheckResult{}, err
	}
	for _, sc := range info.Sidecars {
		if sc.Name == "ktw-agents" {
			return guideapi.Passed("ktw-agents is already in %s.", in.Universe).WithMsg("checks.agentsAlready", map[string]any{"universe": in.Universe}), nil
		}
	}
	if _, err := s.orch.AddSidecar(ctx, api.AddSidecarInput{Universe: in.Universe, Sidecar: api.CreateSidecarSelection{Name: "ktw-agents"}}); err != nil {
		return guideapi.CheckResult{}, err
	}
	return guideapi.Passed("ktw-agents was added to %s.", in.Universe).WithMsg("checks.agentsAdded", map[string]any{"universe": in.Universe}), nil
}

func (s *Setup) CreateSampleProject(ctx context.Context, in guideapi.CheckInput) (guideapi.CheckResult, error) {
	if in.Universe == "" {
		return guideapi.CheckResult{}, fmt.Errorf("no universe is on screen")
	}
	dir := in.With["folder"]
	if dir == "" {
		dir = s.sampleDir()
		if dir == "" {
			return guideapi.CheckResult{}, fmt.Errorf("no folder given and no home folder")
		}
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	// A folder that held nothing is the sample's own: what is in it is
	// written here. A sample written before is reused as it is.
	_, statErr := os.Stat(dir)
	existed := statErr == nil
	entries, _ := os.ReadDir(dir)
	written := len(entries) == 0
	if err := writeSample(dir); err != nil {
		return guideapi.CheckResult{}, err
	}
	// The sample written here by the person's setup is created on this PC
	// : recorded through the orchestrator that attaches it
	// BEFORE the attach - memberBeside refuses beside other members a
	// repository the record does not name, whatever it holds. A failure to
	// record is not the setup's: only an attach beside members is refused.
	if written {
		_, _ = s.orch.RecordCreatedProject(ctx, api.RecordCreatedProjectInput{ProjectDir: dir})
	}
	if _, err := s.orch.AttachProject(ctx, api.AttachProjectInput{ProjectDir: dir, Universe: in.Universe}); err != nil {
		// A refused attach leaves neither the record nor the sample it wrote,
		// as project create does: the record never names a place a later
		// clone could fill, and a second run writes and records it again.
		if written {
			_, _ = s.orch.ForgetCreatedProject(ctx, api.ForgetCreatedProjectInput{ProjectDir: dir})
			removeWrittenSample(dir, existed)
		}
		return guideapi.CheckResult{}, err
	}
	return guideapi.Passed("The sample project is in %s and attached to %s.", dir, in.Universe).WithMsg("checks.sampleAttached", map[string]any{"folder": dir, "universe": in.Universe}), nil
}

// sampleDir is where the sample project goes when no folder is given: the
// installation's projects folder; a program of no installation (a development
// build, a test) puts it in its home.
func (s *Setup) sampleDir() string {
	if inst := cenv.CurrentInstallation(); inst != nil {
		return inst.ProjectDir("katyoti-sample")
	}
	home, err := s.home()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "katyoti-sample")
}

// removeWrittenSample removes the sample the setup wrote into dir: the folder
// itself when the setup made it, else only what it wrote into the empty
// folder it was given.
func removeWrittenSample(dir string, existed bool) {
	if !existed {
		_ = os.RemoveAll(dir)
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
}

// writeSample copies the embedded sample into dir. A folder that already holds
// other files is refused (nothing is overwritten), unless it is a sample
// written before, which is reused as it is.
func writeSample(dir string) error {
	entries, err := os.ReadDir(dir)
	switch {
	case err == nil && len(entries) > 0:
		if _, err := os.Stat(filepath.Join(dir, sampleMarker)); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "modules", "welcome")); err == nil {
				return nil // a sample written before: reuse it
			}
		}
		return fmt.Errorf("%s is not empty; choose an empty folder for the sample project", dir)
	case err != nil && !os.IsNotExist(err):
		return err
	}
	root, err := fs.Sub(sampleFS, "embed/sample")
	if err != nil {
		return err
	}
	return fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}
