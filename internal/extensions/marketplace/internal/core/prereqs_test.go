// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/prereqs"
)

// A solution's members' config checks give the plan: two members declaring
// one program (read from their own declarations, by no name) are one row, the
// script recipe resolved in the folder of the member that ships it, the
// optional program offered, not selected.
func TestASolutionsMembersChecksGiveThePlan(t *testing.T) {
	member := func(name, decl string) mktapi.Installed {
		folder := t.TempDir()
		if err := os.WriteFile(filepath.Join(folder, "ktw-host.yaml"), []byte(decl), 0o644); err != nil {
			t.Fatal(err)
		}
		return mktapi.Installed{Name: name, Kind: "content", FolderPath: folder}
	}
	recipe := "      recipes:\n        - platform: " + runtime.GOOS + "\n          kind: script\n          script: install/prog.sh\n"
	a := member("notes", "name: notes\nconfig:\n  tools:\n    - name: prog\n      commands: [ktw-test-no-such-program]\n      version: \"1.2\"\n"+recipe+
		"    - name: extra\n      optional: true\n      commands: [ktw-test-no-such-extra]\n"+recipe)
	b := member("cards", "name: cards\nconfig:\n  tools:\n    - name: prog\n      commands: [ktw-test-no-such-program]\n      version: \"1.10\"\n"+recipe)
	prev := hostBinaryPath
	hostBinaryPath = func() string { return filepath.Join(t.TempDir(), "no-ktw-host.exe") }
	t.Cleanup(func() { hostBinaryPath = prev })

	home := t.TempDir()
	s := &service{prereqs: &prereqs.Engine{Runner: noInstall{}, Platform: runtime.GOOS}}
	members := []mktapi.Installed{a, b}
	reports := make([]*sidecarapi.ConfigCheckReport, len(members))
	for i, ins := range members {
		rep, err := s.configureInstalled(context.Background(), home, ins)
		if err != nil {
			t.Fatal(err)
		}
		reports[i] = &rep
	}
	plan := s.solutionPrereqs(members, reports)
	if len(plan.Programs) != 2 {
		t.Fatalf("programs %+v, want prog and extra", plan.Programs)
	}
	var prog, extra mktapi.PlannedProgram
	for _, p := range plan.Programs {
		switch p.Name {
		case "prog":
			prog = p
		case "extra":
			extra = p
		}
	}
	if len(prog.NeededBy) != 2 || prog.Version != "1.10" || !prog.Selected || prog.State != mktapi.PrereqInstall {
		t.Errorf("prog = %+v; want needed by both, 1.10, selected", prog)
	}
	if prog.ScriptPath != filepath.Join(b.FolderPath, "install", "prog.sh") {
		t.Errorf("script %q, want it in the folder of cards (the higher version's)", prog.ScriptPath)
	}
	if extra.Required || extra.Selected || extra.State != mktapi.PrereqInstall {
		t.Errorf("extra = %+v; want optional, offered, not selected", extra)
	}
}

// noInstall is a runner a test cannot install anything with.
type noInstall struct{}

func (noInstall) Available(string) bool { return true }
func (noInstall) Install(context.Context, prereqs.Step) error {
	panic("a test installed a program")
}
func (noInstall) InstallElevated(context.Context, []prereqs.Step) error {
	panic("a test installed a program")
}
func (noInstall) AddToPath(string) error { panic("a test changed PATH") }
func (noInstall) RefreshPath() error     { return nil }
