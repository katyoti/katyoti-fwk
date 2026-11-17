// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"os"
	"runtime/debug"
	"testing"
)

// A program on disk says whether a release stamped it - and so whether it is
// a development build - without being run: make dev's builds
// carry `cmd.version=dev` and their commit, a release `cmd.version=<v>`.
func TestBuildVersionTellsAReleaseFromADevBuild(t *testing.T) {
	commit := []debug.BuildSetting{{Key: "vcs.revision", Value: "c4ea4719cfbb09498f1b3f28ce6c8dcac405fc43"}, {Key: "vcs.modified", Value: "true"}}
	for _, c := range []struct {
		name, ldflags, want string
		stamped             bool
	}{
		{"make dev", "-s -w -X github.com/katyoti/katyoti-fwk/app/katyoti/cmd.version=dev", "c4ea4719-dirty", false},
		{"no stamp", "", "c4ea4719-dirty", false},
		{"release", "-s -w -X github.com/katyoti/katyoti-fwk/app/ktw-agents/cmd.version=v2026.10-rc5", "v2026.10-rc5", true},
		{"gui release", `-s -w -X main.version=v2026.10-rc7`, "v2026.10-rc7", true},
	} {
		settings := append([]debug.BuildSetting{{Key: "-ldflags", Value: c.ldflags}}, commit...)
		got, stamped := buildVersion(settings)
		if got != c.want || stamped != c.stamped {
			t.Errorf("%s: buildVersion = %q stamped %v, want %q stamped %v", c.name, got, stamped, c.want, c.stamped)
		}
	}
}

// ProgramVersion reads a Go program's build info from its file - this test
// binary, a build with no release stamp - and refuses a file that is no Go
// program.
func TestProgramVersionReadsAProgramWithoutRunningIt(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("no executable path:", err)
	}
	if v, stamped, err := ProgramVersion(exe); err != nil || stamped || v == "" {
		t.Errorf("ProgramVersion(test binary) = %q stamped %v err %v; want an unstamped build", v, stamped, err)
	}
	notGo := t.TempDir() + "/ktw-fake.exe"
	if err := os.WriteFile(notGo, []byte("#!/bin/sh\necho fake\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ProgramVersion(notGo); err == nil {
		t.Error("a file that is no Go program has a version")
	}
}
