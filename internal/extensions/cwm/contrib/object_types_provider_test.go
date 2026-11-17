// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/api/providertest"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// The PRODUCER link: object-types shipped in a pack's `object-types/` folder
// must come out as catalog contributions of type `object-type`.
//
// Run against the REAL ktw-methods pack, not a fixture. The bug this closes was
// precisely that the files existed and nothing published them, so a fixture
// proving "the provider can publish something" would have stayed green.
func TestObjectTypeProviderEnumeratesShippedPack(t *testing.T) {
	up := filepath.Join("..", "..", "..", "..")
	dir := filepath.Join(up, "app", "ktw-methods", "embed", "cwm")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("embed dir not reachable from test cwd: %s: %v", dir, err)
	}
	kinds, err := contribution.SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	d, _ := kinds.Get("object-type")
	p := manifestcontrib.KindProvider("ktw-methods", loader.NewEmbedFSLoader(os.DirFS(dir), "."), d.SourceFolder(), d)

	got, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	byID := map[string]bool{}
	for _, c := range got {
		if c.Type != "object-type" {
			t.Errorf("contribution %q has type %q, want object-type", c.ID, c.Type)
		}
		byID[c.ID] = true
	}
	// (the project type is CWM's own: TestCWMDeclaresTheProject)
	for _, want := range []string{"feature", "use-case", "documentation"} {
		if !byID[want] {
			t.Errorf("object-type %q not published as a contribution", want)
		}
	}

	// Open must return the same declaration the enumeration advertised — a
	// body-free index entry is resolved through this path, so an unopenable
	// data layer is an empty type at the consumer with no error anywhere.
	rc, ref, err := p.Open(context.Background(), "use-case", contribapi.LayerData)
	if err != nil {
		t.Fatalf("Open(use-case, data): %v", err)
	}
	body, _ := io.ReadAll(rc)
	_ = rc.Close()
	if ref.MediaType != "application/json" {
		t.Errorf("Open returned media type %q, want application/json", ref.MediaType)
	}
	var ot api.ObjectType
	if err := json.Unmarshal(body, &ot); err != nil {
		t.Fatalf("Open body is not an ObjectType: %v", err)
	}
	if ot.Name != "use-case" || len(ot.Capabilities) == 0 {
		t.Errorf("Open body = %+v, want the use-case declaration with its bundle", ot)
	}

	var notFound contribapi.ErrContributionNotFound
	if _, _, err := p.Open(context.Background(), "no-such-type", contribapi.LayerData); !errors.As(err, &notFound) {
		t.Errorf("Open on an unknown object-type: want ErrContributionNotFound, got %v", err)
	}

	// The provider contract, over the real pack.
	providertest.Run(t, p)
}
