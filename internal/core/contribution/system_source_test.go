// SPDX-License-Identifier: Apache-2.0

package contribution

import (
	"context"
	"io"
	"io/fs"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/api/providertest"
)

// Every system provider — one per kind directory, as the extension registers
// them — holds the provider contract.
func TestSystemProviders_HoldTheProviderContract(t *testing.T) {
	items, err := loadSystemContributions()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	byType := map[string][]systemContribution{}
	for _, it := range items {
		byType[it.typ] = append(byType[it.typ], it)
	}
	for typ, its := range byType {
		providertest.Run(t, &systemContributionProvider{typ: typ, items: its})
	}
}

// A kind's declaration is the YAML a person wrote, comments included; the
// decoded descriptor is what a consumer reads. The kind `contribution-type`
// declares both layers (`data`, `source: application/yaml`), and until
// 2026-09-22 no kind entry carried the second: the Resources pane showed
// "declared yes, carried no" and "open declaration" showed JSON with the
// reasons stripped. Every embedded kind now carries its file as `source`,
// byte for byte, beside its decoded `data`.
func TestEverySystemKindEntryCarriesItsAuthoredSource(t *testing.T) {
	items, err := loadSystemContributions()
	if err != nil {
		t.Fatalf("loadSystemContributions: %v", err)
	}
	kinds := 0
	for _, item := range items {
		if item.typ != contribapi.ResourceTypeContributionType {
			continue
		}
		kinds++
		e := item.entry()
		if len(e.Data()) == 0 {
			t.Errorf("%s: no data layer — the decoded descriptor is what consumers read", e.URI)
		}
		ref, ok := e.SourceRef()
		if !ok {
			t.Errorf("%s: no source layer — the kind declares one and nobody carried it", e.URI)
			continue
		}
		if ref.MediaType != "application/yaml" {
			t.Errorf("%s: source media type %q, want the kind's application/yaml", e.URI, ref.MediaType)
		}
		file, rerr := fs.ReadFile(systemFS, "embed/contribution-type/"+item.id+".yaml")
		if rerr != nil {
			t.Fatalf("%s: read embedded file: %v", e.URI, rerr)
		}
		if got, want := ref.Hash, contribapi.HashContent(file); got != want {
			t.Errorf("%s: source hash %s, want the file's %s — the declaration must be the file, not a rendering of it", e.URI, got, want)
		}
		// Open answers from the same envelope: the source layer's bytes.
		p := &systemContributionProvider{typ: item.typ, items: []systemContribution{item}}
		rc, oref, oerr := p.Open(context.Background(), item.id, contribapi.LayerSource)
		var body []byte
		if oerr == nil {
			body, _ = io.ReadAll(rc)
			_ = rc.Close()
		}
		if oerr != nil || contribapi.HashContent(body) != ref.Hash || oref.MediaType != "application/yaml" {
			t.Errorf("%s: Open(source) answered %q/%d bytes, err %v — want the source layer", e.URI, oref.MediaType, len(body), oerr)
		}
	}
	if kinds == 0 {
		t.Fatal("no embedded kinds found — the walk matched nothing")
	}
	// A data-type stays source-only: its DSL is its content, there is no
	// descriptor to put beside it.
	for _, item := range items {
		if item.typ == contribapi.ResourceTypeDataType {
			if e := item.entry(); len(e.Data()) != 0 {
				t.Errorf("%s: a data-type grew a data layer", e.URI)
			}
			break
		}
	}
}
