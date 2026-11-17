// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/datatypegen"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	coreschemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/publications/api"
	publicationscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/publications/contrib"
	publicationslocal "github.com/katyoti/katyoti-fwk/internal/extensions/publications/local"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	schemalocal "github.com/katyoti/katyoti-fwk/internal/extensions/schema/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// wikiSettings are the settings of a « wiki » publisher's instance: where its
// pages go. The publisher declares them as a data type rendered from this Go
// type, and reads them back with api.DecodeSettings.
type wikiSettings struct {
	Space  string `json:"space"            jsonschema:"description=The wiki space"`
	Parent string `json:"parent,omitempty" jsonschema:"description=The page the tree goes under"`
}

// folderSettings are the settings of a second publisher, a folder of files.
type folderSettings struct {
	Dir string `json:"dir" jsonschema:"description=The output folder"`
}

// fakeConnectors are two publishers no part of the publications extension
// names: their publish methods, served in the test as the bus serves a
// sidecar's in a binary. Each counts its calls.
type fakeConnectors struct {
	calls map[string]int
	last  map[string]api.PublishInput
}

func (f *fakeConnectors) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (f *fakeConnectors) Run(_ context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	f.calls[in.URI]++
	var pin api.PublishInput
	if err := json.Unmarshal(in.Input, &pin); err != nil {
		return actionsapi.RunOutput{}, err
	}
	f.last[in.URI] = pin
	var out api.PublishOutput
	switch in.URI {
	case "ktw-wiki:wiki/publish":
		s, err := api.DecodeSettings[wikiSettings](pin.Settings)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		// The state is the wiki's own: the page id of each node, kept apart
		// from the content.
		pages, _ := pin.State["pages"].(map[string]any)
		next := map[string]any{}
		for _, n := range pin.Tree.Documents() {
			o := api.Outcome{Key: n.Key, Title: n.Title}
			if id, ok := pages[n.Key].(string); ok {
				o.URL = "https://wiki.example/" + s.Space + "/" + id
				out.Updated = append(out.Updated, o)
				next[n.Key] = id
				continue
			}
			id := fmt.Sprintf("%s-%d", s.Space, len(next)+1)
			o.URL = "https://wiki.example/" + s.Space + "/" + id
			out.Created = append(out.Created, o)
			next[n.Key] = id
		}
		out.State = map[string]any{"pages": next, "parent": s.Parent}
	case "ktw-folder:folder/publish":
		s, err := api.DecodeSettings[folderSettings](pin.Settings)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		for _, n := range pin.Tree.Documents() {
			out.Created = append(out.Created, api.Outcome{Key: n.Key, Title: n.Title, URL: "file://" + s.Dir + "/" + n.Key + ".md"})
		}
		if !pin.Preview {
			out.State = map[string]any{"written": len(out.Created)}
		}
	case "ktw-wiki:wiki/hosts":
		// The wiki's target lives on its own host.
		raw, _ := json.Marshal(api.HostsOutput{Hosts: []string{"wiki.example"}})
		return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
	default:
		return actionsapi.RunOutput{}, fmt.Errorf("no method %s", in.URI)
	}
	raw, _ := json.Marshal(out)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// publishersWorld boots the publications extension with two publishers two
// other members ship in their packs (a publication-connectors folder), a
// SchemaService knowing their settings' data types, and an ActionService
// serving their publish methods.
func publishersWorld(t *testing.T, more ...application.Extension) (context.Context, api.PublishersService, *fakeConnectors) {
	t.Helper()
	wiki := loader.NewEmbedFSLoader(fstest.MapFS{
		"embed/publication-connectors/wiki.yaml": {Data: []byte("name: wiki\nlabel: Wiki\ndescription: The pages of the team wiki.\nsettings: ktw-wiki:/data-type/wiki-settings\npublish: ktw-wiki:wiki/publish\nhosts: ktw-wiki:wiki/hosts\n")},
	}, ".")
	folder := loader.NewEmbedFSLoader(fstest.MapFS{
		"embed/publication-connectors/folder.yaml": {Data: []byte("name: folder\ndescription: A folder of markdown files.\nsettings: ktw-folder:/data-type/folder-settings\npublish: ktw-folder:folder/publish\n")},
	}, ".")
	fake := &fakeConnectors{calls: map[string]int{}, last: map[string]api.PublishInput{}}
	ctx, _ := cwmapp.New(t, append([]application.Extension{
		publicationscontrib.DeclareKinds("ktw-agents"),
		publicationscontrib.Pack("ktw-agents"),
		publisherPack(t, "ktw-wiki", wiki),
		publisherPack(t, "ktw-folder", folder),
		schemalocal.Extension(),
		publicationslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return fake, nil
		}),
	}, more...)...)
	schemas, err := application.UseService[schemaapi.SchemaService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	register := func(uri, name, root string, ast func() (*coreschemaapi.Type, error)) {
		body, err := datatypegen.RenderAs(datatypegen.Source{Name: name, Version: "1.0.0", Root: root, AST: ast}, datatypegen.Header{URI: uri})
		if err != nil {
			t.Fatal(err)
		}
		if err := schemas.RegisterDataType(ctx, schemaapi.RegisterDataTypeInput{URI: uri, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	register("ktw-wiki:/data-type/wiki-settings", "wiki-settings", "WikiSettings", func() (*coreschemaapi.Type, error) { return ktwreflect.TypeAST[wikiSettings]() })
	register("ktw-folder:/data-type/folder-settings", "folder-settings", "FolderSettings", func() (*coreschemaapi.Type, error) { return ktwreflect.TypeAST[folderSettings]() })
	svc, err := application.UseService[api.PublishersService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, svc, fake
}

// publisherPack reads a made-up member's pack (its embed folder) the way the
// sidecar kit does: the generic pack reader, its kinds from the built-in kind
// reader, publishes each publication-connectors/<name>.yaml - no helper of the
// publications pack.
func publisherPack(t *testing.T, provider string, ldr loader.Loader) application.Extension {
	t.Helper()
	kinds, err := builtin.Kinds()
	if err != nil {
		t.Fatal(err)
	}
	return manifestcontrib.ContributePack(provider, ldr, "embed", kinds)
}

func tree() api.DocumentTree {
	return api.DocumentTree{
		Meta: api.TreeMeta{Name: "Guides", Product: "KaTyoti", Version: "2026.10"},
		Nodes: []api.Node{
			{Key: "intro", Title: "Introduction", Document: &api.Document{Markdown: "# Introduction\n", Language: "en", Source: "guides/intro.md", Version: "h1"}},
			{Key: "concepts", Title: "Concepts", Children: []api.Node{
				{Key: "modules", Title: "Modules", Document: &api.Document{Markdown: "# Modules\n", Language: "en",
					Assets: []api.Asset{{Name: "modules.svg", MediaType: "image/svg+xml", Content: []byte("<svg/>")}}}},
			}},
		},
	}
}

// A publisher is a publication-connector contribution: its settings decoded
// from the data type it declares, it publishes a tree and answers what it
// created and updated and the state it keeps; given that state back, it
// updates what it created.
func TestAConnectorPublishesATreeThroughTheAPI(t *testing.T) {
	ctx, svc, fake := publishersWorld(t)
	settings := map[string]any{"space": "TEW", "parent": "Home"}
	first, err := svc.Publish(ctx, api.PublishRequest{Connector: "wiki", PublishInput: api.PublishInput{Receipt: rcpt(), Tree: tree(), Settings: settings}})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Created) != 2 || len(first.Updated) != 0 || first.Created[0].Key != "intro" || first.Created[1].Key != "modules" {
		t.Fatalf("the first publish does not create the tree's two documents in order: %+v", first)
	}
	if first.State["parent"] != "Home" {
		t.Errorf("the settings did not reach the connector decoded: state %+v", first.State)
	}
	again, err := svc.Publish(ctx, api.PublishRequest{Connector: "wiki", PublishInput: api.PublishInput{Receipt: rcpt(), Tree: tree(), Settings: settings, State: first.State}})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Created) != 0 || len(again.Updated) != 2 || again.Updated[0].URL != first.Created[0].URL {
		t.Errorf("given its state back, the connector does not update the pages it created: %+v", again)
	}
	// The kind is declared once, by the publications extension, with its
	// schema and as executable.
	kinds, err := mbapi.KindsAt(ctx, mbapi.Q{Reach: mbapi.ReachSelf})
	if err != nil {
		t.Fatal(err)
	}
	if decl, ok := kinds.Get(api.ContributionTypePublicationConnector); !ok || decl.Schema != "ktw-agents:/data-type/publication-connector" {
		t.Errorf("the publication-connector kind is not declared with its schema: %+v %v", decl, ok)
	}
	if fake.calls["ktw-wiki:wiki/publish"] != 2 {
		t.Errorf("the wiki's publish ran %d times, want 2", fake.calls["ktw-wiki:wiki/publish"])
	}
}

// Settings that do not match the data type the connector declares are
// refused before the connector runs; so is a tree with a key used twice.
func TestSettingsAreCheckedAgainstTheDeclaredDataType(t *testing.T) {
	ctx, svc, fake := publishersWorld(t)
	_, err := svc.Publish(ctx, api.PublishRequest{Connector: "wiki", PublishInput: api.PublishInput{Receipt: rcpt(), Tree: tree(), Settings: map[string]any{"parent": "Home"}}})
	if !errors.Is(err, api.ErrInvalidSettings) {
		t.Fatalf("settings missing the wiki's required space are not refused as invalid settings: %v", err)
	}
	bad := tree()
	bad.Nodes = append(bad.Nodes, api.Node{Key: "intro", Title: "Again", Document: &api.Document{Markdown: "x"}})
	if _, err := svc.Publish(ctx, api.PublishRequest{Connector: "wiki", PublishInput: api.PublishInput{Receipt: rcpt(), Tree: bad, Settings: map[string]any{"space": "TEW"}}}); err == nil {
		t.Fatal("a tree with a key used twice is published")
	}
	if n := fake.calls["ktw-wiki:wiki/publish"]; n != 0 {
		t.Errorf("the connector ran %d times on refused input", n)
	}
	if _, err := svc.Publish(ctx, api.PublishRequest{Connector: "nowhere", PublishInput: api.PublishInput{Receipt: rcpt(), Tree: tree()}}); err == nil {
		t.Error("a connector no member contributes publishes")
	}
}

// A second publisher works through the same API and the same callers: nothing
// in the publications extension names either.
func TestASecondConnectorNeedsNoChange(t *testing.T) {
	ctx, svc, _ := publishersWorld(t)
	list, err := svc.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]api.ConnectorDecl{}
	for _, c := range list.Connectors {
		got[c.Name] = c
	}
	if got["wiki"].Publish != "ktw-wiki:wiki/publish" || got["folder"].Settings != "ktw-folder:/data-type/folder-settings" {
		t.Fatalf("the two contributed publishers are not listed with their methods and settings: %+v", list.Connectors)
	}
	out, err := svc.Publish(ctx, api.PublishRequest{Connector: "folder", PublishInput: api.PublishInput{Receipt: rcpt(), Tree: tree(), Settings: map[string]any{"dir": "/out"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Created) != 2 || out.Created[1].URL != "file:///out/modules.md" || out.State["written"] != float64(2) {
		t.Errorf("the second publisher does not publish the same tree through the same API: %+v", out)
	}
	if _, err := svc.Publish(ctx, api.PublishRequest{Connector: "folder", PublishInput: api.PublishInput{Receipt: rcpt(), Tree: tree(), Settings: map[string]any{"space": "TEW"}}}); !errors.Is(err, api.ErrInvalidSettings) {
		t.Errorf("the wiki's settings are taken by the folder publisher: %v", err)
	}
}

// The tree is checked before any connector runs: a node with no key or a
// document holding children is refused like a key used twice.
func TestATreeIsCheckedBeforeAnyConnectorRuns(t *testing.T) {
	ctx, svc, fake := publishersWorld(t)
	settings := map[string]any{"space": "TEW"}
	noKey := tree()
	noKey.Nodes = append(noKey.Nodes, api.Node{Title: "Nameless", Document: &api.Document{Markdown: "x"}})
	docWithChildren := tree()
	docWithChildren.Nodes[0].Children = []api.Node{{Key: "child", Title: "Child", Document: &api.Document{Markdown: "x"}}}
	for name, bad := range map[string]api.DocumentTree{"a node with no key": noKey, "a document holding children": docWithChildren} {
		if _, err := svc.Publish(ctx, api.PublishRequest{Connector: "wiki", PublishInput: api.PublishInput{Receipt: rcpt(), Tree: bad, Settings: settings}}); err == nil {
			t.Errorf("a tree with %s is published", name)
		}
	}
	if n := fake.calls["ktw-wiki:wiki/publish"]; n != 0 {
		t.Errorf("the connector ran %d times on a refused tree", n)
	}
}

// Preview and the confirmed overwrites reach the connector as given; a
// preview whose connector answers no state keeps the state it was given.
func TestPreviewAndOverwriteReachTheConnector(t *testing.T) {
	ctx, svc, fake := publishersWorld(t)
	kept := map[string]any{"written": float64(1)}
	out, err := svc.Publish(ctx, api.PublishRequest{Connector: "folder", PublishInput: api.PublishInput{Receipt: rcpt(), Tree: tree(), Settings: map[string]any{"dir": "/out"}, State: kept, Preview: true, Overwrite: []string{"intro"}}})
	if err != nil {
		t.Fatal(err)
	}
	seen := fake.last["ktw-folder:folder/publish"]
	if !seen.Preview || len(seen.Overwrite) != 1 || seen.Overwrite[0] != "intro" {
		t.Errorf("preview and the confirmed overwrites do not reach the connector: %+v", seen)
	}
	if out.State["written"] != float64(1) {
		t.Errorf("a preview does not keep the state it was given: %+v", out.State)
	}
}

// rcpt is a run's receipt naming one target.
func rcpt() *api.Receipt {
	r := api.NewReceipt("run-1", "tester", "wiki", map[string]any{"space": "TEW", "parent": "Home"})
	return &r
}

// TestPublishReceiptTargetsAreRequired: a publish that
// writes carries a receipt naming its targets - with none, or one naming no
// target, it is refused and the connector never runs; a preview needs none.
func TestPublishReceiptTargetsAreRequired(t *testing.T) {
	ctx, svc, fake := publishersWorld(t)
	settings := map[string]any{"space": "TEW", "parent": "Home"}
	for _, r := range []*api.Receipt{nil, {Run: "run-1", Actor: "tester"}} {
		_, err := svc.Publish(ctx, api.PublishRequest{Connector: "wiki", PublishInput: api.PublishInput{Receipt: r, Tree: tree(), Settings: settings}})
		if !errors.Is(err, api.ErrNoReceipt) {
			t.Errorf("receipt %+v: got %v, want ErrNoReceipt", r, err)
		}
	}
	if fake.calls["ktw-wiki:wiki/publish"] != 0 {
		t.Fatalf("the connector ran without a receipt")
	}
	if _, err := svc.Publish(ctx, api.PublishRequest{Connector: "wiki", PublishInput: api.PublishInput{Tree: tree(), Settings: settings, Preview: true}}); err != nil {
		t.Fatalf("a preview needs no receipt: %v", err)
	}
	if got := api.NewReceipt("r", "a", "wiki", settings); len(got.Targets) != 1 || got.Targets[0].Space != "TEW" || got.Targets[0].Parent != "Home" || got.Targets[0].Connection != "wiki" {
		t.Fatalf("NewReceipt: %+v", got)
	}
}
