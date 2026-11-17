// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/diagnose"
)

// DiagnoseInstallation names what is broken in this installation
// , from its own files.
func (s *infraOps) DiagnoseInstallation(_ context.Context) (mktapi.InstallationDiagnosis, error) {
	f, err := installationFiles()
	if err != nil {
		return mktapi.InstallationDiagnosis{}, err
	}
	return diagnose.Diagnose(f), nil
}

// RepairInstallation repairs what DiagnoseInstallation names
// every file it changes copied first into <root>/backups/doctor-<time>.
func (s *infraOps) RepairInstallation(_ context.Context, in mktapi.RepairInstallationInput) (mktapi.InstallationRepair, error) {
	f, err := installationFiles()
	if err != nil {
		return mktapi.InstallationRepair{}, err
	}
	return diagnose.Repair(f, filepath.Join(application.Env().GlobalHomeDir(), "backups"), in.DryRun)
}

// installationFiles are the files of the installation this katyoti serves.
// The temporary folder marks a test's leftovers - unless the installation
// itself lives there (a test's own installation).
func installationFiles() (diagnose.Files, error) {
	root := application.Env().GlobalHomeDir()
	if root == "" {
		return diagnose.Files{}, fmt.Errorf("diagnose: global home dir unresolved")
	}
	instances, err := controlplane.InstallationsRegistryPath()
	if err != nil {
		return diagnose.Files{}, err
	}
	f := diagnose.Files{
		Instances:    instances,
		Universes:    filepath.Join(root, "universes.yaml"),
		Sidecars:     filepath.Join(root, "sidecars.yaml"),
		Marketplaces: filepath.Join(root, "marketplaces.yaml"),
		ControlPlane: filepath.Join(root, "control-plane"),
	}
	if tmp := os.TempDir(); !cenv.Within(root, tmp) {
		f.TempDirs = []string{tmp}
	}
	return f, nil
}
