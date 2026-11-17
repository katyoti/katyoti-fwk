// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// panelsOf boots an application with the pack, and
// returns what the pack publishes as `object-panel`.
func panelsOf(t *testing.T, pack fstest.MapFS) []contribapi.Contribution {
	t.Helper()
	points := func(init *application.ApplicationInit) {
		init.AddRegisterPoints("test", ".providers", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
		})
	}
	app := application.New(application.Config(), points, manifestcontrib.ContributePackPart("ktw-x", "x", "cwm", pack))
	if err := app.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		if p.Type() == api.ObjectPanelKind {
			got, err := p.Enumerate(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			return got
		}
	}
	t.Fatal("no object-panel provider: a pack reader reads every CWM kind")
	return nil
}

// TestObjectPanelYAMLKeepsEveryDefField — a panel is authored in YAML, and a
// render.Def field the YAML decoder cannot name is DROPPED WITHOUT A WORD.
// Found 2026-09-25 in the running GUI: Def carried only json tags, so every
// camelCase key (dslTree) vanished from a pack while kind and
// extension — already lowercase — survived, and the confluence tab rendered
// with no descriptor.
//
// Also pinned: the loader's variable expansion must leave an extension's own
// templates alone (method-panel's ${opt.*} is resolved at click time).
func TestObjectPanelYAMLKeepsEveryDefField(t *testing.T) {
	got := panelsOf(t, fstest.MapFS{"object-panels/p.yaml": &fstest.MapFile{Data: []byte(`
name: P
filter: {capabilities: [x]}
content:
  kind: extension
  extension: method-panel
  schema: "p:/data-type/s"
  variant: F
  ui: {groups: []}
  dslTree: {kind: vbox}
  data:
    options: [{id: dryRun, default: true}]
    actions: [{method: "p:s/m", args: {path: "${node.workspace_path}/c", dry: "${opt.dryRun}"}}]
`)}})
	if len(got) != 1 || got[0].ID != "p" {
		t.Fatalf("panels = %+v, want p (the file's name)", got)
	}
	var d api.PanelDecl
	if err := json.Unmarshal(got[0].Data(), &d); err != nil || d.Content == nil {
		t.Fatalf("decode: %v %+v", err, d)
	}
	c := d.Content
	if c.Schema != "p:/data-type/s" || c.Variant != "F" || c.UI == nil || c.DSLTree == nil {
		t.Errorf("camelCase Def fields dropped: %+v", c)
	}
	v, ok := c.Data.(map[string]any)
	if !ok || v["actions"] == nil {
		t.Fatalf("data dropped: %#v", c.Data)
	}
	acts, _ := v["actions"].([]any)
	args, _ := acts[0].(map[string]any)["args"].(map[string]any)
	if args["dry"] != "${opt.dryRun}" || args["path"] != "${node.workspace_path}/c" {
		t.Errorf("an extension's templates were rewritten at load: %v", args)
	}
}
