// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// The body lane answers the DATA layer and nothing else.
//
// A data-type IS its DSL: it carries a source layer and no data layer. Asking
// its body must return nothing — not the YAML. Every catalog used to fall back
// to the source layer here, and the typed lane then JSON-decoded YAML: every
// data-type in the GUI read "invalid character '$'". One question, one answer.
func sourceOnlyEntry() contribapi.Contribution {
	c := contribapi.Contribution{Provider: "p", Type: contribapi.ResourceTypeDataType, ID: "shape"}
	c.SetSource([]byte("$schema: ktw-schema/v1\nname: shape\n"), "application/yaml")
	return c
}

func TestGetBodyNeverAnswersSource_MemoryCatalog(t *testing.T) {
	cat := NewMemoryCatalog([]*mbapi.Manifest{{Provider: "p", Contributions: []contribapi.Contribution{sourceOnlyEntry()}}}, []string{"p"})
	body, err := cat.GetBody(context.Background(), "p:/data-type/shape")
	if err != nil {
		t.Fatalf("GetBody: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("the body lane answered the SOURCE layer: %q", body)
	}
}

func TestGetBodyNeverAnswersSource_CatalogImpl(t *testing.T) {
	cat := seedCatalog(t, contribapi.ContributionsConfig{}, &validatorStub{}, []contribapi.Contribution{sourceOnlyEntry()})
	body, err := cat.GetBody(context.Background(), "p:/data-type/shape")
	if err != nil {
		t.Fatalf("GetBody: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("the body lane answered the SOURCE layer: %q", body)
	}
}
