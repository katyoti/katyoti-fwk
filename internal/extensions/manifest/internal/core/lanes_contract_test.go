// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/internal/core"
)

// lanes_contract_test.go — the lanes of modules/contributions/contract-matrix.md,
// each asked the question it answers and the one it must refuse (e14).
//
// Query is the index; GetBody is the data layer ONLY; Open(uri, layer) is any
// named layer, and the same consumer call serves all three size bands.

// sourceOnlyDataType is a data-type as its producer publishes it: the DSL is
// its source layer and it has no data layer.
func sourceOnlyDataType() contribapi.Contribution {
	c := contribapi.Contribution{Provider: "ktw-docs", Type: contribapi.ResourceTypeDataType, ID: "shape"}
	c.SetSource([]byte("$schema: ktw-schema/v1\nname: shape\n"), "application/yaml")
	return c
}

// GetBody on a source-only kind answers nothing and is not an error — at the
// catalog and through the consumer calls that use it (GetOne, QueryTyped).
func TestLanes_GetBodyOnSourceOnlyKindIsEmptyNotAnError(t *testing.T) {
	ctx := context.Background()
	cat := core.NewMemoryCatalog([]*mbapi.Manifest{{Provider: "ktw-docs", Contributions: []contribapi.Contribution{sourceOnlyDataType()}}}, nil)
	uri := "ktw-docs:/data-type/shape"

	body, err := cat.GetBody(ctx, uri)
	if err != nil {
		t.Fatalf("GetBody on a source-only kind: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("GetBody answered the source layer: %q", body)
	}

	q := mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}
	one, err := mbapi.GetOne[json.RawMessage](ctx, q, uri)
	if err != nil {
		t.Fatalf("GetOne on a source-only kind: %v", err)
	}
	if len(one.Body) != 0 {
		t.Fatalf("GetOne decoded a body out of the source layer: %q", one.Body)
	}
	q.Kind = contribapi.ResourceTypeDataType
	typed, err := mbapi.QueryTyped[json.RawMessage](ctx, q)
	if err != nil || len(typed) != 1 || len(typed[0].Body) != 0 {
		t.Fatalf("QueryTyped on a source-only kind: %v, %+v", err, typed)
	}

	// The layer the content IS in is reached by naming it.
	l, err := mbapi.Open(ctx, q, uri, contribapi.LayerSource)
	if err != nil {
		t.Fatalf("Open source: %v", err)
	}
	got, _ := l.Bytes()
	if !bytes.Contains(got, []byte("name: shape")) {
		t.Fatalf("Open source returned %q", got)
	}
}

// Open of a layer the entry does not declare is contribapi.ErrLayerNotDeclared
// at universe reach — for every layer name, data and source included, and
// never another layer under that name. Until e16 the MemoryCatalog answered an
// undeclared data/source layer through GetBody: a skill's data came back as its
// "source", and a data-type's (empty) body as its "data".
func TestLanes_OpenUndeclaredLayerIsAnError(t *testing.T) {
	ctx := context.Background()
	skill := contribapi.Contribution{Provider: "ktw-docs", Type: "skill", ID: "doc-create"}
	skill.SetData([]byte(`{"name":"doc-create"}`))
	cases := []struct {
		entry contribapi.Contribution
		uri   string
		layer string
	}{
		{skill, "ktw-docs:/skill/doc-create", "thumbnail"},
		{skill, "ktw-docs:/skill/doc-create", contribapi.LayerSource}, // data-only entry
		{sourceOnlyDataType(), "ktw-docs:/data-type/shape", contribapi.LayerData},
	}
	for _, tc := range cases {
		mem := core.NewMemoryCatalog([]*mbapi.Manifest{{Provider: "ktw-docs", Contributions: []contribapi.Contribution{tc.entry}}}, nil)
		// A body lane that answers anything: if the lane fell back to it, the
		// open would succeed.
		mem.SetContentLane(core.NewContentStore(t.TempDir()), func(context.Context, string, string) ([]byte, error) {
			return []byte("body lane"), nil
		})

		l, err := mbapi.Open(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: mem}, tc.uri, tc.layer)
		if err == nil {
			b, _ := l.Bytes()
			t.Fatalf("%s: mbapi.Open opened undeclared layer %q: %q", tc.uri, tc.layer, b)
		}
		var undeclared contribapi.ErrLayerNotDeclared
		if !errors.As(err, &undeclared) || undeclared.Layer != tc.layer || undeclared.URI != tc.uri {
			t.Fatalf("%s layer %q: mbapi.Open err = %v, want contribapi.ErrLayerNotDeclared naming it", tc.uri, tc.layer, err)
		}
		if _, _, err := mem.OpenLayer(ctx, tc.uri, tc.layer); !errors.As(err, new(contribapi.ErrLayerNotDeclared)) {
			t.Fatalf("%s layer %q: MemoryCatalog.OpenLayer err = %v, want contribapi.ErrLayerNotDeclared", tc.uri, tc.layer, err)
		}
	}
}

// GetBody stays the data lane only while Open refuses: the data-only skill
// still answers its body through GetBody after Open(source) refused.
func TestLanes_GetBodyIsTheDataLaneOnly(t *testing.T) {
	ctx := context.Background()
	skill := contribapi.Contribution{Provider: "ktw-docs", Type: "skill", ID: "doc-create"}
	skill.SetData([]byte(`{"name":"doc-create"}`))
	mem := core.NewMemoryCatalog([]*mbapi.Manifest{{Provider: "ktw-docs", Contributions: []contribapi.Contribution{skill}}}, nil)
	uri := "ktw-docs:/skill/doc-create"
	if _, _, err := mem.OpenLayer(ctx, uri, contribapi.LayerSource); err == nil {
		t.Fatal("Open(source) of a data-only entry succeeded")
	}
	body, err := mem.GetBody(ctx, uri)
	if err != nil || string(body) != `{"name":"doc-create"}` {
		t.Fatalf("GetBody = %q, %v; want the data layer", body, err)
	}
	l, err := mbapi.Open(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: mem}, uri, contribapi.LayerData)
	if err != nil {
		t.Fatalf("Open(data): %v", err)
	}
	if b, _ := l.Bytes(); string(b) != `{"name":"doc-create"}` {
		t.Fatalf("Open(data) = %q", b)
	}
}

// One consumer call — mbapi.Open(ctx, q, uri, "source") — across the three
// bands of the same view: inline bytes come out of the index with no call, a
// control-band layer comes over the layer lane once and is cached by hash, a
// bulk-band layer refuses to materialise and streams from the object store
// without touching the control plane.
func TestLanes_OneOpenCallAcrossInlineControlAndBulk(t *testing.T) {
	ctx := context.Background()

	inline := []byte("# small\n")
	control := bytes.Repeat([]byte("control-band line\n"), (64<<10)/18) // ~64 KiB
	bulk, bulkHash := bulkContent(t, bulkTestSize)
	if contribapi.BandOf(int64(len(inline))) != contribapi.BandNameInline ||
		contribapi.BandOf(int64(len(control))) != contribapi.BandNameControl ||
		contribapi.BandOf(int64(len(bulk))) != contribapi.BandNameBulk {
		t.Fatal("fixture sizes do not cover the three bands")
	}

	ref := func(b []byte, mt string) contribapi.LayerRef {
		return contribapi.LayerRef{Hash: contribapi.HashContent(b), Size: int64(len(b)), MediaType: mt}
	}
	inlineRef := ref(inline, "text/markdown")
	inlineRef.Inline = inline
	entries := []contribapi.Contribution{
		{Provider: "ktw-docs", Type: "reference", ID: "small.md", Layers: map[string]contribapi.LayerRef{contribapi.LayerSource: inlineRef}},
		{Provider: "ktw-docs", Type: "reference", ID: "medium.txt", Layers: map[string]contribapi.LayerRef{contribapi.LayerSource: ref(control, "text/plain")}},
		{Provider: "ktw-docs", Type: "reference", ID: "dataset.bin", Layers: map[string]contribapi.LayerRef{contribapi.LayerSource: {Hash: bulkHash, Size: int64(len(bulk)), MediaType: "application/octet-stream"}}},
	}
	cat := core.NewMemoryCatalog([]*mbapi.Manifest{{Provider: "ktw-docs", Contributions: entries}}, nil)
	blobs := core.NewFileBlobStore(t.TempDir())
	if err := blobs.Put(ctx, bulkHash, bytes.NewReader(bulk)); err != nil {
		t.Fatal(err)
	}
	cat.SetContentLane(core.NewContentStore(t.TempDir()), func(context.Context, string, string) ([]byte, error) {
		t.Fatal("the body lane was asked for a named layer")
		return nil, nil
	})
	layerCalls := map[string]int{}
	cat.SetLayerLane(func(_ context.Context, uri, layer string) ([]byte, contribapi.LayerRef, error) {
		layerCalls[uri]++
		if uri != "ktw-docs:/reference/medium.txt" {
			t.Fatalf("the layer lane was asked for %s — only the control-band layer takes it", uri)
		}
		return control, contribapi.LayerRef{MediaType: "text/plain"}, nil
	})
	cat.SetBulkLane(blobs)
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}

	open := func(uri string) *mbapi.Layer {
		t.Helper()
		l, err := mbapi.Open(ctx, q, uri, contribapi.LayerSource)
		if err != nil {
			t.Fatalf("Open %s: %v", uri, err)
		}
		return l
	}

	// Inline: out of the index, no lane taken.
	l := open("ktw-docs:/reference/small.md")
	if l.Band() != contribapi.BandNameInline {
		t.Fatalf("small band = %s", l.Band())
	}
	if b, err := l.Bytes(); err != nil || !bytes.Equal(b, inline) {
		t.Fatalf("inline bytes: %v %q", err, b)
	}

	// Control: one fetch over the layer lane, then a content-store hit.
	for i := 0; i < 2; i++ {
		l = open("ktw-docs:/reference/medium.txt")
		if l.Band() != contribapi.BandNameControl {
			t.Fatalf("medium band = %s", l.Band())
		}
		if b, err := l.Bytes(); err != nil || !bytes.Equal(b, control) {
			t.Fatalf("control bytes: %v (%d bytes)", err, len(b))
		}
	}
	if n := layerCalls["ktw-docs:/reference/medium.txt"]; n != 1 {
		t.Fatalf("control layer fetched %d times, want 1 (then cached by hash)", n)
	}

	// Bulk: Bytes refuses, streaming delivers.
	l = open("ktw-docs:/reference/dataset.bin")
	if l.Band() != contribapi.BandNameBulk {
		t.Fatalf("dataset band = %s", l.Band())
	}
	if _, err := l.Bytes(); err == nil {
		t.Fatal("Bytes materialised a bulk-band layer")
	}
	l = open("ktw-docs:/reference/dataset.bin")
	dest := filepath.Join(t.TempDir(), "dataset.bin")
	if n, err := l.WriteFile(dest); err != nil || n != int64(len(bulk)) {
		t.Fatalf("stream bulk: n=%d err=%v", n, err)
	}
	l = open("ktw-docs:/reference/dataset.bin")
	head := make([]byte, 32)
	if _, err := io.ReadFull(l, head); err != nil || !bytes.Equal(head, bulk[:32]) {
		t.Fatalf("bulk read: %v", err)
	}
	_ = l.Close()
}
