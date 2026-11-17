// SPDX-License-Identifier: Apache-2.0

// Package api is the file-overview service contract: the curated overview of
// an object, kept as a file in its module by the `file-overview` capability.
// The capability declares two NAMED FILES — `page` (a UI page definition) and
// `overview` (markdown) — and `page` wins when both exist; this service is
// where that rule lives, so the panel reading it composes no path and picks no
// file.
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// Capability is the capability whose named files hold the overview.
const Capability = "file-overview"

// Names are the capability's named files, the first existing one shown: the
// order's one source (TestNamesAreTheDeclaredFiles keeps it in step with the
// files the capability declares).
var Names = []string{"page", "overview"}

// FileOverviewServiceDef describes the service for KTSP publishing.
var FileOverviewServiceDef = meta.ServiceDef{
	Name:    "file-overview",
	Short:   "An object's curated overview file",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Read": {Short: "An object's overview: its page definition when it has one, else its markdown overview, else none yet", ReadOnly: true},
		"Page": {Short: "An object's overview as the display that shows it: its page, else its markdown overview as a document, else a page saying there is none yet", ReadOnly: true},
	},
}

// FileOverviewService reads an object's overview.
type FileOverviewService interface {
	Read(ctx context.Context, in ReadInput) (ReadOutput, error)
	// Page is the overview as a whole display definition — the source of the
	// file-overview panel: the page file as written (a component naming its
	// kind, or a bare page tree), else the markdown overview as a document of
	// its media type, else a page with the empty state.
	Page(ctx context.Context, in ReadInput) (render.Def, error)
}

// ReadInput names the object.
type ReadInput struct {
	ID string `json:"id" jsonschema:"Object id; required"`
}

// ReadOutput is the overview shown: Exists is false when neither file is
// written yet (not an error — the panel invites the overview action).
type ReadOutput struct {
	Name    string `json:"name,omitempty"    jsonschema:"The named file shown: page or overview"`
	Path    string `json:"path,omitempty"    jsonschema:"Its module-relative path; its extension types it"`
	Media   string `json:"media,omitempty"`
	Exists  bool   `json:"exists"`
	Content string `json:"content,omitempty"`
}
