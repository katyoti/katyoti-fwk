// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/sandbox"
)

// A marketplace build runs outside every case's sandbox: it installs a
// solution's case (or refreshes its binaries) through the scenario command run
// inside that case's sandbox, never in its own process — where the install
// would put the sandbox's admin universe outside the case.
func TestSolutionInstallsTheCaseInsideItsSandbox(t *testing.T) {
	t.Setenv(sandbox.Marker, "")
	cases := t.TempDir()
	caseDir := filepath.Join(cases, "demo")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, api.CaseFile), []byte("name: demo\nuniverse:\n  sidecars: [ktw-agents]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()

	ran := errors.New("ran in the sandbox")
	var got *exec.Cmd
	keep := runScenario
	runScenario = func(cmd *exec.Cmd) error { got = cmd; return ran }
	t.Cleanup(func() { runScenario = keep })

	_, err := Solution(context.Background(), SolutionInput{Case: caseDir, Root: root, Bin: filepath.Join(root, "bin")})
	if got == nil {
		t.Fatalf("the case was not installed through the scenario command in its sandbox (err: %v)", err)
	}
	if !errors.Is(err, ran) {
		t.Fatalf("Solution's error = %v, want the scenario command's", err)
	}
	args := got.Args[1:]
	for _, want := range []string{"scenario", "install", "demo", "--force", "--cases", "--root", "--bin"} {
		if !slices.Contains(args, want) {
			t.Errorf("args %v: %q missing", args, want)
		}
	}
	runtime := sandbox.Of(filepath.Join(root, "demo")).Case
	env := strings.Join(got.Env, "\n")
	if !strings.Contains(env, sandbox.Marker+"="+runtime) {
		t.Errorf("the command does not run in the case's sandbox %s", runtime)
	}
	if !strings.Contains(env, "KTW_KATYOTI_ROOT="+sandbox.Of(runtime).Root()) {
		t.Errorf("the command's global root is not the sandbox's %s", sandbox.Of(runtime).Root())
	}
}
