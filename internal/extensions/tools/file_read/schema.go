// SPDX-License-Identifier: Apache-2.0

// Package file_read implements the file_read agent tool.
//
// This file holds the input/output schema. The schema is the only part of
// the tool LLMs see directly (via the auto-generated JSON schema), so the
// shape, field names, and jsonschema tags are load-bearing — they are
// effectively part of the prompt.
//
// Spec reference: docs/agent_tools/tool_file_read_spec.md
//
// MVP scope notes:
//   - Image, PDF, and notebook variants are reserved here but NOT
//     implemented in ms04-p1. They land in
//     ms05/tool-file-read-rich-formats. Reserving the type strings now
//     keeps the JSON schema stable across the two ships.
//   - The path API diverges from spec FR-FR-01 (which mandates a single
//     `file_path` absolute-only field). Rationale: the LLM benefits from
//     being forced to consciously choose between workspace-internal
//     reads (the safe default) and out-of-workspace reads (which trigger
//     a permission prompt).
package file_read

// ReadInput is the input schema for the file_read tool.
//
// **Exactly one** of WorkspacePath or AbsolutePath MUST be set.
// Validation rejects both-set or neither-set with a nudge toward
// WorkspacePath as the safe default.
type ReadInput struct {
	// WorkspacePath is a path relative to the agent's workspace root.
	// This is the safe default — workspace-internal reads are always
	// allowed without prompting the user. Mutually exclusive with
	// AbsolutePath.
	WorkspacePath string `json:"workspace_path,omitempty" jsonschema:"Workspace-relative path. Use this for any file inside the project. Mutually exclusive with absolute_path."`

	// AbsolutePath is an absolute filesystem path. Reading a file
	// outside the workspace requires user authorization (the tool will
	// ask via the kernel permission gate). Mutually exclusive with
	// WorkspacePath.
	AbsolutePath string `json:"absolute_path,omitempty" jsonschema:"Absolute filesystem path. Use this only when the file is outside the workspace; this will require user approval. Mutually exclusive with workspace_path."`

	// Offset is the 0-based starting line for a partial read. nil means
	// "read from the beginning of the file." Negative values are
	// rejected by validation.
	Offset *int `json:"offset,omitempty" jsonschema:"Optional 0-based starting line for a partial read."`

	// Limit is the maximum number of lines to read. nil means "use the
	// configured default" (typically 2000). Zero or negative values are
	// rejected by validation.
	Limit *int `json:"limit,omitempty" jsonschema:"Optional maximum number of lines to read. Defaults to 2000 when omitted."`
}

// ReadOutputType is the discriminated-union tag for ReadOutput.File.
// Only `text` and `file_unchanged` are produced by the ms04-p1 MVP.
// The other constants are reserved for ms05/tool-file-read-rich-formats
// so the JSON schema does not churn when image/PDF/notebook variants
// land.
type ReadOutputType string

const (
	// OutputTypeText is the standard text-content variant.
	OutputTypeText ReadOutputType = "text"

	// OutputTypeFileUnchanged is returned when a dedup hit short-
	// circuits the read. The agent already has the content from a
	// previous turn.
	OutputTypeFileUnchanged ReadOutputType = "file_unchanged"

	// OutputTypeImage is reserved for ms05 (rich-formats todo).
	OutputTypeImage ReadOutputType = "image"

	// OutputTypePDF is reserved for ms05 (rich-formats todo).
	OutputTypePDF ReadOutputType = "pdf"

	// OutputTypeNotebook is reserved for ms05 (rich-formats todo).
	OutputTypeNotebook ReadOutputType = "notebook"

	// OutputTypeParts is reserved for ms05 (rich-formats todo) — used
	// when a large PDF is split into per-page image files.
	OutputTypeParts ReadOutputType = "parts"
)

// ReadOutput is the discriminated-union output of file_read.
//
// Type identifies which payload variant File holds. The JSON-marshalled
// shape is `{"type": "...", "file": {...}}`. Variant payloads are typed
// structs (TextOutputFile, FileUnchangedOutputFile) so unmarshalling can
// drive off the Type field with json.RawMessage if needed downstream;
// inside this package the producer always knows the variant.
type ReadOutput struct {
	Type ReadOutputType `json:"type"           jsonschema:"Output variant tag. Always 'text' for normal reads or 'file_unchanged' for dedup hits."`
	File any            `json:"file,omitempty" jsonschema:"Variant-specific payload. See the corresponding *OutputFile type."`

	// Warning is an optional human-readable note attached to a successful
	// read. Used for cases that are not errors but worth flagging — for
	// example, an empty file, an offset past EOF, or an absolute_path
	// that resolved inside the workspace and would have been better
	// expressed as workspace_path.
	Warning string `json:"warning,omitempty" jsonschema:"Optional human-readable note about the read (e.g. empty file, offset past EOF)."`
}

// TextOutputFile is the payload for ReadOutput.File when Type == "text".
//
// Content is the cat-n-formatted body returned to the LLM (line number,
// tab, content). NumLines counts the lines actually returned (so a
// partial read with Offset=10, Limit=5 returns NumLines=5). StartLine is
// the 1-based line number of the first line in Content (so a partial
// read with Offset=10 returns StartLine=11). TotalLines is the total
// line count of the file on disk, regardless of the read window.
type TextOutputFile struct {
	FilePath   string `json:"file_path"   jsonschema:"Absolute path of the file that was read."`
	Content    string `json:"content"     jsonschema:"Cat-n-formatted file content (line number, tab, line text)."`
	NumLines   int    `json:"num_lines"   jsonschema:"Number of lines included in Content."`
	StartLine  int    `json:"start_line"  jsonschema:"1-based line number of the first line in Content."`
	TotalLines int    `json:"total_lines" jsonschema:"Total number of lines in the entire file on disk."`
}

// FileUnchangedOutputFile is the payload for ReadOutput.File when
// Type == "file_unchanged". Returned by a dedup hit; the LLM should
// refer back to the prior turn's content for the body.
type FileUnchangedOutputFile struct {
	FilePath string `json:"file_path" jsonschema:"Absolute path of the file. The previous read result is still valid; refer to it."`
}

// PathResolver resolves a workspace-relative path to an absolute path
// AND exposes the workspace root so file_read can classify
// absolute_path inputs against the workspace boundary.
//
// This is wider than the PathResolver interfaces declared in
// file_write and file_edit (they only need Resolve). The widening is
// duck-typed: the workspace struct implements both, so existing code
// is unaffected. file_write and file_edit will adopt this richer
// interface in their own ms04-p1 audits if they need the boundary
// check.
type PathResolver interface {
	// Resolve maps a workspace-relative path to an absolute path,
	// rejecting absolute inputs and `..` escapes. Used for the
	// workspace_path branch of file_read.
	Resolve(relPath string) (string, error)

	// WorkspaceRoot returns the canonicalized absolute path of the
	// workspace root. Used to classify absolute_path inputs as
	// in-workspace vs out-of-workspace.
	WorkspaceRoot() string
}
