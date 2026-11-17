// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/projectcmd"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// /: a case's run step and command check run in
// bash on every platform, in the case's project - bash-only syntax (a
// negation, [[ ]], $(...)) works; the exit code is the check's.
func TestScenarioCommandsRunInBash(t *testing.T) {
	if _, err := projectcmd.FindBash(); err != nil {
		t.Skipf("no bash on this machine: %v", err)
	}
	dir := t.TempDir()
	e := &Engine{Case: &api.Case{Name: "demo"}, RT: Runtime{Dir: dir}}
	if err := os.MkdirAll(filepath.Join(e.RT.Project(), "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, out := e.runCmd(context.Background(), api.RunStep{Cmd: `[[ -d sub ]] && echo "$(basename "$PWD")" > made.txt`})
	if code != 0 {
		t.Fatalf("a bash run step: exit %d %q", code, out)
	}
	if b, err := os.ReadFile(filepath.Join(e.RT.Project(), "made.txt")); err != nil || strings.TrimSpace(string(b)) != "project" {
		t.Fatalf("the run step did not run in the project: %q %v", b, err)
	}
	one := 1
	for cmd, want := range map[api.RunStep]bool{
		{Cmd: `! grep -l "Remplir :" made.txt`}:         true,
		{Cmd: `test $(grep -c project made.txt) -ge 1`}: true,
		{Cmd: `! grep -q project made.txt`}:             false,
		{Cmd: `grep -q nothing made.txt`, Exit: &one}:   true,
	} {
		if ok, why := e.check(context.Background(), nil, api.Check{Cmd: &cmd}, State{}); ok != want {
			t.Errorf("check %q = %v (%s); want %v", cmd.Cmd, ok, why, want)
		}
	}
}
