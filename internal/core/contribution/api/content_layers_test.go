// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"bytes"
	"testing"
)

// Each layer accessor answers its own layer and nothing else. There is no
// preference baked into the envelope any more: which layer a consumer wants is
// the consumer's question (the body lane asks for `data`, the content lane asks
// for the layer it was given, and the kind names the default). Two accessors
// with opposite fallbacks used to live here — `Content()` source-first and
// `DataOrSource()` data-first — and the second is how YAML reached a JSON
// decoder: a body lane that answered the source layer when there was no body.
func TestLayerAccessorsAnswerTheirOwnLayerOnly(t *testing.T) {
	both := Contribution{Provider: "p", Type: "notation", ID: "uml2"}
	both.SetData([]byte(`{"descriptor":true}`))
	both.SetSource([]byte("the authored file"), "text/plain")

	if got := string(both.Data()); got != `{"descriptor":true}` {
		t.Errorf("Data() = %q", got)
	}
	if got := string(both.Source()); got != "the authored file" {
		t.Errorf("Source() = %q", got)
	}

	// A data-type IS its DSL: source only, no data layer, and Data() says so
	// with nil rather than reaching for the source.
	dsl := Contribution{Provider: "p", Type: ResourceTypeDataType, ID: "notation"}
	dsl.SetSource([]byte("$schema: ktw-schema/v1\nname: notation\n"), "application/yaml")
	if len(dsl.Data()) != 0 {
		t.Error("Data() must be nil for a source-only kind — the body lane has nothing to say")
	}
	if len(dsl.Source()) == 0 {
		t.Error("Source() must carry the DSL")
	}

	bare := Contribution{Provider: "p", Type: "service", ID: "cwm"}
	if len(bare.Data()) != 0 || len(bare.Source()) != 0 {
		t.Error("a contribution with no layers must report no content")
	}
}

// Dropping inline bytes must not change which layer answers, only whether the
// bytes are here — and only on the copy. The index plane drops on every list.
func TestData_AfterDropInline(t *testing.T) {
	c := Contribution{Provider: "p", Type: "task", ID: "t"}
	c.SetData(bytes.Repeat([]byte("z"), int(BandInline)+1))
	hash := c.DataHash()
	cp := c
	cp.DropInline()
	if len(cp.Data()) != 0 {
		t.Error("a dropped copy carries no bytes")
	}
	if cp.DataHash() != hash {
		t.Error("a dropped copy keeps its descriptor")
	}
	if len(c.Data()) == 0 || c.DataHash() != hash {
		t.Error("the source contribution must be untouched by a drop on a copy")
	}
}
