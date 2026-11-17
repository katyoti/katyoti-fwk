// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// A universe created with a project opens on it: the folder is created and
// attached — listed by the universe, pointing back at it — so CWM gives it its
// project object, inbox and planification the first time it opens. Without
// one, the universe holds no object at all.
func TestANewUniverseCanComeWithItsProject(t *testing.T) {
	tempDir := t.TempDir()
	registry, _ := catalog.LoadRegistry(filepath.Join(tempDir, "universes.yaml"))
	m := &Service{registry: registry}
	isolateDataDir(t)

	project := filepath.Join(tempDir, "Katyoti", "nouveau")
	if _, err := m.CreateUniverse(context.Background(), api.CreateUniverseInput{
		Name: "nouveau", HomePath: filepath.Join(tempDir, "universes", "nouveau"), ProjectDir: project,
	}); err != nil {
		t.Fatalf("CreateUniverse: %v", err)
	}
	if got := m.loadProjects("nouveau").Projects; len(got) != 1 || got[0] != project {
		t.Fatalf("the universe should serve its new project %s, serves %v", project, got)
	}
	if _, err := os.Stat(filepath.Join(application.EnvOf(project).HomeDir(), "project.yaml")); err != nil {
		t.Fatalf("the project should point back at its universe: %v", err)
	}

	if _, err := m.CreateUniverse(context.Background(), api.CreateUniverseInput{
		Name: "bare", HomePath: filepath.Join(tempDir, "universes", "bare"),
	}); err != nil {
		t.Fatalf("CreateUniverse: %v", err)
	}
	if got := m.loadProjects("bare").Projects; len(got) != 0 {
		t.Fatalf("a universe created without a project should serve none, serves %v", got)
	}
}
