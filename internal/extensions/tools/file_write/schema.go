// SPDX-License-Identifier: Apache-2.0

// Package file_write implements the file_write agent tool.
//
// Spec: docs/agent_tools/tool_file_write_spec.md
//
// MVP scope (ms04-p1): mutex-fields path API (workspace_path XOR
// absolute_path), workspace boundary check on the absolute branch,
// read-before-write enforcement via filestate.Store, staleness
// rejection on mtime drift, parent-dir creation, LF line endings,
// 1 MiB content cap. Always writes UTF-8.
//
// Out of scope, deferred to ms05:
//   - Structured Hunk[] diff output → ms05/tool-file-write-diff
//   - BOM-aware encoding preservation → ms05/tool-file-write-encoding
//   - File history backup (no service exists)
//   - Team memory secret guard (no concept exists)
//   - UNC path Windows special-case
//   - Content-comparison fallback for unreliable mtime
//   - Permission deny rules pre-check (handled by kernel-permission-gate)
//
// File layout (mirrors file_read):
//   - schema.go      — input/output types, WriteOutputType constants,
//     PathResolver interface
//   - config.go      — FileWriteConfig and DI extension
//   - validate.go    — validation, path resolution, blocklists,
//     workspace boundary check (intentional copy of
//     file_read's helpers; refactor target after
//     file_edit becomes the third consumer)
//   - permission.go  — CheckPermissions policy
//   - file_write.go  — tool constructor and handler
//   - *_test.go      — unit tests
package file_write

// WriteInput is the input schema for the file_write tool.
//
// **Exactly one** of WorkspacePath or AbsolutePath MUST be set.
// Validation rejects both-set or neither-set with a nudge toward
// WorkspacePath as the safe default.
type WriteInput struct {
	// WorkspacePath is a path relative to the agent's workspace root.
	// This is the safe default — workspace-internal writes are always
	// allowed without prompting the user. Mutually exclusive with
	// AbsolutePath.
	WorkspacePath string `json:"workspace_path,omitempty" jsonschema:"Workspace-relative path. Use this for any file inside the project. Mutually exclusive with absolute_path."`

	// AbsolutePath is an absolute filesystem path. Writing a file
	// outside the workspace requires user authorization (the tool
	// will ask via the kernel permission gate). Mutually exclusive
	// with WorkspacePath.
	AbsolutePath string `json:"absolute_path,omitempty" jsonschema:"Absolute filesystem path. Use this only when the file is outside the workspace; this will require user approval. Mutually exclusive with workspace_path."`

	// Content is the bytes to write to the file. Empty content is
	// allowed (creating a placeholder file is a legitimate use).
	// Content with `\r\n` line endings is normalized to `\n` before
	// writing per spec FR-FW-08.
	Content string `json:"content" jsonschema:"File content to write. CRLF line endings are normalized to LF before writing."`
}

// WriteOutputType is the discriminated-union tag for WriteOutput.File.
//
// MVP variants: `create` and `update`. The `update_with_diff`
// constant is reserved for ms05/tool-file-write-diff so the JSON
// schema doesn't churn when structured diff output lands.
type WriteOutputType string

const (
	// OutputTypeCreate signals that a new file was created (no prior
	// version existed on disk).
	OutputTypeCreate WriteOutputType = "create"

	// OutputTypeUpdate signals that an existing file was overwritten.
	OutputTypeUpdate WriteOutputType = "update"

	// OutputTypeUpdateWithDiff is reserved for ms05 (rich diff output).
	// MVP file_write never produces this variant — included so the
	// JSON schema is forward-compatible.
	OutputTypeUpdateWithDiff WriteOutputType = "update_with_diff"
)

// WriteOutput is the discriminated-union output of file_write.
//
// Type identifies which payload variant File holds. The JSON-marshalled
// shape is `{"type": "...", "file": {...}}`. Variant payloads are typed
// structs (CreateOutputFile, UpdateOutputFile) so unmarshalling can
// drive off the Type field with json.RawMessage if needed downstream;
// inside this package the producer always knows the variant.
//
// Warning is an optional human-readable note attached to a successful
// write. Used for cases that are not errors but worth flagging — for
// example, an absolute_path that resolved inside the workspace and
// would have been better expressed as workspace_path.
type WriteOutput struct {
	Type    WriteOutputType `json:"type"           jsonschema:"Output variant tag. 'create' for new files, 'update' for overwrites."`
	File    any             `json:"file,omitempty" jsonschema:"Variant-specific payload. See the corresponding *OutputFile type."`
	Warning string          `json:"warning,omitempty" jsonschema:"Optional human-readable note (e.g. prefer workspace_path)."`
}

// CreateOutputFile is the payload for WriteOutput.File when
// Type == "create". A new file was written; no prior version existed.
type CreateOutputFile struct {
	FilePath     string `json:"file_path"     jsonschema:"Absolute path of the file that was created."`
	BytesWritten int    `json:"bytes_written" jsonschema:"Number of bytes written to disk."`
}

// UpdateOutputFile is the payload for WriteOutput.File when
// Type == "update". An existing file was overwritten.
//
// Note: a structured diff is NOT included in MVP — that's deferred
// to ms05/tool-file-write-diff. The MVP output gives the LLM enough
// to know the write succeeded; for self-correction the LLM can
// re-read the file via file_read.
type UpdateOutputFile struct {
	FilePath     string `json:"file_path"     jsonschema:"Absolute path of the file that was overwritten."`
	BytesWritten int    `json:"bytes_written" jsonschema:"Number of bytes written to disk."`
}

// PathResolver resolves a workspace-relative path to an absolute
// path AND exposes the workspace root so file_write can classify
// absolute_path inputs against the workspace boundary.
//
// Same shape as file_read's PathResolver — duck-typed, so the
// existing workspace struct in internal/extensions/workspace
// satisfies both with no further changes.
type PathResolver interface {
	// Resolve maps a workspace-relative path to an absolute path,
	// rejecting absolute inputs and `..` escapes. Used for the
	// workspace_path branch of file_write.
	Resolve(relPath string) (string, error)

	// WorkspaceRoot returns the canonicalized absolute path of the
	// workspace root. Used to classify absolute_path inputs as
	// in-workspace vs out-of-workspace for the permission check.
	WorkspaceRoot() string
}
