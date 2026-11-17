// SPDX-License-Identifier: Apache-2.0

package bundling

import "testing"

func TestSlug(t *testing.T) {
	tests := []struct {
		source  string
		relPath string
		wantExt string
		wantHas string // substring the slug must contain
	}{
		{
			source:  "gdrive-specs",
			relPath: "Product/Architecture/overview.pdf",
			wantExt: ".pdf",
			wantHas: "overview-",
		},
		{
			source:  "confluence-eng",
			relPath: "Engineering/API/api-design.html",
			wantExt: ".html",
			wantHas: "api-design-",
		},
		{
			source:  "local-docs",
			relPath: "Meeting Notes (2026).docx",
			wantExt: ".docx",
			wantHas: "meeting-notes-2026-",
		},
		{
			source:  "source-a",
			relPath: "report.pdf",
			wantExt: ".pdf",
			wantHas: "report-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.source+"/"+tt.relPath, func(t *testing.T) {
			slug := Slug(tt.source, tt.relPath)

			// Check extension preserved
			if ext := slug[len(slug)-len(tt.wantExt):]; ext != tt.wantExt {
				t.Errorf("Slug(%q, %q) = %q, want ext %q", tt.source, tt.relPath, slug, tt.wantExt)
			}

			// Check contains cleaned name
			if len(tt.wantHas) > 0 && !contains(slug, tt.wantHas) {
				t.Errorf("Slug(%q, %q) = %q, want contains %q", tt.source, tt.relPath, slug, tt.wantHas)
			}

			// Check deterministic
			slug2 := Slug(tt.source, tt.relPath)
			if slug != slug2 {
				t.Errorf("Slug not deterministic: %q != %q", slug, slug2)
			}
		})
	}

	// Collision safety: same filename, different sources
	slug1 := Slug("source-a", "report.pdf")
	slug2 := Slug("source-b", "report.pdf")
	if slug1 == slug2 {
		t.Errorf("Slugs should differ for same filename in different sources: %q == %q", slug1, slug2)
	}

	// Collision safety: same filename, different paths in same source
	slug3 := Slug("docs", "folder-a/report.pdf")
	slug4 := Slug("docs", "folder-b/report.pdf")
	if slug3 == slug4 {
		t.Errorf("Slugs should differ for same filename in different paths: %q == %q", slug3, slug4)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
