// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// currentInstallation is this process's installation (nil: a development
// build). A variable so a test gives one of its own.
var currentInstallation = cenv.CurrentInstallation

// ProjectsFolder is the installation's projects folder.
func (s *service) ProjectsFolder(_ context.Context) (mktapi.ProjectsFolderOutput, error) {
	return projectsFolderOf(currentInstallation()), nil
}

// SetProjectsFolder chooses it: refused inside the installation, created when
// missing, recorded in katyoti.yml.
func (s *service) SetProjectsFolder(_ context.Context, in mktapi.SetProjectsFolderInput) (mktapi.ProjectsFolderOutput, error) {
	inst := currentInstallation()
	if err := inst.CheckProjectsFolder(in.Folder); err != nil {
		return projectsFolderOf(inst), err
	}
	if err := os.MkdirAll(in.Folder, 0o755); err != nil {
		return projectsFolderOf(inst), err
	}
	if err := inst.SetProjectsFolder(in.Folder); err != nil {
		return projectsFolderOf(inst), err
	}
	return projectsFolderOf(inst), nil
}

func projectsFolderOf(inst *cenv.Installation) mktapi.ProjectsFolderOutput {
	dir, chosen := inst.ProjectsFolder()
	out := mktapi.ProjectsFolderOutput{Folder: dir, Chosen: chosen, Proposed: cenv.DefaultProjectsFolder()}
	if inst != nil {
		out.Installation = inst.Dir()
	}
	return out
}
