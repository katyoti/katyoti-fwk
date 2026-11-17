// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// Checks answers the guide's universe checks by reading the orchestrator it
// is given, or the one the context carries.
type Checks struct {
	orch api.OrchestratorService
}

// NewChecks builds the checks over the OrchestratorService of the context.
func NewChecks(ctx context.Context) (api.UniverseChecks, error) {
	orch, err := application.UseService[api.OrchestratorService](ctx)
	if err != nil {
		return nil, fmt.Errorf("universe checks: %w", err)
	}
	return ChecksOver(orch), nil
}

// ChecksOver builds the checks over a given OrchestratorService.
func ChecksOver(orch api.OrchestratorService) api.UniverseChecks { return &Checks{orch: orch} }

var _ api.UniverseChecks = (*Checks)(nil)

func (c *Checks) UniverseReady(ctx context.Context, in guideapi.CheckInput) (guideapi.CheckResult, error) {
	if in.Universe == "" {
		return guideapi.Failed("No universe is on screen yet.").WithMsg("checks.noUniverse", nil), nil
	}
	info, err := c.orch.GetUniverse(ctx, api.GetUniverseInput{Name: in.Universe})
	if err != nil {
		return guideapi.Failed("Universe %q is not registered.", in.Universe).WithMsg("checks.universeUnknown", map[string]any{"universe": in.Universe}), nil
	}
	if info.Summary.Protected {
		return guideapi.Failed("%s is the admin universe; create a universe of your own.", in.Universe).WithMsg("checks.adminUniverse", map[string]any{"universe": in.Universe}), nil
	}
	return guideapi.Passed("Universe %s is ready.", displayName(info)).WithMsg("checks.universeReady", map[string]any{"universe": displayName(info)}), nil
}

func (c *Checks) ProjectAttached(ctx context.Context, in guideapi.CheckInput) (guideapi.CheckResult, error) {
	if in.Universe == "" {
		return guideapi.Failed("No universe is on screen yet.").WithMsg("checks.noUniverse", nil), nil
	}
	out, err := c.orch.ListProjects(ctx, api.ListProjectsInput{Universe: in.Universe})
	if err != nil {
		return guideapi.Failed("The projects of %q cannot be read: %v", in.Universe, err).WithMsg("checks.projectsUnreadable", map[string]any{"universe": in.Universe, "error": err.Error()}), nil
	}
	switch n := len(out.Projects); n {
	case 0:
		return guideapi.Failed("No project is attached to %s yet.", in.Universe).WithMsg("checks.noProject", map[string]any{"universe": in.Universe}), nil
	case 1:
		return guideapi.Passed("Project %s is attached.", out.Projects[0]).WithMsg("checks.projectAttached", map[string]any{"project": out.Projects[0]}), nil
	default:
		return guideapi.Passed("%d projects are attached.", n).WithMsg("checks.projectsAttached", map[string]any{"count": n}), nil
	}
}

func (c *Checks) CapabilityWired(ctx context.Context, in guideapi.CheckInput) (guideapi.CheckResult, error) {
	if in.Universe == "" {
		return guideapi.Failed("No universe is on screen yet.").WithMsg("checks.noUniverse", nil), nil
	}
	info, err := c.orch.GetUniverse(ctx, api.GetUniverseInput{Name: in.Universe})
	if err != nil {
		return guideapi.Failed("Universe %q is not registered.", in.Universe).WithMsg("checks.universeUnknown", map[string]any{"universe": in.Universe}), nil
	}
	want := in.With["sidecar"]
	var others []string
	for _, sc := range info.Sidecars {
		if want != "" && sc.Name == want {
			return guideapi.Passed("%s is wired into %s.", want, in.Universe).WithMsg("checks.wired", map[string]any{"sidecar": want, "universe": in.Universe}), nil
		}
		if sc.Name != "ktw-agents" {
			others = append(others, sc.Name)
		}
	}
	if want != "" {
		return guideapi.Failed("%s is not wired into %s yet.", want, in.Universe).WithMsg("checks.notWired", map[string]any{"sidecar": want, "universe": in.Universe}), nil
	}
	if len(others) == 0 {
		return guideapi.Failed("No capability beyond ktw-agents is wired into %s yet.", in.Universe).WithMsg("checks.noCapability", map[string]any{"universe": in.Universe}), nil
	}
	return guideapi.Passed("Wired: %s.", strings.Join(others, ", ")).WithMsg("checks.wiredList", map[string]any{"sidecars": strings.Join(others, ", ")}), nil
}

func (c *Checks) RepositoriesHaveRemote(ctx context.Context, in guideapi.CheckInput) (guideapi.CheckResult, error) {
	if in.Universe == "" {
		return guideapi.Failed("No universe is on screen yet.").WithMsg("checks.noUniverse", nil), nil
	}
	info, err := c.orch.GetUniverse(ctx, api.GetUniverseInput{Name: in.Universe})
	if err != nil {
		return guideapi.Failed("Universe %q is not registered.", in.Universe).WithMsg("checks.universeUnknown", map[string]any{"universe": in.Universe}), nil
	}
	dirs := []string{info.Summary.HomePath}
	if out, err := c.orch.ListProjects(ctx, api.ListProjectsInput{Universe: in.Universe}); err == nil {
		dirs = append(dirs, out.Projects...)
	}
	seen := map[string]bool{}
	var repos, bare []string
	for _, d := range dirs {
		top := repositoryOf(ctx, d)
		if top == "" || seen[top] {
			continue
		}
		seen[top] = true
		repos = append(repos, top)
		if !hasRemote(ctx, top) {
			bare = append(bare, top)
		}
	}
	if len(bare) > 0 {
		list := strings.Join(bare, ", ")
		return guideapi.Failed("No remote for %s: its work is not backed up off this machine.", list).WithMsg("checks.noRemote", map[string]any{"repositories": list}), nil
	}
	if len(repos) == 0 {
		return guideapi.Passed("No repository holds %s.", in.Universe).WithMsg("checks.noRepository", map[string]any{"universe": in.Universe}), nil
	}
	return guideapi.Passed("Every repository of %s has a remote.", in.Universe).WithMsg("checks.remotes", map[string]any{"universe": in.Universe}), nil
}

// repositoryOf is the top folder of the git repository holding dir; empty
// when dir is empty, missing or in no repository.
func repositoryOf(ctx context.Context, dir string) string {
	if dir == "" {
		return ""
	}
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return filepath.Clean(strings.TrimSpace(string(out)))
}

// hasRemote tells whether the repository at top names at least one remote.
func hasRemote(ctx context.Context, top string) bool {
	out, err := exec.CommandContext(ctx, "git", "-C", top, "remote").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func displayName(info api.UniverseInfo) string {
	if info.Summary.DisplayName != "" {
		return info.Summary.DisplayName
	}
	return info.Summary.Name
}
