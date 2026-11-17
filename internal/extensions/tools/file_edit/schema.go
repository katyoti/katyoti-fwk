// SPDX-License-Identifier: Apache-2.0

// Package file_edit implements the file-edit agent tool.
//
// Spec: docs/agent_tools/tool_file-edit_spec.md
// MVP scope (ms04-p1): exact string replacement + replace_all mode,
// read-before-write enforcement via filestate.Store, staleness
// rejection, mutex-fields path API, workspace boundary policy,
// new-file-via-empty-old_string creation. Deferred: quote
// normalization (ms05), structured diff output (ms05), encoding
// preservation (UTF-8 only), file history backup.
package file_edit

import (
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/pathtool"
)

// PathResolver is a type alias for pathtool.PathResolver so the tool
// constructor keeps its existing signature shape. Per-tool aliases
// will be removed when the file-read/file-write migration to pathtool
// lands.
type PathResolver = pathtool.PathResolver

// EditInput is the input schema for the file-edit tool.
type EditInput struct {
	WorkspacePath string `json:"workspace_path,omitempty" jsonschema:"Workspace-relative path. Use this for any file inside the project. Mutually exclusive with absolute_path."`
	AbsolutePath  string `json:"absolute_path,omitempty"  jsonschema:"Absolute filesystem path. Use this only when the file is outside the workspace; this will require user approval. Mutually exclusive with workspace_path."`
	OldString     string `json:"old_string"               jsonschema:"The text to find and replace. Must be unique in the file unless replace_all is true. Empty string + non-existent file = create a new file."`
	NewString     string `json:"new_string"               jsonschema:"The replacement text. Must differ from old_string."`
	ReplaceAll    bool   `json:"replace_all,omitempty"    jsonschema:"Replace all occurrences of old_string (default: false, requires uniqueness)."`
}

// EditOutput is the output of file_edit.
type EditOutput struct {
	FilePath     string `json:"file_path"                jsonschema:"Absolute path of the edited file."`
	Replacements int    `json:"replacements"             jsonschema:"Number of replacements made."`
	Created      bool   `json:"created,omitempty"        jsonschema:"True if a new file was created (empty old_string + non-existent file)."`
	Warning      string `json:"warning,omitempty"        jsonschema:"Optional human-readable note (e.g. prefer workspace_path)."`
}
