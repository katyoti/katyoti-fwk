// SPDX-License-Identifier: Apache-2.0

package api

import "time"

// AN OBJECT'S ATTACHED FILES LIVE IN ITS MODULE (decided 2026-09-25).
//
// The rule, in the user's words: *each module should have a self-contained
// view, so in the module — and if we do not put it in git, then we just attach
// a link to it.* So there are exactly TWO shapes and no third:
//
//   - a FILE IN THE MODULE, committed with it, referenced by a module-relative
//     path. The module is self-contained: it travels with its own bytes, and
//     G1 ("a module is self-contained") is the reason the path is relative to
//     the module rather than to the project.
//   - a LINK, when the bytes are not in git — an edge whose kind declares
//     `target: uri`. Not a smaller file, not a stub: a reference to something
//     that lives elsewhere, declared so it can be listed and checked.
//
// WHAT THAT REPLACES. The node file verbs wrote into the node's own folder
// under `.katyoti` — recreatable STATE, gitignored, and invisible to anyone
// who cloned the repo. A module whose attachments only exist in one person's
// workspace is the opposite of self-contained.
//
// THE MODULE OWNS THE BYTES; AN OBJECT REFERENCES THEM. There is deliberately
// no per-object folder: two objects may point at one diagram, an object may
// point at a file that was there before it, and inventing
// `files/<object-id>/` would make the association a path again — the thing
// this refactor has removed three times. The association is an EDGE with a
// `path` target, checked like any other.

// ObjectFileReadInput reads bytes attached to an object's module.
type ObjectFileReadInput struct {
	// ID is any object in the module whose files are being read. The MODULE is
	// what scopes the path; the object is how a caller names it.
	ID string `json:"id"   jsonschema:"An object id; its module is the file root. Required"`
	// Path is relative to the module folder, e.g. `diagrams/flow.png`.
	Path     string `json:"path"                jsonschema:"Path relative to the module folder; required"`
	Offset   int    `json:"offset,omitempty"    jsonschema:"Byte offset for a partial read"`
	Limit    int    `json:"limit,omitempty"     jsonschema:"Max bytes to return; 0 = the whole file"`
	WithMeta bool   `json:"with_meta,omitempty" jsonschema:"Also return the sidecar FileMeta when present"`
}

// ObjectFileReadOutput returns the bytes and what is known about them.
type ObjectFileReadOutput struct {
	Content   []byte    `json:"content"`
	SizeBytes int64     `json:"size_bytes"           jsonschema:"Full size on disk, whatever was read"`
	IsPartial bool      `json:"is_partial,omitempty"`
	Meta      *FileMeta `json:"meta,omitempty"`
}

// ObjectFileWriteInput writes bytes into the object's module.
type ObjectFileWriteInput struct {
	ID      string    `json:"id"               jsonschema:"An object id; its module is the file root. Required"`
	Path    string    `json:"path"             jsonschema:"Path relative to the module folder; required"`
	Content []byte    `json:"content"          jsonschema:"Raw bytes; required"`
	Meta    *FileMeta `json:"meta,omitempty"   jsonschema:"Optional sidecar metadata written beside the file"`
	Append  bool      `json:"append,omitempty" jsonschema:"Append instead of overwriting when the file exists"`
}

// ObjectFileWriteOutput reports what was written, with the path as the caller
// would reference it in an edge.
type ObjectFileWriteOutput struct {
	// Path is module-relative, as given.
	Path string `json:"path"`
	// ProjectPath is the same file relative to the PROJECT, which is what a
	// `target: path` edge stores — so a caller can attach what it just wrote
	// without composing the path itself and getting it subtly wrong.
	ProjectPath string    `json:"project_path"`
	SizeBytes   int64     `json:"size_bytes"`
	ModTime     time.Time `json:"mod_time"`
}

// ObjectFileListInput lists the files of an object's module.
type ObjectFileListInput struct {
	ID        string `json:"id"                  jsonschema:"An object id; its module is the file root. Required"`
	Prefix    string `json:"prefix,omitempty"    jsonschema:"Only paths under this module-relative prefix"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"Walk subdirectories"`
	WithMeta  bool   `json:"with_meta,omitempty" jsonschema:"Include sidecar metadata"`
}

// ObjectFileListOutput returns the entries, sorted by path.
type ObjectFileListOutput struct {
	// Module is which module's files these are — stated, because the caller
	// passed an object id and the answer is about its module.
	Module string      `json:"module"`
	Files  []FileEntry `json:"files"`
}

// ObjectFileDeleteInput removes a file from the object's module.
type ObjectFileDeleteInput struct {
	ID       string `json:"id"                  jsonschema:"An object id; its module is the file root. Required"`
	Path     string `json:"path"                jsonschema:"Path relative to the module folder; required"`
	WithMeta bool   `json:"with_meta,omitempty" jsonschema:"Also remove the sidecar .meta.yaml when present"`
}

// ObjectFileDeleteOutput lists everything removed.
type ObjectFileDeleteOutput struct {
	DeletedPaths []string `json:"deleted_paths"`
}

// ObjectFileStatInput asks about one file without reading it.
type ObjectFileStatInput struct {
	ID   string `json:"id"   jsonschema:"An object id; its module is the file root. Required"`
	Path string `json:"path" jsonschema:"Path relative to the module folder; required"`
}

// ObjectFileStatOutput describes the file.
type ObjectFileStatOutput struct {
	Entry FileEntry `json:"entry"`
	// ProjectPath is what a `target: path` edge would store for this file.
	ProjectPath string `json:"project_path"`
}

// ObjectFileAttachInput keeps an object's `path` edges in step with files of
// its module: after the call the object attaches exactly the files the
// patterns match — the missing edges added, the edges whose file is gone (or
// no longer matches) removed. Edges of the kind that no pattern covers are
// left as they are, so one object may attach several folders, each kept in
// step by its own call.
type ObjectFileAttachInput struct {
	ID string `json:"id" jsonschema:"An object id; its module is the file root. Required"`
	// Patterns are module-relative globs (path.Match: `*` stays inside one
	// folder, `[0-9]` a class). A folder (`confluence/as-is`) stands for the
	// files directly in it.
	Patterns []string `json:"patterns" jsonschema:"Module-relative globs such as confluence/as-is/*.md; a folder stands for the files directly in it. Required"`
	Kind     string   `json:"kind,omitempty"    jsonschema:"The edge kind; it must target a path. Default attaches"`
	DryRun   bool     `json:"dry_run,omitempty" jsonschema:"Report what would change without writing"`
}

// ObjectFileAttachOutput reports the change, each file as the edge stores it
// (relative to the project).
type ObjectFileAttachOutput struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Added    []string `json:"added"`
	Removed  []string `json:"removed"`
	Attached []string `json:"attached" jsonschema:"Every file the patterns cover now attached, sorted"`
	Changed  bool     `json:"changed"  jsonschema:"The object was written (false: already in step, or a dry run)"`
}

// EdgeKindAttaches is the edge kind of a file attached to an object, in its
// module (ktw-methods' `attaches`, target: path).
const EdgeKindAttaches = "attaches"
