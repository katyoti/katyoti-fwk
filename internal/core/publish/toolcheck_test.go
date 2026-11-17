// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestVersionMatches(t *testing.T) {
	cases := []struct {
		found, want string
		ok          bool
	}{
		{"0.15.1 (9dfd3a08)", "0.15.1", true},
		{"3.11", "3.11", true},
		{"3.11.1", "3.11", false},
		{"3.110", "3.11", false},
		{"0.12.0 (737895d7)", "0.15.1", false},
		{"anything", "", true},
	}
	for _, c := range cases {
		if got := VersionMatches(c.found, c.want); got != c.ok {
			t.Errorf("VersionMatches(%q, %q) = %v, want %v", c.found, c.want, got, c.ok)
		}
	}
}

// A found tool at another version than the validated one is not OK, and says
// which version it should be and how to install it.
func TestCheckToolReportsAnotherVersion(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	tool := ToolDef{Name: "go", Commands: []string{"go"}, VersionFlag: "version", Version: "0.0.1", InstallSkill: "x:/skill/install-go"}
	dep := CheckTool(tool, context.Background())
	if dep == nil || dep.OK || dep.FoundVersion == "" || dep.RequiredVersion != "0.0.1" || dep.InstallSkill != tool.InstallSkill {
		t.Fatalf("CheckTool = %+v, want not OK with found/required versions and the install skill", dep)
	}
	tool.Version = ""
	if dep := CheckTool(tool, context.Background()); dep == nil || !dep.OK {
		t.Fatalf("without Version, a found tool is OK: %+v", dep)
	}
}

// A command that answers its version flag with words and no version (the
// Windows Store's python stub) is not the program: the next command is tried,
// and none left means missing.
func TestCheckToolPassesACommandAnsweringNoVersion(t *testing.T) {
	dir := t.TempDir()
	script := func(name, line string) {
		var path, body string
		if runtime.GOOS == "windows" {
			path, body = filepath.Join(dir, name+".cmd"), "@echo "+line+"\r\n"
		} else {
			path, body = filepath.Join(dir, name), "#!/bin/sh\necho '"+line+"'\n"
		}
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script("stubpy", "Python was not found; run without arguments to install from the Microsoft Store")
	script("realpy", "Python 3.12.10")
	t.Setenv("PATH", dir)
	tool := ToolDef{Name: "python", Commands: []string{"stubpy"}, VersionFlag: "--version"}
	if dep := CheckTool(tool, context.Background()); dep == nil || dep.OK {
		t.Fatalf("only the stub: %+v, want missing", dep)
	}
	tool.Commands = []string{"stubpy", "realpy"}
	if dep := CheckTool(tool, context.Background()); dep == nil || !dep.OK || dep.FoundVersion != "3.12.10" {
		t.Fatalf("stub then python: %+v, want ok 3.12.10", dep)
	}
}
