// SPDX-License-Identifier: Apache-2.0

package api

import "time"

// FileMeta is the optional sidecar metadata for any file in a node
// workspace. Stored on disk as `<filename>.meta.yaml` next to the
// file (e.g. `diagram.png.meta.yaml` next to `diagram.png`).
//
// Items use this convention via dual-file storage (yaml + body); raw
// attachments use it directly.
type FileMeta struct {
	Title       string    `json:"title,omitempty"        jsonschema:"Human-readable name"`
	Description string    `json:"description,omitempty"  jsonschema:"Free-form prose ≤ 800 chars"`
	Tags        []string  `json:"tags,omitempty"`
	MimeType    string    `json:"mime_type,omitempty"    jsonschema:"RFC 6838 media type (e.g. image/png, application/pdf)"`
	Source      string    `json:"source,omitempty"       jsonschema:"Provenance hint (sync source, capture tool, URL)"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

// FileEntry is a directory-listing entry — file path + metadata
// summary + size + mtime, without loading content.
type FileEntry struct {
	Path      string    `json:"path"               jsonschema:"Relative to the node folder"`
	IsDir     bool      `json:"is_dir,omitempty"`
	SizeBytes int64     `json:"size_bytes,omitempty"`
	ModTime   time.Time `json:"mod_time,omitempty"`
	Meta      *FileMeta `json:"meta,omitempty"     jsonschema:"Sidecar metadata if present"`
}
