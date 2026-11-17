// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// The installation diagnosed is the one this katyoti serves - here the test
// world's, in the temporary folder - so the temporary folder marks no row a
// test leftover there.
func TestInstallationFiles_TestInstallation(t *testing.T) {
	f, err := installationFiles()
	if err != nil {
		t.Fatal(err)
	}
	root := application.Env().GlobalHomeDir()
	if !cenv.Within(root, os.TempDir()) {
		t.Fatalf("the test world's root %s is not temporary", root)
	}
	if len(f.TempDirs) != 0 {
		t.Errorf("an installation in the temporary folder marks its rows as test leftovers: %v", f.TempDirs)
	}
	for _, p := range []string{f.Universes, f.Sidecars, f.Marketplaces, f.ControlPlane} {
		if filepath.Dir(p) != filepath.Clean(root) {
			t.Errorf("%s is not in the installation's root %s", p, root)
		}
	}
	if !cenv.Within(f.Instances, os.TempDir()) {
		t.Errorf("the registry of installations %s is the person's", f.Instances)
	}
}

// The doctor repairs the installation this katyoti serves - here a root of
// the test's own - and backs up into that installation's own backups
// folder: a row to a vanished path is removed, its registry copied first
func TestRepairInstallation_OwnRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	t.Setenv("KTW_KATYOTI_ROOT", root)
	t.Setenv(cenv.InstancesEnv, filepath.Join(base, "katyoti-instances.yaml"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	reg := filepath.Join(root, "universes.yaml")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	gone := filepath.ToSlash(filepath.Join(root, "no-such-universe"))
	if err := os.WriteFile(reg, []byte("universes:\n  - {name: gone-u, home_path: '"+gone+"'}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, err := NewInfra()
	if err != nil {
		t.Fatal(err)
	}
	rep, err := svc.RepairInstallation(context.Background(), mktapi.RepairInstallationInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Actions) != 1 || rep.Actions[0].Problem.Entry != "gone-u" || !rep.Actions[0].Done {
		t.Fatalf("want the vanished universe repaired, got %+v", rep.Actions)
	}
	if !cenv.Within(rep.Backup, filepath.Join(root, "backups")) {
		t.Errorf("backup %s is not in the installation's backups folder", rep.Backup)
	}
	if data, _ := os.ReadFile(reg); strings.Contains(string(data), "gone-u") {
		t.Errorf("universes.yaml still holds the vanished row:\n%s", data)
	}
}
