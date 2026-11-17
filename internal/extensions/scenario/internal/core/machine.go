// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/admin"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/sandbox"
)

// THE SANDBOX'S MACHINE — what a katyoti installation keeps at machine level,
// kept for the case under <case>/machine: the universe registry the case's
// universe is registered in, the admin universe (its data in the sandbox, set
// in its own Settings.yaml), and the control plane — the case's own hub, GUI
// and ktw-univers — so a GUI opened in the sandbox spawns the case's binaries
// and nothing of the installation it was run from.

// controlPlaneBinaries are copied from the case's bin folder into the
// sandbox's control plane, when the bin folder has them.
var controlPlaneBinaries = []string{hubBinary, guiBinaryName, universBinary}

// universBinary serves the admin universe's management slots.
const universBinary = "ktw-univers"

// Sandbox is the case's sandbox.
func (e *Engine) Sandbox() sandbox.Sandbox { return sandbox.Of(e.RT.Dir) }

// RequireSandbox refuses to work on the case outside its sandbox: every
// registry, cache and hub the process would reach is then the installation's.
func (e *Engine) RequireSandbox() error {
	if !sandbox.Inside(e.RT.Dir) {
		return fmt.Errorf("not inside the sandbox of %s: run it through `ktw-compose scenario`, which enters the case's sandbox itself", e.RT.Dir)
	}
	// Marked inside, yet on another root: a program of an installation that
	// took its installation's roots back would work on that installation.
	if root, sb := application.Machine().Root, e.Sandbox().Root(); !cenv.Within(root, sb) {
		return fmt.Errorf("inside the sandbox of %s but on the root %s, not the case's %s: nothing of the case is done there", e.RT.Dir, root, sb)
	}
	return nil
}

// ensureMachine gives the sandbox its control plane (from the case's bin
// folder) and its admin universe, with the admin's data in the sandbox.
func (e *Engine) ensureMachine() error {
	sb := e.Sandbox()
	for _, b := range controlPlaneBinaries {
		src := filepath.Join(e.RT.Bin(), exe(b))
		if !fileExists(src) {
			continue
		}
		if err := copyIfChanged(src, filepath.Join(sb.ControlPlane(), exe(b))); err != nil {
			return fmt.Errorf("control plane %s: %w", b, err)
		}
	}
	// The admin universe is created where the sandbox's global root says
	// (KTW_KATYOTI_ROOT): its manifest names the control plane's ktw-univers,
	// copied above.
	// Only inside the sandbox: a process that took another root (an
	// installation's, from its executable's katyoti.yml) would create and
	// configure that installation's admin universe instead.
	if home := admin.Home(); !cenv.Within(home, sb.Root()) {
		return fmt.Errorf("the sandbox's admin universe would be %s, outside the case's root %s: this process did not enter the sandbox", home, sb.Root())
	}
	if err := admin.EnsureDefault(); err != nil {
		return fmt.Errorf("the sandbox's admin universe: %w", err)
	}
	if err := os.MkdirAll(sb.AdminData(), 0o755); err != nil {
		return err
	}
	return setDataDir(application.EnvOf(admin.Home()).ConfigDir(), sb.AdminData())
}

// Adopt makes an installed case whole in its sandbox: the machine, then its
// universe registered there and its project attached, when they are not — a
// case installed before it had a sandbox is adopted as it is (its stages,
// checkpoints and data kept), and an adopted one is left alone.
func (e *Engine) Adopt(ctx context.Context) error {
	st, err := loadState(e.RT)
	if err != nil || !st.Installed {
		return err
	}
	if err := e.ensureMachine(); err != nil {
		return err
	}
	orch, err := application.UseService[orchapi.OrchestratorService](ctx)
	if err != nil {
		return fmt.Errorf("the orchestrator service: %w", err)
	}
	if _, err := orch.GetUniverse(ctx, orchapi.GetUniverseInput{Name: e.Case.UniverseName()}); err == nil {
		return nil
	}
	if _, err := orch.AddExistingUniverse(ctx, orchapi.AddExistingUniverseInput{Path: e.RT.Universe()}); err != nil {
		return fmt.Errorf("register %s in its sandbox: %w", e.Case.UniverseName(), err)
	}
	if _, err := orch.AttachProject(ctx, orchapi.AttachProjectInput{ProjectDir: e.RT.Project(), Universe: e.Case.UniverseName()}); err != nil {
		return fmt.Errorf("attach the project in its sandbox: %w", err)
	}
	e.logf("universe %s adopted into its sandbox (%s)", e.Case.UniverseName(), e.Sandbox().Machine())
	return nil
}

// copyIfChanged copies a file unless the copy already has its size and
// modification time (which the copy keeps): every verb ensures the control
// plane, and copying a hundred megabytes of binaries each time — each copy
// scanned again by an antivirus — would cost seconds per verb.
func copyIfChanged(src, dst string) error {
	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	if di, err := os.Stat(dst); err == nil && di.Size() == si.Size() && di.ModTime().Equal(si.ModTime()) {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Chtimes(dst, si.ModTime(), si.ModTime())
}
