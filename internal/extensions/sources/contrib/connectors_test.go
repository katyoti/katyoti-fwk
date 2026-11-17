// SPDX-License-Identifier: Apache-2.0

package contrib_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

const connectorYAML = `name: example-files
label: Example files
description: Reads the files of an example place.
sync: ktw-example:example/source-sync
`

// A connector ships its connectors as a folder of its pack: the generic pack
// reader, its kinds from the built-in kind reader, publishes each
// source-connectors/<name>.yaml as <provider>:/source-connector/<name> - its
// tree the data layer, the file the source, its name and description the
// meta the builder derives - with no helper of the sources pack.
func TestConnectorsAreReadFromAPack(t *testing.T) {
	kinds, err := builtin.Kinds()
	if err != nil {
		t.Fatal(err)
	}
	pack := fstest.MapFS{
		"embed/source-connectors/example-files.yaml": {Data: []byte(connectorYAML)},
		"embed/source-connectors/notes.txt":          {Data: []byte("not a connector")},
	}
	point := func(init *application.ApplicationInit) {
		init.AddRegisterPoints("test", ".providers-point", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
		})
	}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), point,
		manifestcontrib.ContributePack("ktw-example", loader.NewEmbedFSLoader(pack, "."), "embed", kinds))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	var got []contribapi.Contribution
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			if c.Type == api.ContributionTypeSourceConnector {
				got = append(got, c)
			}
		}
	}
	if len(got) != 1 {
		t.Fatalf("source-connector entries %v, want the one YAML of the folder", got)
	}
	c := got[0]
	if uri := c.ResolvedURI(); uri != "ktw-example:/source-connector/example-files" {
		t.Errorf("uri %q", uri)
	}
	var d api.ConnectorDecl
	if err := json.Unmarshal(c.Data(), &d); err != nil {
		t.Fatalf("data layer %s: %v", c.Data(), err)
	}
	if d.Name != "example-files" || d.Label != "Example files" || d.Sync != "ktw-example:example/source-sync" {
		t.Errorf("data layer %+v", d)
	}
	if !strings.Contains(string(c.Source()), "label: Example files") {
		t.Errorf("source layer %q, want the file as written", c.Source())
	}
	// The manifest builder derives the meta from the data layer, as the
	// kind declares (meta.from): no producer writes it.
	if err := contribapi.ApplyDerivedMeta(&c, kinds); err != nil {
		t.Fatal(err)
	}
	if c.Metadata == nil || c.Metadata.Name != "example-files" || c.Metadata.Description != "Reads the files of an example place." {
		t.Errorf("meta %+v, want the name and description of the data", c.Metadata)
	}
}
