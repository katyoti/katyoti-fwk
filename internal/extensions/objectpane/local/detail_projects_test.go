// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// paneMember makes a member repository holding its project object of key;
// its module m is created through the CWM once it is served.
func paneMember(t *testing.T, root, key string) string {
	t.Helper()
	dir := filepath.Join(root, "repo-"+key)
	if err := os.MkdirAll(filepath.Join(dir, "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, cwmapi.ProjectFileName), []byte("id: "+key+"\ntype: project\nname: "+key+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// paneMembers runs the object pane over a real CWM serving the members given
// (the store set when there are several), each holding module m - created by
// its qualified module when there are several.
func paneMembers(t *testing.T, root string, keys ...string) (context.Context, cwmapi.CWMService, api.ObjectPaneService) {
	t.Helper()
	return paneMembersWith(t, root, nil, keys...)
}

// paneMembersWith is paneMembers with more pack files (a type replaced by
// one of files when it names the same path).
func paneMembersWith(t *testing.T, root string, files map[string]string, keys ...string) (context.Context, cwmapi.CWMService, api.ObjectPaneService) {
	t.Helper()
	return paneMembersServed(t, root, files, nil, keys...)
}

// paneMembersServed is paneMembersWith with more extensions served beside
// the CWM and the pane (a journal, an event bus).
func paneMembersServed(t *testing.T, root string, files map[string]string, more []application.Extension, keys ...string) (context.Context, cwmapi.CWMService, api.ObjectPaneService) {
	t.Helper()
	pack := fstest.MapFS{
		"object-types/module.yaml":   {Data: []byte("name: module\ntree: node\n")},
		"object-types/thing.yaml":    {Data: []byte("name: thing\ntree: node\n")},
		"capabilities/alphacap.yaml": {Data: []byte("name: alphacap\n")},
		"capabilities/betacap.yaml":  {Data: []byte("name: betacap\n")},
		"aspects/alpha-ish.yaml":     {Data: []byte("name: alpha-ish\ncapabilities: [alphacap]\n")},
		"aspects/beta-ish.yaml":      {Data: []byte("name: beta-ish\ncapabilities: [betacap]\n")},
		"edge-types/depends-on.yaml": {Data: []byte("name: depends-on\nfrom: any\nto: any\ncardinality: M:N\ntarget: object\nobject_storage: edges\ninverse_name: needed-by\n")},
	}
	for p, body := range files {
		pack[p] = &fstest.MapFile{Data: []byte(body)}
	}
	members := make([]string, 0, len(keys))
	for _, k := range keys {
		members = append(members, paneMember(t, root, k))
	}
	ctx, cwm := cwmapp.Members(t, root, members, append([]application.Extension{cwmapp.Pack(pack), panelocal.Extension()}, more...)...)
	for _, k := range keys {
		module := "m"
		if len(keys) > 1 {
			module = k + ":m"
		}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: cwmapi.NewContainerObject("m", "m", "module", "", "m", "")}); err != nil {
			t.Fatalf("create module %s: %v", module, err)
		}
	}
	svc, err := application.UseService[api.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc
}

// createIn creates an object in module m of the project key ("" with one).
func createIn(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, key string, o cwmapi.Object) {
	t.Helper()
	module := "m"
	if key != "" {
		module = key + ":m"
	}
	if o.Parent == "" {
		o.Parent = "m"
	}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o}); err != nil {
		t.Fatalf("create %s in %s: %v", o.ID, module, err)
	}
}

// THE DETAIL NAMES THE PROJECT HOLDING THE OBJECT (
// ). A universe serves alpha and beta, both holding the bare id
// - each with its own capability, an incoming edge from its own m.S-1
// and a child of its own. The detail of alpha: names project alpha, with
// alpha's capability, alpha's source as an INCOMING link titled from alpha's
// row and alpha's child only; the same for beta. An id only alpha holds opens
// bare and names alpha. The bare homonym is refused, naming the project to
// name. One project served alone answers no project key.
func TestTheDetailNamesTheProjectHoldingTheObject(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(root, "installation"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	ctx, cwm, svc := paneMembers(t, root, "alpha", "beta")
	for _, k := range []string{"alpha", "beta"} {
		createIn(t, ctx, cwm, k, cwmapi.Object{ID: "m.F-1", Type: "thing", Name: k + " feature", Aspects: []string{k + "-ish"}})
		createIn(t, ctx, cwm, k, cwmapi.Object{ID: "m.S-1", Type: "thing", Name: k + " source",
			Edges: []cwmapi.ObjectEdge{{Kind: "depends-on", To: "m.F-1"}}})
	}
	createIn(t, ctx, cwm, "alpha", cwmapi.Object{ID: "m.C-1", Type: "thing", Name: "alpha child", Parent: "m.F-1"})
	createIn(t, ctx, cwm, "beta", cwmapi.Object{ID: "m.C-2", Type: "thing", Name: "beta child", Parent: "m.F-1"})
	createIn(t, ctx, cwm, "alpha", cwmapi.Object{ID: "m.U-1", Type: "thing", Name: "only alpha's"})

	for _, c := range []struct{ key, capability, other, child string }{
		{"alpha", "alphacap", "betacap", "m.C-1"},
		{"beta", "betacap", "alphacap", "m.C-2"},
	} {
		for _, body := range []bool{true, false} {
			d := detail(t, ctx, svc, c.key+":m.F-1", body)
			if d.Project != c.key || d.Object.ID != "m.F-1" || d.Object.Name != c.key+" feature" {
				t.Errorf("%s:m.F-1 (body %v): project %q, object %s %q - want %s's, its id bare", c.key, body, d.Project, d.Object.ID, d.Object.Name, c.key)
			}
			caps := strings.Join(d.Capabilities, ",")
			if !strings.Contains(caps, c.capability) || strings.Contains(caps, c.other) {
				t.Errorf("%s:m.F-1 capabilities %v: want %s's own only", c.key, d.Capabilities, c.key)
			}
			if len(d.Links) != 1 || d.Links[0].Direction != "in" || d.Links[0].TargetID != "m.S-1" ||
				!d.Links[0].Resolved || d.Links[0].TargetTitle != c.key+" source" {
				t.Errorf("%s:m.F-1 links %+v: want one incoming from its own m.S-1, titled %q", c.key, d.Links, c.key+" source")
			}
			var children []string
			for _, g := range d.Children {
				for _, o := range g.Objects {
					children = append(children, o.ID)
				}
			}
			if strings.Join(children, ",") != c.child {
				t.Errorf("%s:m.F-1 children %v: want its own %s only", c.key, children, c.child)
			}
		}
		// The source's link to its feature resolves to its own project's row.
		src := detail(t, ctx, svc, c.key+":m.S-1", false)
		if len(src.Links) != 1 || src.Links[0].Direction != "out" || src.Links[0].TargetTitle != c.key+" feature" {
			t.Errorf("%s:m.S-1 links %+v: want one outgoing to its own feature", c.key, src.Links)
		}
	}

	if d := detail(t, ctx, svc, "m.U-1", false); d.Project != "alpha" {
		t.Errorf("m.U-1 opened bare names project %q, want alpha (the only one holding it)", d.Project)
	}
	if _, err := svc.Detail(ctx, api.DetailInput{ID: "m.F-1"}); err == nil || !strings.Contains(err.Error(), "name the project") {
		t.Errorf("the bare homonym m.F-1 = %v; want refused asking to name the project", err)
	}

	// One project served alone: no project key in the answer.
	application.ResetGlobals()
	one := t.TempDir()
	ctx1, cwm1, svc1 := paneMembers(t, one, "solo")
	createIn(t, ctx1, cwm1, "", cwmapi.Object{ID: "m.F-1", Type: "thing", Name: "solo feature"})
	d := detail(t, ctx1, svc1, "m.F-1", false)
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	if _, has := top["project"]; d.Project != "" || has {
		t.Errorf("one project served: project %q, the answer carries a project key: %v", d.Project, has)
	}
}
