// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// : a program pip installs is checked as a Python package of the
// Python pip installs with - found by its metadata although no command of its
// name is on PATH (pip's scripts folder is on none) - and reported missing
// once it is not installed.
func TestAPythonPackageIsCheckedAsAPackageNotACommand(t *testing.T) {
	if PythonCommand() == "" {
		t.Skip("no Python on this machine")
	}
	site := t.TempDir()
	const pkg = "ktwprobepkg"
	info := filepath.Join(site, pkg+"-1.2.3.dist-info")
	if err := os.MkdirAll(info, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := "Metadata-Version: 2.1\nName: " + pkg + "\nVersion: 1.2.3\n"
	if err := os.WriteFile(filepath.Join(info, "METADATA"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PYTHONPATH", site)

	dep := CheckTool(ToolDef{Name: pkg, PythonPackage: pkg, InstallSkill: "x:/skill/install"}, context.Background())
	if dep == nil || !dep.OK || dep.FoundVersion != "1.2.3" {
		t.Fatalf("an installed Python package with no command is not found: %+v", dep)
	}

	dep = CheckTool(ToolDef{Name: "ktwabsentpkg", PythonPackage: "ktwabsentpkg", InstallSkill: "x:/skill/install"}, context.Background())
	if dep == nil || dep.OK || dep.InstallSkill == "" {
		t.Fatalf("a package that is not installed is found: %+v", dep)
	}
}
