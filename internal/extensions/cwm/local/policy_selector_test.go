// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/spf13/cobra"
)

// TestPolicyReachesObjectBySelector is the anti-orphan test for UC-4.
//
// TestPolicyReachesObjectBySelector: a policy a pack contributes (`policies/`,
// the shape config-no-direct-io ships in) reaches every object of the types it
// applies to — through the catalog, into the service — and stays a policy: the
// object's surface names it, its capability set does not carry it.
func TestPolicyReachesObjectBySelector(t *testing.T) {
	root := t.TempDir()
	objDir := filepath.Join(root, "modules", "demo", "objects")
	if err := os.MkdirAll(objDir, 0o755); err != nil {
		t.Fatal(err)
	}
	objYAML := `id: demo.thing
type: use-case
name: a thing
parent: demo
capabilities:
  want: {reachability: closable}
`
	if err := os.WriteFile(filepath.Join(objDir, "thing.cwm.yaml"), []byte(objYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	pack := manifestcontrib.ContributePackPart("", "zz-test", "cwm", fstest.MapFS{
		"policies/zz-test-policy.yaml": {Data: []byte("name: zz-test-policy\nlabel: Seeded by the test\n" +
			"applies: {types: [\"*\"]}\n")},
	})
	app := application.New(
		config.NewConfigService(loader.NewMapLoader(nil)),
		func(init *application.ApplicationInit) {
			init.AddContribute("test", ".root", func(app *application.Application) error {
				application.PutService[appcore.CobraCommand](app, "root", appcore.CobraCommand{Cmd: &cobra.Command{Use: "ktw-test"}})
				return nil
			})
		},
		manifestlocal.Extension(),
		ExtensionWith(Options{WorkspaceRoot: root}),
		pack,
	)
	app.SetProvider("ktw-test")
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	ctx := application.WithApplication(context.Background(), app)
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		t.Fatalf("resolve CWMService: %v", err)
	}

	surf, err := cwm.ObjectEffectiveSurface(ctx, cwmapi.ObjectEffectiveSurfaceInput{ID: "demo.thing"})
	if err != nil {
		t.Fatalf("ObjectEffectiveSurface: %v", err)
	}
	found := false
	for _, p := range surf.Policies {
		found = found || p == "zz-test-policy"
	}
	if !found {
		t.Errorf("policies=%v — the pack's '*' policy did not reach the object", surf.Policies)
	}
	eff, err := cwm.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: "demo.thing"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range eff.Capabilities {
		if c.ID == "zz-test-policy" {
			t.Errorf("a policy is not a capability: %+v", eff.Capabilities)
		}
	}
}
