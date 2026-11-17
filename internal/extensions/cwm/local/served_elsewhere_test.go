// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/spf13/cobra"
)

// A CLIENT OF ITS RUNNING UNIVERSE OPENS NO WORKSPACE:
// when ServedElsewhere answers true, resolving the CWM service takes no lock
// and writes nothing under the workspace root; when it answers false, the
// workspace is opened as always (its lock taken).
func TestServedElsewhereOpensNoWorkspace(t *testing.T) {
	open := func(t *testing.T, served bool) string {
		t.Helper()
		root := t.TempDir()
		all := []application.Extension{
			func(init *application.ApplicationInit) {
				init.AddContribute("test", ".root", func(app *application.Application) error {
					application.PutService[appcore.CobraCommand](app, "root", appcore.CobraCommand{Cmd: &cobra.Command{Use: "ktw-test"}})
					return nil
				})
			},
			manifestlocal.Extension(),
			ExtensionWith(Options{WorkspaceRoot: root, ObjectTypeValidation: cwmapi.TypeValidationWarn,
				ServedElsewhere: func(context.Context) bool { return served }}),
		}
		app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), all...)
		app.SetProvider("ktw-test")
		if err := app.Run(); err != nil {
			t.Fatalf("app.Run: %v", err)
		}
		// Registered after t.TempDir, so it runs first: the host's open
		// workspace (its lock, its index) is closed before the root is
		// removed - Windows refuses to remove a folder a handle keeps.
		t.Cleanup(func() { appcore.CloseApplicationScope(application.WithApplication(context.Background(), app)) })
		if _, err := application.UseService[cwmapi.CWMService](app.Context()); err != nil {
			t.Fatalf("resolve CWMService: %v", err)
		}
		return root
	}

	root := open(t, true)
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("a client wrote into the workspace root: %v (no lock, no first-run file is owed)", names)
	}

	root = open(t, false)
	if _, err := os.Stat(filepath.Join(root, ".lock")); err != nil {
		t.Fatalf("the host did not take the workspace lock: %v", err)
	}
}
