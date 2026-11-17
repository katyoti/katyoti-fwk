// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// The in-hub content lane serves what tier 1 HOLDS and refuses the rest.
//
// A resident layer (a data-type's DSL, kept by the builder because its kind
// declares it resident) is served from the manifest and contacts nobody. A
// bulk-band layer is refused with its address. A non-resident layer is refused
// too — reaching the producer is the CatalogService's job, not this catalog's,
// which is what keeps its answers safe during discovery.
func TestCatalogImplOpenLayer_ServesResidentRefusesTheRest(t *testing.T) {
	dsl := contribapi.Contribution{Provider: "p", Type: contribapi.ResourceTypeDataType, ID: "shape"}
	dsl.SetSource([]byte("$schema: ktw-schema/v1\nname: shape\n"), "application/yaml")

	skill := contribapi.Contribution{Provider: "p", Type: "skill", ID: "s"}
	skill.SetData([]byte(`{"name":"s"}`))
	skill.Layers[contribapi.LayerSource] = contribapi.LayerRef{Hash: "sha256:abc", Size: 7000, MediaType: "text/markdown"}

	big := contribapi.Contribution{Provider: "p", Type: "reference", ID: "dataset"}
	big.SetData([]byte(`{}`))
	big.Layers[contribapi.LayerSource] = contribapi.LayerRef{Hash: "sha256:big", Size: contribapi.BandControl + 1}

	cat := seedCatalog(t, contribapi.ContributionsConfig{}, &validatorStub{}, []contribapi.Contribution{
		kindDecl(contribapi.ResourceTypeDataType, "system:/data-type/data-type"), dsl, skill, big,
	})

	rc, ref, err := cat.OpenLayer(context.Background(), "p:/data-type/shape", contribapi.LayerSource)
	if err != nil {
		t.Fatalf("resident source: %v", err)
	}
	b, _ := io.ReadAll(rc)
	if !strings.HasPrefix(string(b), "$schema") || ref.MediaType != "application/yaml" || ref.Inline != nil {
		t.Errorf("resident source: bytes %q, ref %+v", b, ref)
	}

	rc, _, err = cat.OpenLayer(context.Background(), "p:/skill/s", contribapi.LayerData)
	if err != nil {
		t.Fatalf("resident data: %v", err)
	}
	if b, _ := io.ReadAll(rc); string(b) != `{"name":"s"}` {
		t.Errorf("resident data = %q", b)
	}

	if _, _, err := cat.OpenLayer(context.Background(), "p:/skill/s", contribapi.LayerSource); !errors.As(err, new(contribapi.ErrLayerNotCarried)) {
		t.Errorf("non-resident source must be refused (ErrLayerNotCarried), got %v", err)
	}
	var bulk contribapi.ErrBulkLayer
	if _, _, err := cat.OpenLayer(context.Background(), "p:/reference/dataset", contribapi.LayerSource); !errors.As(err, &bulk) || bulk.Ref.Hash != "sha256:big" {
		t.Errorf("bulk layer must be refused with its address (ErrBulkLayer), got %v", err)
	}
	if _, _, err := cat.OpenLayer(context.Background(), "p:/skill/s", "thumbnail"); err == nil || !strings.Contains(err.Error(), `declares no layer "thumbnail"`) {
		t.Errorf("undeclared layer: %v", err)
	}
}
