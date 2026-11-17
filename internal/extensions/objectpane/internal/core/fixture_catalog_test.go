// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	mbcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// layerCatalog is an in-memory universe catalog whose layers are opened by
// the provider that published the entries, as the hub's catalog does.
type layerCatalog struct {
	*drawiotesting.Catalog
	provider *mbcontrib.EmbedFSResourceProvider
}

func (c layerCatalog) OpenLayer(ctx context.Context, uri, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	u, err := contribapi.ParseResourceURI(uri)
	if err != nil || u.Provider != c.provider.ProviderName || u.Type != c.provider.ResourceType {
		return nil, contribapi.LayerRef{}, fmt.Errorf("no layer %s", uri)
	}
	return c.provider.Open(ctx, u.ID, layer)
}

// THE FIXTURE SOURCE THROUGH THE CATALOG: the reference files an extension
// ships under embed/references/ (published by the reference provider, as
// ContributeReferenceResources installs it) are found among the universe's
// references and read from their source layer; a file no reference names is
// never opened, whatever its path.
func TestTheFixtureSourceReadsAShippedReferenceThroughTheCatalog(t *testing.T) {
	fs := fstest.MapFS{
		"embed/references/mock/demo/board.default.json": {Data: []byte(`{"rows":[{"name":"default"}]}`)},
		"embed/references/mock/demo/board.full.json":    {Data: []byte(`{"rows":[{"name":"one"},{"name":"two"}]}`)},
		"embed/references/secret.default.json":          {Data: []byte(`{"secret":true}`)},
	}
	p := &mbcontrib.EmbedFSResourceProvider{
		ProviderName:        "acme",
		ResourceType:        mbapi.ResourceTypeReference,
		Loader:              loader.NewEmbedFSLoader(fs, ""),
		Root:                "embed/references",
		KeepFolderStructure: true,
		KeepExtension:       true,
		BodyDecoder: func(itemPath string, _ []byte) (json.RawMessage, error) {
			return json.Marshal(mbapi.ReferenceBody{Path: itemPath})
		},
	}
	entries, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cat := layerCatalog{Catalog: drawiotesting.NewCatalog(map[string][]contribapi.Contribution{"acme": entries}), provider: p}
	app := application.New(application.Config())
	application.PutService[mbapi.Catalog](app, "default", cat)
	ctx := application.WithApplication(context.Background(), app)

	svc := &Service{}
	got, err := svc.Fixture(ctx, api.FixtureInput{Fixture: "acme:/reference/mock/demo/board", State: "full"})
	if err != nil || string(got) != `{"rows":[{"name":"one"},{"name":"two"}]}` {
		t.Fatalf("state full: got %q, %v", got, err)
	}
	if got, err := svc.Fixture(ctx, api.FixtureInput{Fixture: "acme:/reference/mock/demo/board"}); err != nil || string(got) != `{"rows":[{"name":"default"}]}` {
		t.Fatalf("default: got %q, %v", got, err)
	}
	for _, in := range []api.FixtureInput{
		{Fixture: "acme:/reference/mock/demo/board", State: "gone"},
		{Fixture: "acme:/reference/mock/../secret", State: "default"},
		{Fixture: "acme:/reference/mock/demo/../../secret", State: "default"},
	} {
		if got, err := svc.Fixture(ctx, in); err != nil || got != nil {
			t.Errorf("%+v: got %q, %v; want nothing", in, got, err)
		}
	}
}
