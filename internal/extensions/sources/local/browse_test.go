// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// wikiPages is the fake wiki's tree: a space and its three pages.
var wikiPages = []string{"p1", "p2", "p3"}

// browseActions is the ActionService of the browse tests: the local-folder
// connector routed to the service; a fake wiki that browses its tree and
// syncs the pages picked (all of them when none is); a fake drive that only
// syncs. selections records the selection each wiki sync received.
type browseActions struct {
	mu         *sync.Mutex
	selections *[]map[string]any
}

func (a browseActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a browseActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	var value any
	switch in.URI {
	case "ktw-agents:sources/folder-sync":
		var sin api.ConnectorSyncInput
		_ = json.Unmarshal(in.Input, &sin)
		if value, err = svc.FolderSync(ctx, sin); err != nil {
			return actionsapi.RunOutput{}, err
		}
	case "ktw-agents:sources/folder-browse":
		var bin api.ConnectorBrowseInput
		_ = json.Unmarshal(in.Input, &bin)
		if value, err = svc.FolderBrowse(ctx, bin); err != nil {
			return actionsapi.RunOutput{}, err
		}
	case "ktw-wiki:wiki/browse":
		var bin api.ConnectorBrowseInput
		_ = json.Unmarshal(in.Input, &bin)
		out := api.ConnectorBrowseOutput{Items: []api.BrowseItem{}}
		switch bin.Parent {
		case "":
			out.Items = append(out.Items, api.BrowseItem{Key: "space", Title: "Space", Kind: "space", HasChildren: true})
		case "space":
			for _, p := range wikiPages {
				out.Items = append(out.Items, api.BrowseItem{Key: p, Title: strings.ToUpper(p), Kind: "page"})
			}
		}
		value = out
	case "ktw-wiki:wiki/source-sync", "ktw-drive:drive/source-sync":
		var sin api.ConnectorSyncInput
		_ = json.Unmarshal(in.Input, &sin)
		a.mu.Lock()
		*a.selections = append(*a.selections, sin.Selection)
		a.mu.Unlock()
		pages := wikiPages
		if items := api.SelectedItems(sin.Selection); items != nil {
			pages = nil
			for _, it := range items {
				pages = append(pages, it.Key)
			}
		}
		out := api.ConnectorSyncOutput{Items: []api.ConnectorItem{}}
		for _, p := range pages {
			if err := os.WriteFile(filepath.Join(sin.Dest, p+".md"), []byte("# "+p+"\n\nPage "+p+".\n"), 0o644); err != nil {
				return actionsapi.RunOutput{}, err
			}
			out.Items = append(out.Items, api.ConnectorItem{Path: p + ".md", URI: "wiki://" + p, Title: p})
		}
		value = out
	case "ktw-agents:sources/source-check":
		// Sources' save gate: the service in the process.
		svc, err := application.UseService[api.SourcesService](ctx)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		var cin api.SourceCheckInput
		if err := json.Unmarshal(in.Input, &cin); err != nil {
			return actionsapi.RunOutput{}, err
		}
		got, err := svc.SourceCheck(ctx, cin)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		value = got
	case "ktw-wiki:gen/source-sync":
		// A connector that recognises a generator's mark and flags it but
		// leaves nothing out: one human page, one generated.
		var sin api.ConnectorSyncInput
		_ = json.Unmarshal(in.Input, &sin)
		out := api.ConnectorSyncOutput{Items: []api.ConnectorItem{}}
		for _, d := range []struct {
			name string
			gen  bool
		}{{"human", false}, {"generated", true}} {
			if err := os.WriteFile(filepath.Join(sin.Dest, d.name+".md"), []byte("# "+d.name+"\n\nA "+d.name+" page.\n"), 0o644); err != nil {
				return actionsapi.RunOutput{}, err
			}
			it := api.ConnectorItem{Path: d.name + ".md", URI: "gen://" + d.name, Title: d.name}
			if d.gen {
				it.Generated, it.Generator = true, "katyoti"
			}
			out.Items = append(out.Items, it)
		}
		value = out
	default:
		return actionsapi.RunOutput{}, fmt.Errorf("no method %s", in.URI)
	}
	raw, _ := json.Marshal(value)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// browseWorld boots a real CWM with the sources pack, the local-folder
// connector, a browsable wiki connector and a drive connector with no browse.
func browseWorld(t *testing.T) (context.Context, cwmapi.CWMService, api.SourcesService, string, *[]map[string]any) {
	t.Helper()
	root := t.TempDir()
	selections := &[]map[string]any{}
	foreign := loader.NewEmbedFSLoader(fstest.MapFS{
		"connectors/wiki-page.yaml":  {Data: []byte("name: wiki-page\ndescription: A page of the team wiki.\nsync: ktw-wiki:wiki/source-sync\nbrowse: ktw-wiki:wiki/browse\n")},
		"connectors/drive-file.yaml": {Data: []byte("name: drive-file\ndescription: A drive folder.\nsync: ktw-drive:drive/source-sync\n")},
		"connectors/gen-docs.yaml":   {Data: []byte("name: gen-docs\ndescription: Pages some of which a generator marked.\nsync: ktw-wiki:gen/source-sync\n")},
	}, ".")
	ctx, cwm := cwmapp.NewAt(t, root,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-wiki", foreign, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return browseActions{mu: &sync.Mutex{}, selections: selections}, nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc, root, selections
}

func createSource(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, local string, fields map[string]any) string {
	t.Helper()
	obj := cwmapi.NewContainerObject("m", local, api.InformationSourceType, "m", local, "")
	raw, _ := json.Marshal(fields)
	obj.SetFieldsJSON(raw)
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: obj})
	if err != nil {
		t.Fatalf("create %s: %v", local, err)
	}
	return out.Object.ID
}

func anchorsUnder(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, source string) []string {
	t.Helper()
	kids, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: "m", Parent: source, Type: api.SourceDocumentType})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, o := range kids.Objects {
		ids = append(ids, o.ID)
	}
	sort.Strings(ids)
	return ids
}

func keysOf(items []api.BrowseItem) string {
	var keys []string
	for _, it := range items {
		keys = append(keys, it.Key)
	}
	return strings.Join(keys, " ")
}

// Every connector is browsed and synced by picked items through one
// contract: browse lists a level, a selection holds the items picked, a sync
// brings only those and drops what was unpicked since; a connector with no
// browse syncs its whole location; local-folder browses a folder tree.
func TestConnectorBrowse(t *testing.T) {
	ctx, cwm, svc, root, selections := browseWorld(t)
	wiki := createSource(t, ctx, cwm, "wiki", map[string]any{"connector": "wiki-page", "location": map[string]any{"site": "team"}})

	t.Run("browse lists the top level then a parent's children", func(t *testing.T) {
		top, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{ID: wiki})
		if err != nil || keysOf(top.Items) != "space" || !top.Items[0].HasChildren {
			t.Fatalf("top level: %v %+v", err, top)
		}
		kids, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{Connector: "wiki-page", Location: map[string]any{"site": "team"}, Parent: "space"})
		if err != nil || keysOf(kids.Items) != "p1 p2 p3" {
			t.Fatalf("children of space: %v %+v", err, kids)
		}
	})

	folder := func(source string) string {
		got, _ := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: source})
		var f api.InformationSourceFields
		_ = json.Unmarshal(got.Object.FieldsJSON(), &f)
		return filepath.Join(root, filepath.FromSlash(f.Folder))
	}
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }

	t.Run("a selection of two of three is synced alone, and an item unpicked is dropped", func(t *testing.T) {
		sel, err := svc.SourceSelect(ctx, api.SourceSelectInput{ID: wiki, Items: []api.SelectedItem{{Key: "p1", Title: "P1"}, {Key: "p3", Title: "P3"}}})
		if err != nil || !sel.OK || sel.Items != 2 {
			t.Fatalf("select: %v %+v", err, sel)
		}
		got, _ := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: wiki})
		var f api.InformationSourceFields
		_ = json.Unmarshal(got.Object.FieldsJSON(), &f)
		if items := api.SelectedItems(f.Selection); len(items) != 2 || items[0].Key != "p1" || items[1].Key != "p3" || f.Location["site"] != "team" {
			t.Fatalf("the selection is not saved on the source: %+v", f)
		}
		res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: wiki})
		if err != nil || !res.OK || res.Items != 2 {
			t.Fatalf("sync: %v %+v", err, res)
		}
		last := (*selections)[len(*selections)-1]
		if items := api.SelectedItems(last); len(items) != 2 {
			t.Errorf("the connector did not receive the picked items: %+v", last)
		}
		dir := folder(wiki)
		if !exists(filepath.Join(dir, "p1.md")) || !exists(filepath.Join(dir, "p3.md")) || exists(filepath.Join(dir, "p2.md")) {
			t.Errorf("a sync brought more or less than the picked items")
		}
		if a := anchorsUnder(t, ctx, cwm, wiki); len(a) != 2 {
			t.Fatalf("anchors after the first sync: %v", a)
		}

		// p3 unpicked: the next sync drops its file and archives its anchor.
		if _, err := svc.SourceSelect(ctx, api.SourceSelectInput{ID: wiki, Items: []api.SelectedItem{{Key: "p1"}}}); err != nil {
			t.Fatal(err)
		}
		res, err = svc.SourceSync(ctx, api.SourceSyncInput{ID: wiki})
		if err != nil || !res.OK || res.Items != 1 || len(res.Removed) != 1 || !strings.HasSuffix(res.Removed[0], "-p3") {
			t.Fatalf("sync after unpicking p3: %v %+v", err, res)
		}
		if exists(filepath.Join(dir, "p3.md")) || !exists(filepath.Join(dir, "p1.md")) {
			t.Errorf("the unpicked item's file is still there, or the kept one is gone")
		}
		if a := anchorsUnder(t, ctx, cwm, wiki); len(a) != 1 || !strings.HasSuffix(a[0], "-p1") {
			t.Errorf("anchors after unpicking p3: %v", a)
		}
	})

	t.Run("a connector without browse is said so and syncs its whole location", func(t *testing.T) {
		drive := createSource(t, ctx, cwm, "drive", map[string]any{"connector": "drive-file", "location": map[string]any{"folder": "x"}})
		if _, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{ID: drive}); err == nil || !strings.Contains(err.Error(), "cannot be browsed") {
			t.Errorf("browse of a connector with none: %v", err)
		}
		res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: drive})
		if err != nil || !res.OK || res.Items != len(wikiPages) {
			t.Errorf("a connector without browse syncs its whole location: %v %+v", err, res)
		}
	})

	t.Run("local-folder browses a folder tree and syncs the picked items", func(t *testing.T) {
		for name, body := range map[string]string{"docs/a.md": "# A\n", "docs/sub/b.md": "# B\n", "docs/c.md": "# C\n", "readme.md": "# R\n", ".hidden/x.md": "# X\n"} {
			p := filepath.Join(root, "ext", filepath.FromSlash(name))
			_ = os.MkdirAll(filepath.Dir(p), 0o755)
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		local := createSource(t, ctx, cwm, "local", map[string]any{"connector": api.FolderConnector, "location": map[string]any{"path": "ext"}})
		top, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{ID: local})
		if err != nil || keysOf(top.Items) != "docs readme.md" || !top.Items[0].HasChildren || top.Items[0].Kind != "folder" {
			t.Fatalf("local-folder top level: %v %+v", err, top)
		}
		docs, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{ID: local, Parent: "docs"})
		if err != nil || keysOf(docs.Items) != "docs/sub docs/a.md docs/c.md" {
			t.Fatalf("local-folder docs/: %v %+v", err, docs)
		}
		found, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{ID: local, Query: "b."})
		if err != nil || keysOf(found.Items) != "docs/sub/b.md" {
			t.Errorf("local-folder search: %v %+v", err, found)
		}
		if _, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{ID: local, Parent: "../.."}); err == nil {
			t.Errorf("a parent leaving the folder is browsed")
		}
		// docs without its children brings a.md and c.md, not sub/b.md.
		if _, err := svc.SourceSelect(ctx, api.SourceSelectInput{ID: local, Items: []api.SelectedItem{{Key: "docs"}}}); err != nil {
			t.Fatal(err)
		}
		res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: local})
		if err != nil || !res.OK || res.Items != 2 || exists(filepath.Join(folder(local), "docs", "sub", "b.md")) {
			t.Errorf("a picked folder without its children: %v %+v", err, res)
		}
		if _, err := svc.SourceSelect(ctx, api.SourceSelectInput{ID: local, Items: []api.SelectedItem{{Key: "docs", WithChildren: true}}}); err != nil {
			t.Fatal(err)
		}
		if res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: local}); err != nil || !res.OK || res.Items != 3 {
			t.Errorf("a picked folder with its children: %v %+v", err, res)
		}
	})
}

// The Sources tab picks each source's items: the source's
// « Choisir » tab is the generic picker - searched through sources/search,
// browsed through sources/source-browse, applied through sources/source-select
// -, opened from its row in the Sources tab
// the browse marks the saved choice so the tree opens with it ticked; the row
// shows what is picked; sync-all then reads only the picked items.
func TestThePickPanel(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	panels := filepath.Join(filepath.Dir(here), "..", "contrib", "embed", "cwm", "object-panels")
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(panels, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	pick := read("source-pick.yaml")
	// The generic picker bound to the source, its pick tree
	// replaced.
	for _, want := range []string{`kind: "picker"`, `search: "ktw-agents:sources/search"`, `browse: "ktw-agents:sources/source-browse"`, `apply: "ktw-agents:sources/source-select"`, `args: {id: "${id}", source: "${id}"}`, `source: "ktw-agents:sources/source-picked"`, `- "information-source"`, `uri: "ktw-agents:sources/source-sync"`} {
		if !strings.Contains(pick, want) {
			t.Errorf("source-pick.yaml should declare %s", want)
		}
	}
	if tab := read("sources.yaml"); !strings.Contains(tab, `navigate: {object: "${item.id}", panel: "source-pick"}`) || !strings.Contains(tab, `"$.item.picked_label"`) {
		t.Errorf("a source's row in the Sources tab should open its pick tab and show what is picked")
	}

	ctx, cwm, svc, _, selections := browseWorld(t)
	wiki := createSource(t, ctx, cwm, "wiki", map[string]any{"connector": "wiki-page", "location": map[string]any{"site": "team"}})
	if _, err := svc.SourceSelect(ctx, api.SourceSelectInput{ID: wiki, Items: []api.SelectedItem{{Key: "p1", Title: "P1"}, {Key: "p3", Title: "P3"}}}); err != nil {
		t.Fatal(err)
	}
	kids, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{ID: wiki, Parent: "space"})
	if err != nil {
		t.Fatal(err)
	}
	var picked []string
	for _, it := range kids.Items {
		if it.Picked {
			picked = append(picked, it.Key)
		}
	}
	if strings.Join(picked, " ") != "p1 p3" {
		t.Errorf("the browse of a source should mark its saved choice, got %v", picked)
	}
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil || len(of.Sources) != 1 || of.Sources[0].Picked != 2 || !strings.Contains(of.Sources[0].PickedLabel, "P1, P3") {
		t.Fatalf("the source's row should show the two items picked: %v %+v", err, of.Sources)
	}
	res, err := svc.SyncAll(ctx, api.SyncAllInput{ID: "m"})
	if err != nil || len(res.Sources) != 1 {
		t.Fatalf("sync-all: %v %+v", err, res)
	}
	if last := (*selections)[len(*selections)-1]; len(api.SelectedItems(last)) != 2 {
		t.Errorf("sync-all should read only the two picked items: %+v", last)
	}
}
