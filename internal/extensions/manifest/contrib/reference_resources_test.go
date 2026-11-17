// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"testing/fstest"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/api/providertest"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

func referenceTestFS() fstest.MapFS {
	return fstest.MapFS{
		"embed/references/overview.md":              {Data: []byte("# Overview\nTop-level note")},
		"embed/references/diagrams/architecture.md": {Data: []byte("# Architecture\nDiagram doc")},
		"embed/references/diagrams/sequence.png":    {Data: []byte{0x89, 0x50, 0x4E, 0x47}}, // PNG magic
		"embed/references/modules/auth/charter.md":  {Data: []byte("# Charter\nNested")},
	}
}

// buildRefProvider mirrors what ContributeReferenceResources installs,
// without going through the Extension wiring — tests the parameterized
// EmbedFSResourceProvider shape directly.
func buildRefProvider(provider, root string, fs fstest.MapFS) *EmbedFSResourceProvider {
	return &EmbedFSResourceProvider{
		ProviderName:        provider,
		ResourceType:        mbapi.ResourceTypeReference,
		Loader:              loader.NewEmbedFSLoader(fs, ""),
		Root:                root,
		KeepFolderStructure: true,
		KeepExtension:       true,
		BodyDecoder: func(itemPath string, _ []byte) (json.RawMessage, error) {
			return json.Marshal(mbapi.ReferenceBody{Path: itemPath})
		},
		MetadataDecoder: func(itemPath string, _ []byte) *contribapi.ContributionMeta {
			return &contribapi.ContributionMeta{MediaType: detectMediaType(itemPath)}
		},
	}
}

func TestReferenceResources_Enumerate(t *testing.T) {
	p := buildRefProvider("ktw-docs", "embed/references", referenceTestFS())

	entries, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4 (overview, architecture, sequence, charter); entries=%+v", len(entries), entries)
	}

	want := map[string]string{
		"overview.md":              "text/markdown",
		"diagrams/architecture.md": "text/markdown",
		"diagrams/sequence.png":    "image/png",
		"modules/auth/charter.md":  "text/markdown",
	}
	for _, e := range entries {
		if e.Provider != "ktw-docs" {
			t.Errorf("entry %q: Provider = %q, want ktw-docs", e.ID, e.Provider)
		}
		if e.Type != mbapi.ResourceTypeReference {
			t.Errorf("entry %q: Type = %q, want %q", e.ID, e.Type, mbapi.ResourceTypeReference)
		}
		wantMedia, ok := want[e.ID]
		if !ok {
			t.Errorf("unexpected entry ID: %q", e.ID)
			continue
		}
		var body mbapi.ReferenceBody
		if err := json.Unmarshal(e.Data(), &body); err != nil {
			t.Errorf("entry %q: decode body: %v", e.ID, err)
			continue
		}
		if body.Path != e.ID {
			t.Errorf("entry %q: body.Path = %q, want %q", e.ID, body.Path, e.ID)
		}
		// The media type is a picker field, so it is in meta and only there.
		if got := e.ResolvedMetadata().MediaType; got != wantMedia {
			t.Errorf("entry %q: meta.MediaType = %q, want %q", e.ID, got, wantMedia)
		}
	}
}

func TestReferenceResources_Open(t *testing.T) {
	p := buildRefProvider("ktw-docs", "embed/references", referenceTestFS())

	rc, ref, err := p.Open(context.Background(), "diagrams/architecture.md", contribapi.LayerSource)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if ref.MediaType != "text/markdown" {
		t.Errorf("MediaType = %q, want text/markdown", ref.MediaType)
	}
	if string(b) != "# Architecture\nDiagram doc" {
		t.Errorf("bytes content mismatch: %q", string(b))
	}

	var notFound contribapi.ErrContributionNotFound
	if _, _, err := p.Open(context.Background(), "nonexistent.md", contribapi.LayerSource); !errors.As(err, &notFound) {
		t.Errorf("Open(nonexistent): want ErrContributionNotFound, got %v", err)
	}

	providertest.Run(t, p)
}

// TestDetectMediaType: every provider of this package types a file through
// the one extension table (render.MediaTypeForPath).
func TestDetectMediaType(t *testing.T) {
	cases := []struct{ file, want string }{
		{"x.md", "text/markdown"},
		{"x.markdown", "text/markdown"},
		{"x.pdf", "application/pdf"},
		{"x.png", "image/png"},
		{"x.jpg", "image/jpeg"},
		{"x.jpeg", "image/jpeg"},
		{"x.svg", "image/svg+xml"},
		{"x.html", "text/html"},
		{"x.txt", "text/plain"},
		{"x.yaml", "application/yaml"},
		{"x.yml", "application/yaml"},
		{"x.json", "application/json"},
		{"x.typ", "text/x-typst"},
		{"x.webp", "image/webp"},
		{"x.woff2", "font/woff2"},
		{"x.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
		{"x.bin", "application/octet-stream"},
		{"noext", "application/octet-stream"},
	}
	for _, tc := range cases {
		if got := detectMediaType(tc.file); got != tc.want {
			t.Errorf("detectMediaType(%q) = %q, want %q", tc.file, got, tc.want)
		}
	}
}
