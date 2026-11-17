// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// The OBJECT tools — how an agent reaches CWM. Each tool forwards to one
// Object API verb; the file tools take and return TEXT, because the verbs
// carry []byte (base64 over JSON — an LLM cannot author it).

// namedFileReadToolOutput returns a named file's content as text.
type namedFileReadToolOutput struct {
	File      api.ObjectNamedFile `json:"file"`
	Content   string              `json:"content"`
	IsPartial bool                `json:"is_partial,omitempty"`
}

// namedFileWriteToolInput is cwm-object-named-file-write's input: text, not bytes.
type namedFileWriteToolInput struct {
	ID         string `json:"id"                   jsonschema:"Object id; required"`
	Capability string `json:"capability,omitempty" jsonschema:"Owning capability; empty = the object's own file"`
	Name       string `json:"name"                 jsonschema:"The file's name; required"`
	Content    string `json:"content"              jsonschema:"File text content; required"`
	Append     bool   `json:"append,omitempty"     jsonschema:"Append instead of overwriting"`
}

// moduleFileReadToolOutput returns a module file's content as text.
type moduleFileReadToolOutput struct {
	Content   string `json:"content"`
	SizeBytes int64  `json:"size_bytes"`
	IsPartial bool   `json:"is_partial,omitempty"`
}

// moduleFileWriteToolInput is cwm-object-file-write's input: text, not bytes.
type moduleFileWriteToolInput struct {
	ID      string `json:"id"               jsonschema:"An object id; its module is the file root. Required"`
	Path    string `json:"path"             jsonschema:"Path relative to the module folder; required"`
	Content string `json:"content"          jsonschema:"File text content; required"`
	Append  bool   `json:"append,omitempty" jsonschema:"Append instead of overwriting"`
}

// objectTools returns the object tools: reads first, writes after.
func objectTools() []toolsapi.Tool {
	return []toolsapi.Tool{
		// --- reads ---
		cwmTool("cwm-object-get",
			"Get one object by module and id: its type, capabilities (field groups) and edges.",
			func(s api.CWMService, ctx context.Context, in api.ObjectGetInput) (api.ObjectGetOutput, error) {
				return s.ObjectGet(ctx, in)
			}, toolsapi.WithReadOnly()),

		cwmTool("cwm-object-list",
			"List objects, filtered by module, type, parent and/or an effective capability. At most 50 unless limit says otherwise (negative = all); truncated and total say when the list was cut. fields keeps only the named parts of each object (e.g. name, parent, capabilities.lifecycle.state).",
			func(s api.CWMService, ctx context.Context, in api.ObjectListInput) (api.ObjectListOutput, error) {
				// The service lists all at limit 0 for its Go callers; an
				// agent's call is capped unless it asks for more.
				if in.Limit == 0 {
					in.Limit = api.ObjectListSurfaceLimit
				}
				return s.ObjectList(ctx, in)
			}, toolsapi.WithReadOnly()),

		cwmTool("cwm-object-tree",
			"The object tree under a root (a module, a project, any object), as nested summaries.",
			func(s api.CWMService, ctx context.Context, in api.ObjectTreeInput) (api.ObjectTreeOutput, error) {
				return s.ObjectTree(ctx, in)
			}, toolsapi.WithReadOnly()),

		cwmTool("cwm-object-search",
			"Search objects. mode fulltext: words over every field and the body, ranked, each hit with the field it matched and a snippet — use it to find what an object SAYS. Default mode: substring over id, name, title, description and tags. within: <id> limits to that object's subtree.",
			func(s api.CWMService, ctx context.Context, in api.ObjectSearchInput) (api.ObjectSearchOutput, error) {
				return s.ObjectSearch(ctx, in)
			}, toolsapi.WithReadOnly()),

		cwmTool("cwm-object-resolve",
			"Resolve a full object name (project / module / object) to the object it names.",
			func(s api.CWMService, ctx context.Context, in api.ObjectResolveInput) (api.ObjectResolveOutput, error) {
				return s.ObjectResolve(ctx, in)
			}, toolsapi.WithReadOnly()),

		cwmTool("cwm-object-type-get",
			"Get an object type: its capabilities, folder, id prefix and allowed children.",
			func(s api.CWMService, ctx context.Context, in api.ObjectTypeGetInput) (api.ObjectTypeGetOutput, error) {
				return s.ObjectTypeGet(ctx, in)
			}, toolsapi.WithReadOnly()),

		cwmTool("cwm-object-allowed-children",
			"The object types that may be created under an object.",
			func(s api.CWMService, ctx context.Context, in api.ObjectAllowedChildrenInput) (api.ObjectAllowedChildrenOutput, error) {
				return s.ObjectAllowedChildren(ctx, in)
			}, toolsapi.WithReadOnly()),

		cwmTool("cwm-object-capabilities",
			"The capabilities in force on an object (type, aspects and its own), with where each comes from.",
			func(s api.CWMService, ctx context.Context, in api.ObjectEffectiveCapabilitiesInput) (api.ObjectEffectiveCapabilitiesOutput, error) {
				return s.ObjectEffectiveCapabilities(ctx, in)
			}, toolsapi.WithReadOnly()),

		cwmTool("cwm-object-named-file-list",
			"List the named files an object has (from its capabilities, its type or itself) and whether each exists.",
			func(s api.CWMService, ctx context.Context, in api.ObjectNamedFileListInput) (api.ObjectNamedFileListOutput, error) {
				return s.ObjectNamedFileList(ctx, in)
			}, toolsapi.WithReadOnly()),

		cwmTool("cwm-object-named-file-read",
			"Read one of an object's named files as text; a declared file not written yet reads empty.",
			func(s api.CWMService, ctx context.Context, in api.ObjectNamedFileReadInput) (namedFileReadToolOutput, error) {
				out, err := s.ObjectNamedFileRead(ctx, in)
				if err != nil {
					return namedFileReadToolOutput{}, err
				}
				return namedFileReadToolOutput{File: out.File, Content: string(out.Content), IsPartial: out.IsPartial}, nil
			}, toolsapi.WithReadOnly()),

		cwmTool("cwm-object-error-list",
			"Every object carrying errors (invalid, a state outside its workflow, a link to an object that does not exist), with the errors, and the files that could not be read. Narrow with module or within.",
			func(s api.CWMService, ctx context.Context, in api.ObjectErrorListInput) (api.ObjectErrorListOutput, error) {
				return s.ObjectErrorList(ctx, in)
			}, toolsapi.WithReadOnly()),

		cwmTool("cwm-object-file-list",
			"List the files of an object's module (paths relative to the module folder), optionally under a prefix. Object files are not listed.",
			func(s api.CWMService, ctx context.Context, in api.ObjectFileListInput) (api.ObjectFileListOutput, error) {
				return s.ObjectFileList(ctx, in)
			}, toolsapi.WithReadOnly()),

		cwmTool("cwm-object-file-read",
			"Read a file of an object's module as text, by its path relative to the module folder. Prefer cwm-object-named-file-read for a file the object declares.",
			func(s api.CWMService, ctx context.Context, in api.ObjectFileReadInput) (moduleFileReadToolOutput, error) {
				out, err := s.ObjectFileRead(ctx, in)
				if err != nil {
					return moduleFileReadToolOutput{}, err
				}
				return moduleFileReadToolOutput{Content: string(out.Content), SizeBytes: out.SizeBytes, IsPartial: out.IsPartial}, nil
			}, toolsapi.WithReadOnly()),

		// --- writes ---
		cwmTool("cwm-object-create",
			"Create an object in a module. The object carries its id, type and capabilities; the type decides the folder.",
			func(s api.CWMService, ctx context.Context, in api.ObjectCreateInput) (api.ObjectCreateOutput, error) {
				return s.ObjectCreate(ctx, in)
			}),

		cwmTool("cwm-object-update",
			"Replace an object wholesale: get it first, change what you need, send the whole object back with the revision cwm-object-get gave. Without a revision the write is refused (an edit made since would be lost); force overwrites whatever is stored.",
			func(s api.CWMService, ctx context.Context, in api.ObjectUpdateInput) (api.ObjectUpdateOutput, error) {
				return s.ObjectUpdate(ctx, in)
			}),

		cwmTool("cwm-object-add-capability",
			"Give an object a capability: its fields (the schema's defaults fill what you leave out), after the capabilities it uses. A capability with a workflow starts in its initial state.",
			func(s api.CWMService, ctx context.Context, in api.ObjectAddCapabilityInput) (api.ObjectAddCapabilityOutput, error) {
				return s.ObjectAddCapability(ctx, in)
			}),

		cwmTool("cwm-object-remove-capability",
			"Remove a capability the object carries itself, with its fields. Refused when its type, an aspect or its attachment brings it, or another capability uses it.",
			func(s api.CWMService, ctx context.Context, in api.ObjectRemoveCapabilityInput) (api.ObjectRemoveCapabilityOutput, error) {
				return s.ObjectRemoveCapability(ctx, in)
			}),

		cwmTool("cwm-object-move",
			"Move an object under another parent, into another module, or rename it (new_local_id) — the last two change its id, and every parent link and edge naming it follows. A reason is required.",
			func(s api.CWMService, ctx context.Context, in api.ObjectMoveInput) (api.ObjectMoveOutput, error) {
				return s.ObjectMove(ctx, in)
			}),

		cwmTool("cwm-object-split",
			"Split an object into two or more successors, each taking some of its children. A reason is required.",
			func(s api.CWMService, ctx context.Context, in api.ObjectSplitInput) (api.ObjectSplitOutput, error) {
				return s.ObjectSplit(ctx, in)
			}),

		cwmTool("cwm-object-named-file-write",
			"Write (or append to) one of an object's named files, as text.",
			func(s api.CWMService, ctx context.Context, in namedFileWriteToolInput) (api.ObjectNamedFileWriteOutput, error) {
				return s.ObjectNamedFileWrite(ctx, api.ObjectNamedFileWriteInput{
					ID: in.ID, Capability: in.Capability, Name: in.Name,
					Content: []byte(in.Content), Append: in.Append,
				})
			}),

		cwmTool("cwm-object-file-write",
			"Write (or append to) a file in an object's module, as text, at a path relative to the module folder. Prefer cwm-object-named-file-write for a file the object declares.",
			func(s api.CWMService, ctx context.Context, in moduleFileWriteToolInput) (api.ObjectFileWriteOutput, error) {
				return s.ObjectFileWrite(ctx, api.ObjectFileWriteInput{ID: in.ID, Path: in.Path, Content: []byte(in.Content), Append: in.Append})
			}),

		cwmTool("cwm-object-file-attach",
			"Keep an object's attaches edges in step with files of its module: give module-relative globs (confluence/as-is/*.md, or a folder); the object then attaches exactly the files they match — missing edges added, edges to files gone removed, both reported. dry_run only reports.",
			func(s api.CWMService, ctx context.Context, in api.ObjectFileAttachInput) (api.ObjectFileAttachOutput, error) {
				return s.ObjectFileAttach(ctx, in)
			}),

		cwmTool("cwm-object-file-delete",
			"Delete a file from an object's module (irreversible). Object files cannot be deleted this way.",
			func(s api.CWMService, ctx context.Context, in api.ObjectFileDeleteInput) (api.ObjectFileDeleteOutput, error) {
				return s.ObjectFileDelete(ctx, in)
			}, toolsapi.WithDestructive()),

		cwmTool("cwm-object-archive",
			"End an object: its file moves, unchanged, to its module's .ktw/_history/<yyyy-mm>/ and leaves the project's objects. Links to it stay valid and show it ended; its id is never reused. Refused while it has children. Prefer it to delete for finished work.",
			func(s api.CWMService, ctx context.Context, in api.ObjectArchiveInput) (api.ObjectArchiveOutput, error) {
				return s.ObjectArchive(ctx, in)
			}, toolsapi.WithDestructive()),

		cwmTool("cwm-object-delete",
			"Delete one object file (irreversible). Refused while it has children or other objects link to it, naming them; detach removes those links first and lists them.",
			func(s api.CWMService, ctx context.Context, in api.ObjectDeleteInput) (api.ObjectDeleteOutput, error) {
				return s.ObjectDelete(ctx, in)
			}, toolsapi.WithDestructive()),
	}
}
