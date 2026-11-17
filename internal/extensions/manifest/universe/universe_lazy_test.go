// SPDX-License-Identifier: Apache-2.0

package universe

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcore "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/internal/core"
)

// The lazy view is what every agent process holds as its universe catalog.
// The universe-reach content lane requires a LayerOpener, and this wrapper
// did not forward it: every `skills-get-instruction` from an agent failed
// with "this aggregate cannot open layers" from d1 (2026-09-21) until e9.
func TestLazyCatalog_OpensLayersThroughTheBuiltView(t *testing.T) {
	src := []byte("name: shape\n")
	dt := contribapi.Contribution{Provider: "p", Type: "data-type", ID: "shape"}
	dt.SetSource(src, "application/yaml")
	lazy := newLazyCatalog(func(context.Context) (mbapi.Catalog, error) {
		return manifestcore.NewMemoryCatalog([]*mbapi.Manifest{{Provider: "p", Contributions: []contribapi.Contribution{dt}}}, nil), nil
	})
	var _ mbapi.LayerOpener = lazy

	rc, ref, err := lazy.OpenLayer(context.Background(), "p:/data-type/shape", contribapi.LayerSource)
	if err != nil {
		t.Fatalf("OpenLayer through the lazy view: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != string(src) || ref.Hash != contribapi.HashContent(src) {
		t.Fatalf("got %q ref %+v", got, ref)
	}

	// A view that failed to build refuses with the build error, not a panic.
	broken := newLazyCatalog(func(context.Context) (mbapi.Catalog, error) { return nil, errors.New("hub unreachable") })
	if _, _, err := broken.OpenLayer(context.Background(), "p:/data-type/shape", contribapi.LayerSource); err == nil || !strings.Contains(err.Error(), "hub unreachable") {
		t.Fatalf("unbuilt view: err = %v, want the build error", err)
	}
}
