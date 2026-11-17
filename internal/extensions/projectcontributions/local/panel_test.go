// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	cwmcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/contrib"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
	contrib "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// The Contributions panel is placed on the system module that operates the
// project's contributions, and on no other system module; its source answers
// what the universe publishes under the project's token, a group per kind.
// (In a test universe every pack publishes under ktw-test: the project takes
// that token.)
func TestTheContributionsPanelListsTheProjectsEntriesByKind(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmcontrib.CorePack(""), cwmapp.SystemModules(t), contrib.Pack(""), local.Extension(), panelocal.Extension())
	mk := func(id string, caps map[string]map[string]any) {
		o := cwmapi.Object{ID: id, Type: cwmapi.SystemModuleType, Name: id, Capabilities: caps}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: id, Object: o}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mk("sys-contributions", map[string]map[string]any{cwmapi.CapSystemModule: {"operates": "contributions"}})
	mk("sys-other", nil)

	r, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: "sys-contributions"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.ProjectRoot, "project.cwm.yaml"), []byte("id: acme\ncapabilities:\n  contributions:\n    provider: ktw-test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	placed := func(id string) (paneapi.ObjectPanel, bool) {
		d, err := pane.Detail(ctx, paneapi.DetailInput{ID: id})
		if err != nil {
			t.Fatalf("Detail(%s): %v", id, err)
		}
		for _, p := range d.Panels {
			if p.ID == "tab:contributions" {
				return p, true
			}
		}
		return paneapi.ObjectPanel{}, false
	}
	p, ok := placed("sys-contributions")
	if !ok || p.Source != "ktw-test:project-contributions/panel" {
		t.Fatalf("panel on sys-contributions = %+v, %v; want the Contributions panel with its source", p, ok)
	}
	if _, ok := placed("sys-other"); ok {
		t.Fatal("the Contributions panel is on a system module that operates nothing")
	}

	svc, err := application.UseService[api.ProjectContributionsService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.Panel(ctx, api.ObjectInput{ID: "sys-contributions"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Provider != "ktw-test" || out.Folder != "modules/sys-contributions" {
		t.Fatalf("provider %q folder %q", out.Provider, out.Folder)
	}
	kinds := map[string]map[string]string{}
	for i, g := range out.Groups {
		if i > 0 && out.Groups[i-1].Kind >= g.Kind {
			t.Errorf("groups out of order: %s before %s", out.Groups[i-1].Kind, g.Kind)
		}
		kinds[g.Kind] = map[string]string{}
		for _, e := range g.Entries {
			kinds[g.Kind][e.ID] = e.URI
			if e.URI == "" {
				t.Errorf("%s/%s has no URI to open it by", g.Kind, e.ID)
			}
		}
	}
	if kinds["object-panel"]["contributions"] != "ktw-test:/object-panel/contributions" || kinds["capability"]["contributions"] == "" {
		t.Fatalf("groups = %v; want the pack's capability and panel, each by kind with its URI", kinds)
	}
}
