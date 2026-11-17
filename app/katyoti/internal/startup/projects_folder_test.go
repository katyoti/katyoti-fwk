// SPDX-License-Identifier: Apache-2.0

package startup

import (
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// `katyoti config set projects_folder <dir>` records a full path, refuses a
// relative one, and the folder survives a later set of another key (Save
// rewrites every setting from the config).
func TestTheProjectsFolderIsAStartupSettingKeptAcrossSets(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	if _, err := Set("projects_folder", "relative/projects"); err == nil {
		t.Fatal("a relative projects folder was accepted")
	}
	want := filepath.Join(t.TempDir(), "projects")
	if _, err := Set("projects_folder", want); err != nil {
		t.Fatal(err)
	}
	if _, err := Set("launch_mode", LaunchCLI); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProjectsFolder != want || cfg.LaunchMode != LaunchCLI {
		t.Errorf("projects folder %q launch mode %q, want %q kept after setting launch_mode", cfg.ProjectsFolder, cfg.LaunchMode, want)
	}
}
