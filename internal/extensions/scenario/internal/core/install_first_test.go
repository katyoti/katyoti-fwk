// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// universeExists answers every universe as existing: Install stops at the
// universe's own check, after the runtime folder's - which one refused tells
// which check let it through.
type universeExists struct{ orchapi.OrchestratorService }

func (universeExists) GetUniverse(context.Context, orchapi.GetUniverseInput) (orchapi.UniverseInfo, error) {
	return orchapi.UniverseInfo{}, nil
}

// A FIRST INSTALL NEEDS NO --force: on a root where the case
// never ran - its runtime folder holding only the sandbox's machine folder -
// Install passes the runtime check without --force; a second install, its
// runtime folder holding a previous install, is still refused without it.
func TestAFirstInstallIsNotRefusedAsExisting(t *testing.T) {
	app := application.New(application.Config())
	application.PutService[orchapi.OrchestratorService](app, "default", universeExists{})
	ctx := application.WithApplication(context.Background(), app)
	e := NewEngine(&api.Case{Name: "demo"}, t.TempDir(), "", nil)
	if err := os.MkdirAll(filepath.Join(e.RT.Dir, "machine", "root"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := e.Install(ctx, false)
	if err == nil || strings.Contains(err.Error(), "reinstall the case from scratch") || !strings.Contains(err.Error(), "universe") {
		t.Fatalf("a first install: %v; want it past the runtime check (stopped at the universe's)", err)
	}
	if err := os.WriteFile(filepath.Join(e.RT.Dir, "state.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.Install(ctx, false); err == nil || !strings.Contains(err.Error(), "reinstall the case from scratch") {
		t.Fatalf("a second install without --force: %v; want refused as existing", err)
	}
}
