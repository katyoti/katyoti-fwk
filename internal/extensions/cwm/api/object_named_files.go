// SPDX-License-Identifier: Apache-2.0

package api

// NAMED FILES — a capability declares the files it manages (decided
// 2026-09-26, the user: "cwm should be able to list/read/write files from a
// module, allowing capabilities to manage named files").
//
// A capability says, once, which files it owns and where they live:
//
//	files:
//	  overview: {path: "overview/${id}.md", media: text/markdown}
//
// The path is a plain MODULE-RELATIVE path, and may use `${...}` over the
// object (current / object / id / type — the object vocabulary), so a file can
// be one per module (`notes.md`), one per object (`notes/${id}.md`), or laid
// out however the capability's author chooses. No caller composes the path:
// `ObjectNamedFile*` resolve it from the declaration, which is the whole point
// — the file overview, a Confluence workspace config and an agent's memory are
// one mechanism instead of three hand-built path conventions.
//
// The resolved path goes through the ObjectFile* verbs, so it gets their
// rules unchanged: rooted at the object's module, `..` refused, and an object
// file (.cwm.yaml / .cwm.md) is never reachable — those are the Object API's.

// THREE SOURCES, closer wins -- the same rule as capabilities (decided
// 2026-09-26, the user: "a file can come from the capability, from the object
// type, or can be directly declared in an object, allowing a capability to add
// a file at runtime, object per object"):
//
//   - a CAPABILITY's `files:` -- every object carrying it has the file;
//   - an OBJECT TYPE's `files:` -- every object of the type has it;
//   - the OBJECT's own `files:` -- this object alone, added at runtime through
//     ObjectNamedFileDeclare (a capability's code declaring a file on the
//     object it works on).
//
// A file is addressed by (capability, name). A type or object declaration may
// name its owning `capability`; one that names none belongs to the object
// itself (capability ""). The same key declared closer -- object over type over
// capability -- replaces the farther one.

// NamedFile is one declared file.
type NamedFile struct {
	Path        string `json:"path"                  yaml:"path"                  jsonschema:"description=Module-relative path; may use ${...} over the object (current / object / id / type)"`
	Media       string `json:"media,omitempty"       yaml:"media,omitempty"       jsonschema:"description=Media type of the content (for example text/markdown)"`
	Description string `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"description=What the file holds"`
	// Capability is the OWNER of a type or object declaration; empty = the
	// object's own. Ignored in a capability's own files (it owns them).
	Capability string `json:"capability,omitempty" yaml:"capability,omitempty" jsonschema:"description=Owning capability of a type or object declaration; empty means the object's own file"`
	// Owned says the file lives with the object: moved when a rename moves
	// its path, deleted with the object. A file the object only names (a
	// module's concepts page) is never carried or removed.
	Owned bool `json:"owned,omitempty" yaml:"owned,omitempty" jsonschema:"description=The file lives with the object: moved with it when renamed and deleted with it"`
}

// Named-file sources (ObjectNamedFile.Source).
const (
	NamedFileFromCapability = "capability"
	NamedFileFromType       = "type"
	NamedFileFromObject     = "object"
)

// ObjectNamedFile is one declared file, resolved for one object.
type ObjectNamedFile struct {
	Capability string `json:"capability" jsonschema:"Owning capability; empty = the object's own file"`
	Name       string `json:"name"`
	// Source is where the declaration in force came from: capability | type |
	// object -- the closer one wins.
	Source      string `json:"source"`
	Path        string `json:"path"                  jsonschema:"Module-relative path the declaration resolved to"`
	Media       string `json:"media,omitempty"`
	Description string `json:"description,omitempty"`
	Exists      bool   `json:"exists"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	// Error is set when the declaration could not be resolved for this object
	// (a `${...}` naming nothing, a path escaping the module). Reported per
	// file, so one bad declaration does not hide the others.
	Error string `json:"error,omitempty"`
}

// ObjectNamedFileListInput lists the named files of every capability in force
// on an object.
type ObjectNamedFileListInput struct {
	ID string `json:"id" jsonschema:"Object id; required"`
}

// ObjectNamedFileListOutput returns them, by capability then name.
type ObjectNamedFileListOutput struct {
	Module string            `json:"module"`
	Files  []ObjectNamedFile `json:"files"`
}

// ObjectNamedFileReadInput reads one named file.
type ObjectNamedFileReadInput struct {
	ID         string `json:"id"               jsonschema:"Object id; required"`
	Capability string `json:"capability,omitempty" jsonschema:"Owning capability; empty = the object's own file. A capability must be in force on the object"`
	Name       string `json:"name"             jsonschema:"The file's name"`
	Offset     int    `json:"offset,omitempty" jsonschema:"Byte offset for a partial read"`
	Limit      int    `json:"limit,omitempty"  jsonschema:"Max bytes to return; 0 = the whole file"`
}

// ObjectNamedFileReadOutput returns the bytes. A declared file that does not
// exist yet is not an error: File.Exists is false and Content is empty.
type ObjectNamedFileReadOutput struct {
	File      ObjectNamedFile `json:"file"`
	Content   []byte          `json:"content"`
	IsPartial bool            `json:"is_partial,omitempty"`
}

// ObjectNamedFileWriteInput writes (or appends to) one named file.
type ObjectNamedFileWriteInput struct {
	ID         string `json:"id"               jsonschema:"Object id; required"`
	Capability string `json:"capability,omitempty" jsonschema:"Owning capability; empty = the object's own file. A capability must be in force on the object"`
	Name       string `json:"name"             jsonschema:"The file's name"`
	Content    []byte `json:"content"          jsonschema:"Raw bytes"`
	Append     bool   `json:"append,omitempty" jsonschema:"Append instead of overwriting"`
}

// ObjectNamedFileWriteOutput reports what was written.
type ObjectNamedFileWriteOutput struct {
	File        ObjectNamedFile `json:"file"`
	ProjectPath string          `json:"project_path" jsonschema:"The same file relative to the project — what a target: path edge stores"`
}

// ObjectNamedFileDeclareInput adds (or removes) a named file on ONE object --
// its own `files:` map. This is how a capability's code gives the object it
// works on a file at runtime. Declaring does not write bytes; the file is then
// read and written by name like any other.
type ObjectNamedFileDeclareInput struct {
	ID   string    `json:"id"   jsonschema:"Object id; required"`
	Name string    `json:"name" jsonschema:"The file's name; unique in the object's own files"`
	File NamedFile `json:"file" jsonschema:"The declaration: path (module-relative) and optionally media / description / owning capability"`
	// Remove drops the object's own declaration of Name (the bytes stay; a
	// farther declaration of the same key, if any, is in force again).
	Remove bool `json:"remove,omitempty" jsonschema:"Remove the object's own declaration instead of adding it"`
}

// ObjectNamedFileDeclareOutput returns the declaration as now in force.
type ObjectNamedFileDeclareOutput struct {
	File ObjectNamedFile `json:"file"`
}
