// SPDX-License-Identifier: Apache-2.0

// Package publish is the scenario cases' entry for a marketplace build: a case
// published as a solution — installed when it is not yet, its binaries taken
// from the build just published (a solution pins its members by content, so
// they must be those very bytes), then its snapshot.
package publish

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/sandbox"
)

// SolutionInput is a case to publish as a solution.
type SolutionInput struct {
	// Case is the case's folder; Root where the cases run (default
	// sandbox.DefaultRoot); Bin the build's binaries for this machine.
	Case, Root, Bin string
	// Name of the solution (default: the case's); Checkpoint to publish from
	// (default: the case as it stands).
	Name, Checkpoint string
	// Version, Catalog (the catalog file) and SecretKey (signs it).
	Version, Catalog, SecretKey string
	// ConfigSets and Vaults it pins (name@version); Docs its documents folder
	// (default: the case's docs/); WhatsNew a change log made into « Quoi de neuf ».
	ConfigSets, Vaults []string
	Docs, WhatsNew     string
	// LetterCovers are the change logs a written « Quoi de neuf » covers
	// (empty: WhatsNew alone) -.
	LetterCovers []string
	// LetterRequests is how many requests the changes of those change logs
	// name, checked against the letter's figure; 0 when unknown.
	LetterRequests int
	// OtherConfigSets are the config-sets (name@version) each of the case's
	// other universes applies, by its name.
	OtherConfigSets map[string][]string
	Out             io.Writer
}

// Check says whether the case can be published: its folder is a case.
func Check(caseDir string) error {
	_, err := api.Load(caseDir)
	return err
}

// Solution publishes the case as a solution.
func Solution(ctx context.Context, in SolutionInput) (authoring.AddSolutionResult, error) {
	c, err := api.Load(in.Case)
	if err != nil {
		return authoring.AddSolutionResult{}, err
	}
	c.Dir, _ = filepath.Abs(in.Case)
	root := in.Root
	if root == "" {
		root = sandbox.DefaultRoot()
	}
	out := in.Out
	if out == nil {
		out = io.Discard
	}
	e := core.NewEngine(c, root, in.Bin, out)
	e.InstalledBinary = controlplane.TargetPath
	e.CasesDir = filepath.Dir(c.Dir)
	_, statErr := os.Stat(e.RT.Universe())
	installed := statErr == nil
	if !installed {
		fmt.Fprintf(out, "case %s is not installed in %s: installing it\n", c.Name, root)
	}
	switch {
	case !sandbox.Inside(e.RT.Dir):
		// The build runs outside the case's sandbox: installing the case and
		// refreshing its binaries (which give the sandbox its control plane and
		// its admin universe) are the scenario commands', run inside it as
		// `ktw-compose scenario` enters it itself. The snapshot reads the case's
		// folders and needs no sandbox.
		verb := "refresh-bin"
		if !installed {
			verb = "install"
		}
		if err := inSandbox(ctx, e.RT.Dir, out, verb, c.Name, filepath.Dir(c.Dir), root, in.Bin); err != nil {
			return authoring.AddSolutionResult{}, fmt.Errorf("%s the case %s: %w", verb, c.Name, err)
		}
	case !installed:
		if err := e.Install(ctx, false); err != nil {
			return authoring.AddSolutionResult{}, fmt.Errorf("install the case %s: %w", c.Name, err)
		}
	default:
		if err := e.RefreshBinaries(ctx); err != nil {
			return authoring.AddSolutionResult{}, fmt.Errorf("refresh the case %s's binaries: %w", c.Name, err)
		}
	}
	return e.Snapshot(snapshotOptions(in))
}

// snapshotOptions are the snapshot's options from what the build asks: the
// checkpoint, version, catalog and name, the config sets, vaults, docs, the
// « Quoi de neuf » and every change log it covers.
func snapshotOptions(in SolutionInput) core.SnapshotOptions {
	return core.SnapshotOptions{
		Checkpoint: in.Checkpoint, Version: in.Version, Marketplace: in.Catalog, Name: in.Name,
		ConfigSets: in.ConfigSets, Vaults: in.Vaults, Docs: in.Docs, WhatsNew: in.WhatsNew, SecretKey: in.SecretKey,
		LetterCovers: in.LetterCovers, LetterRequests: in.LetterRequests, OtherConfigSets: in.OtherConfigSets,
	}
}

// inSandbox runs `scenario <verb> <case>` of this very binary inside the case's
// sandbox (the environment sandbox.Enter gives a scenario command), its output
// to out.
func inSandbox(ctx context.Context, caseDir string, out io.Writer, verb, name, cases, root, bin string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"scenario", verb, name, "--cases", cases, "--root", root}
	if verb == "install" {
		// Installed only when the case has no universe: its folder may already
		// hold what the engine wrote (its logs), which install refuses without
		// --force.
		args = append(args, "--force")
	}
	if bin != "" {
		args = append(args, "--bin", bin)
	}
	cmd := exec.CommandContext(ctx, self, args...)
	cmd.Env = sandbox.Of(caseDir).Env(os.Environ(), application.Machine())
	cmd.Stdout, cmd.Stderr = out, out
	return runScenario(cmd)
}

// runScenario runs a scenario command inSandbox prepared (a test sees it).
var runScenario = func(cmd *exec.Cmd) error { return cmd.Run() }
