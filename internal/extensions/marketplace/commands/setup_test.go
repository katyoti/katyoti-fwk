// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// fakeInfra is a control plane already in place whose channel names solution.
type fakeInfra struct {
	mktapi.InfraService
	solution string
}

func (f fakeInfra) SetupMachine(context.Context) (mktapi.SetupMachineOutput, error) {
	return mktapi.SetupMachineOutput{Channel: "fixture", Marketplace: "m", Solution: f.solution}, nil
}

// fakeEngine installs a solution of two universes, the main one where it is
// named, and records what it was asked. It can install nothing else — no
// program: setup only ever installs the solution.
type fakeEngine struct{ calls []mktapi.InstallSolutionInput }

func (f *fakeEngine) InstallSolution(_ context.Context, in mktapi.InstallSolutionInput) (mktapi.InstallSolutionOutput, error) {
	f.calls = append(f.calls, in)
	return mktapi.InstallSolutionOutput{Main: in.UniverseHome, Project: in.ProjectDir,
		Universes: []mktapi.InstalledUniverse{{Name: in.UniverseHome, Main: true}, {Name: in.UniverseHome + "-review"}}}, nil
}

// runSetup runs `setup` against a channel naming solution, in a home of its
// own (its Documents), outside any installation; it answers the engine, the
// universe made the default, the one the GUI opened on, and the output.
func runSetup(t *testing.T, solution string, args ...string) (eng *fakeEngine, def, opened, out, home string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("XDG_DOCUMENTS_DIR", "")
	t.Setenv("KTW_INSTALLATION", "")
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(home, "root"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	eng = &fakeEngine{}
	prevInfra, prevEngine := setupInfra, setupEngine
	setupInfra = func(*cobra.Command) (mktapi.InfraService, error) { return fakeInfra{solution: solution}, nil }
	setupEngine = func(*cobra.Command) (solutionInstaller, error) { return eng, nil }
	t.Cleanup(func() { setupInfra, setupEngine = prevInfra, prevEngine })

	cmd := NewSetupCommand(func(_ *cobra.Command, u string) error { opened = u; return nil },
		func(u string) { def = u }, nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(append([]string{"--no-shortcut"}, args...))
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("setup: %v\n%s", err, buf.String())
	}
	return eng, def, opened, buf.String(), home
}

// The channel names a solution: setup installs it under its own name — its
// main universe named after it, never « demo » — its project in the person's
// Documents, makes the main universe the default and opens the GUI on it. It
// asks the engine for nothing but the solution: no program is installed.
func TestSetupOpensOnTheChannelsSolution(t *testing.T) {
	eng, def, opened, out, home := runSetup(t, "archi@1.2")
	if len(eng.calls) != 1 {
		t.Fatalf("setup should install the solution once, got %d calls\n%s", len(eng.calls), out)
	}
	in := eng.calls[0]
	if in.Ref.Name != "archi" || in.Ref.Version != "1.2" || in.UniverseHome != "archi" || !in.SolutionName {
		t.Errorf("install %+v; want archi@1.2 into the universe archi, under its own name", in)
	}
	if want := filepath.Join(home, "Documents", "Katyoti", "archi"); in.ProjectDir != want {
		t.Errorf("project %q, want %q (the person's Documents)", in.ProjectDir, want)
	}
	if def != "archi" || opened != "archi" {
		t.Errorf("default %q, opened %q; want the main universe archi for both", def, opened)
	}
	if strings.Contains(strings.ToLower(out), "demo") {
		t.Errorf("setup should say nothing of a demo:\n%s", out)
	}
}

// --no-solution skips it, and so does its former name --no-demo.
func TestSetupNoSolution(t *testing.T) {
	for _, flag := range []string{"--no-solution", "--no-demo"} {
		eng, def, opened, out, _ := runSetup(t, "archi@1.2", flag)
		if len(eng.calls) != 0 || def != "" || opened != "" {
			t.Errorf("%s: calls %d, default %q, opened %q; want nothing installed, the GUI opened on no universe\n%s", flag, len(eng.calls), def, opened, out)
		}
	}
}
