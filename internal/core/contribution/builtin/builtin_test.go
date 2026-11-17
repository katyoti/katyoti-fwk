// SPDX-License-Identifier: Apache-2.0

package builtin

import (
	"context"
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin/leaf"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// fakeBody is the test leaf's entry shape.
type fakeBody struct {
	Name string `json:"name"`
}

// fakeLeaf declares the pack kind `skill` as a test pack would, with its own
// validate and a schema version the skills pack does not carry.
func fakeLeaf() leaf.Declaration {
	return leaf.Declaration{
		Kind:   "skill",
		YAML:   []byte("name: skill\ndescription: A test leaf.\nvalidate: error\nsource:\n  folder: skills\n  match: [\"*.md\"]\n"),
		Schema: manifestcontrib.GoTypeOf[fakeBody]("skill", "7.3.0", "Skill", "A test leaf schema."),
	}
}

// The reader is the system set plus the imported leaves: skill, task, persona,
// source-connector and publication-connector come from their packs, and the
// system set declares none.
func TestKinds_SystemSetPlusThePackLeaves(t *testing.T) {
	sys, err := contribution.SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Kinds()
	if err != nil {
		t.Fatal(err)
	}
	if got.Len() != sys.Len()+len(leaves) {
		t.Fatalf("Kinds() has %d kinds, want the system set's %d plus %d leaves", got.Len(), sys.Len(), len(leaves))
	}
	formats := Formats()
	for _, kind := range []string{"skill", "task", "persona", "source-connector", "publication-connector"} {
		if _, ok := sys.Get(kind); ok {
			t.Errorf("the system set still declares %s", kind)
		}
		if got, want := got.DeclaredBy(kind), ReaderProvider+":/contribution-type/"+kind; got != want {
			t.Errorf("%s declared by %q, want %q", kind, got, want)
		}
		if formats[kind] == "" {
			t.Errorf("no format for %s", kind)
		}
	}
	for _, n := range sys.Names() {
		if got.DeclaredBy(n) != sys.DeclaredBy(n) {
			t.Errorf("%s declared by %q, want the system's %q", n, got.DeclaredBy(n), sys.DeclaredBy(n))
		}
	}
}

// Leaves is a copy: a caller cannot change what Kinds() reads.
func TestLeaves_IsACopyOfTheImportedLeaves(t *testing.T) {
	saved := leaves
	defer func() { leaves = saved }()
	leaves = []leaf.Declaration{fakeLeaf()}
	got := Leaves()
	if len(got) != 1 || got[0].Kind != "skill" {
		t.Fatalf("Leaves() = %v, want the one imported leaf", got)
	}
	got[0].Kind = "changed"
	if leaves[0].Kind != "skill" {
		t.Fatal("Leaves() handed out the package's own slice")
	}
	if got := Leaves("skill", "task"); len(got) != 1 || got[0].Kind != "skill" {
		t.Errorf("Leaves(skill, task) = %v, want the skill leaf only", got)
	}
	if got := Leaves("task"); len(got) != 0 {
		t.Errorf("Leaves(task) = %v, want none", got)
	}
}

// A call with no leaf contributes no provider.
func TestContributeLeafKinds_NoLeafPublishesNothing(t *testing.T) {
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)),
		providersPoint, ContributeLeafKinds("ktw-test"))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	if ps := application.UseAppExtensions[contribapi.ContributionProvider](app); len(ps) != 0 {
		t.Errorf("%d providers contributed, want none", len(ps))
	}
}

// A leaf's schema is named under the reader provider, and its format follows.
func TestKinds_LeafIsDeclaredUnderTheReader(t *testing.T) {
	k, err := kindsOf([]leaf.Declaration{fakeLeaf()})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := k.Get("skill")
	if !ok {
		t.Fatal("skill undeclared")
	}
	if d.Validate != "error" {
		t.Errorf("validate = %q, want the leaf's error", d.Validate)
	}
	if got, want := k.DeclaredBy("skill"), ReaderProvider+":/contribution-type/skill"; got != want {
		t.Errorf("declared by %q, want %q", got, want)
	}
	if got, want := k.SchemaFor("skill"), ReaderProvider+":/data-type/skill"; got != want {
		t.Errorf("schema %q, want %q", got, want)
	}
	sys, _ := contribution.SystemKinds()
	if k.Len() != sys.Len()+1 {
		t.Errorf("%d kinds, want the system set's %d plus the leaf", k.Len(), sys.Len())
	}
	if f := formatsOf([]leaf.Declaration{fakeLeaf()})["skill"]; f != "7.3" {
		t.Errorf("skill format %q, want the leaf schema's 7.3", f)
	}
}

// A kind has one home: a leaf naming a system kind is refused.
func TestKinds_LeafNamingASystemKindIsRefused(t *testing.T) {
	l := fakeLeaf()
	l.Kind = "workflow"
	l.YAML = []byte("name: workflow\ndescription: A test leaf.\n")
	if _, err := kindsOf([]leaf.Declaration{l}); err == nil {
		t.Fatal("a leaf declaring the system kind workflow was accepted")
	}
}

func providersPoint(init *application.ApplicationInit) {
	init.AddRegisterPoints("test", ".providers-point", func(app *application.Application) error {
		return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
	})
}

// Called once per root or layer, the helper publishes once: one declaration
// and one data-type under the provider; the system set never carries it.
func TestContributeLeafKinds_PublishesOncePerBinary(t *testing.T) {
	l := fakeLeaf()
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)),
		providersPoint,
		contribution.SystemContributionsExtension(),
		ContributeLeafKinds("ktw-test", l),
		ContributeLeafKinds("ktw-test", l),
		ContributeLeafKinds("ktw-test", l),
	)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			count[c.ResolvedURI()]++
		}
	}
	for uri, want := range map[string]int{
		"ktw-test:/contribution-type/skill":  1,
		"ktw-test:/data-type/skill":          1,
		"system:/contribution-type/skill":    0,
		"system:/data-type/skill":            0,
		"system:/contribution-type/workflow": 1, // the system kinds stay
	} {
		if count[uri] != want {
			t.Errorf("%s published %d times, want %d", uri, count[uri], want)
		}
	}
}

// A binary without the helper declares none of the pack kinds.
func TestSystemContributions_CarryNoPackKind(t *testing.T) {
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)),
		providersPoint, contribution.SystemContributionsExtension())
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		cs, _ := p.Enumerate(context.Background())
		for _, c := range cs {
			for _, kind := range []string{"skill", "task", "persona", "source-connector", "publication-connector"} {
				if c.ResolvedURI() == "system:/contribution-type/"+kind || c.ResolvedURI() == "system:/data-type/"+kind {
					t.Errorf("%s published by the system set", c.ResolvedURI())
				}
			}
		}
	}
}

// Every binary knows the drawio kinds from their leaf: Kinds() declares
// notation, viewpoint and drawio-template under the reader, each with its
// pack folder and its leaf schema, and a pack holding those folders is read
// as them, the .drawio beside a library folded into its data as `drawio`.
func TestBuiltinKnowsTheDrawioKinds(t *testing.T) {
	ks, err := Kinds()
	if err != nil {
		t.Fatal(err)
	}
	folders := map[string]string{"notation": "notations", "viewpoint": "viewpoints", "drawio-template": "drawio-templates"}
	for kind, folder := range folders {
		d, ok := ks.Get(kind)
		if !ok {
			t.Errorf("Kinds() does not declare %s", kind)
			continue
		}
		if got, want := ks.DeclaredBy(kind), ReaderProvider+":/contribution-type/"+kind; got != want {
			t.Errorf("%s declared by %q, want %q", kind, got, want)
		}
		if d.Source == nil || d.SourceFolder() != folder {
			t.Errorf("%s: source folder %q, want %q", kind, d.SourceFolder(), folder)
		}
		if got, want := d.Schema, ReaderProvider+":/data-type/"+kind; got != want {
			t.Errorf("%s: schema %q, want its leaf's %q", kind, got, want)
		}
		if Formats()[kind] == "" {
			t.Errorf("no format for %s", kind)
		}
	}

	fsys := fstest.MapFS{
		"pack/notations/flow.yaml":          {Data: []byte("name: flow\ntitle: Flow\n")},
		"pack/viewpoints/flow-map.yaml":     {Data: []byte("name: flow-map\ntitle: Flow map\nnotation: flow\n")},
		"pack/drawio-templates/flow.yaml":   {Data: []byte("name: flow\nnotation: flow\n")},
		"pack/drawio-templates/flow.drawio": {Data: []byte("<mxfile/>")},
	}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), providersPoint,
		manifestcontrib.ContributePack("ktw-test", loader.NewEmbedFSLoader(fsys, "."), "pack", ks))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	got := map[string]contribapi.Contribution{}
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			got[c.ResolvedURI()] = c
		}
	}
	for _, uri := range []string{"ktw-test:/notation/flow", "ktw-test:/viewpoint/flow-map", "ktw-test:/drawio-template/flow"} {
		if _, ok := got[uri]; !ok {
			t.Errorf("%s: not read from the pack (got %d entries)", uri, len(got))
		}
	}
	if lib, ok := got["ktw-test:/drawio-template/flow"]; ok {
		var m map[string]any
		if err := json.Unmarshal(lib.Data(), &m); err != nil {
			t.Fatal(err)
		}
		if m["drawio"] != "<mxfile/>" {
			t.Errorf("the .drawio beside the library is not folded into its data: %v", m)
		}
	}
}

// Every binary knows the slide-deck kinds from their leaf: Kinds() declares
// deck-theme, deck-asset and slide-group under the reader, each with its pack
// folder and its leaf schema, and a pack holding those folders is read as
// them: a theme file's body is its path under deck-assets (the theme its
// first segment), and a theme's picker fields come from its data.
func TestBuiltinKnowsTheDeckKinds(t *testing.T) {
	ks, err := Kinds()
	if err != nil {
		t.Fatal(err)
	}
	folders := map[string]string{"deck-theme": "deck-themes", "deck-asset": "deck-assets", "slide-group": "slide-groups"}
	for kind, folder := range folders {
		d, ok := ks.Get(kind)
		if !ok {
			t.Errorf("Kinds() does not declare %s", kind)
			continue
		}
		if got, want := ks.DeclaredBy(kind), ReaderProvider+":/contribution-type/"+kind; got != want {
			t.Errorf("%s declared by %q, want %q", kind, got, want)
		}
		if d.Source == nil || d.SourceFolder() != folder {
			t.Errorf("%s: source folder %q, want %q", kind, d.SourceFolder(), folder)
		}
		if got, want := d.Schema, ReaderProvider+":/data-type/"+kind; got != want {
			t.Errorf("%s: schema %q, want its leaf's %q", kind, got, want)
		}
		if Formats()[kind] == "" {
			t.Errorf("no format for %s", kind)
		}
	}

	fsys := fstest.MapFS{
		"pack/deck-themes/brand.yaml":     {Data: []byte("name: brand\ndescription: A brand.\nchrome:\n  logo: {asset: logo.png, box: {x: 1, y: 1, w: 10, h: 5}}\n")},
		"pack/deck-assets/brand/logo.png": {Data: []byte("png")},
		"pack/slide-groups/our-team.yaml": {Data: []byte("name: our-team\ndescription: Who we are.\nslides: |\n  # Our team\n")},
	}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), providersPoint,
		manifestcontrib.ContributePack("ktw-test", loader.NewEmbedFSLoader(fsys, "."), "pack", ks))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	got := map[string]contribapi.Contribution{}
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			got[c.ResolvedURI()] = c
		}
	}
	for _, uri := range []string{"ktw-test:/deck-theme/brand", "ktw-test:/deck-asset/brand/logo.png", "ktw-test:/slide-group/our-team"} {
		if _, ok := got[uri]; !ok {
			t.Errorf("%s: not read from the pack (got %d entries)", uri, len(got))
		}
	}
	if a, ok := got["ktw-test:/deck-asset/brand/logo.png"]; ok {
		var m map[string]any
		if err := json.Unmarshal(a.Data(), &m); err != nil {
			t.Fatal(err)
		}
		if m["path"] != "brand/logo.png" || len(m) != 1 {
			t.Errorf("a theme file's body is its path under deck-assets: %v", m)
		}
		if string(a.Source()) != "png" {
			t.Errorf("a theme file's source is the file: %q", a.Source())
		}
	}
	if th, ok := got["ktw-test:/deck-theme/brand"]; ok {
		if err := contribapi.ApplyDerivedMeta(&th, ks); err != nil {
			t.Fatal(err)
		}
		if m := th.ResolvedMetadata(); m.Name != "brand" || m.Description != "A brand." {
			t.Errorf("a theme's picker fields come from its data: %q/%q", m.Name, m.Description)
		}
	}
}
