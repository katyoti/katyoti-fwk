// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// kind_source.go — how a kind's files become its layers.
//
// A kind declares WHICH layers its entries carry (Layers); its `source:` block
// says how a file of a pack becomes them: in which folder its files sit, which
// files are its, how an entry's id is read from the path, how the file is read
// (its format) and what each layer is taken from. One generic producer reads
// any kind from this declaration, so knowing how a skill or a task is laid out
// is data a kind states once, never Go code linked into every producer.
// See modules/contributions/contribution-types.md ("How files become layers").

// SourceDecl says how the files of a pack become entries of the kind.
type SourceDecl struct {
	// Folder is the pack folder holding the kind's files. Absent: the
	// kind's name. Two kinds never claim one folder.
	Folder string `json:"folder,omitempty" jsonschema:"description=The pack folder holding the kind files. Absent - the kind name. Two kinds never claim one folder."`

	// Former names folders the kind's files were read from before its folder
	// was renamed: a pack (an override tree, a project pack) still laid out
	// that way is read from one of them, with a warning, while it has no
	// folder of the current name.
	Former []string `json:"former,omitempty" jsonschema:"description=Folders the kind was read from before its folder was renamed: a pack with no folder of the current name is read from one of these with a warning"`

	// Match lists the globs (path.Match, on the file's name) a file must
	// match to be an entry. Absent: every file of the folder.
	Match []string `json:"match,omitempty" jsonschema:"description=Globs a file name must match to be an entry. Absent - every file of the folder."`

	// ID is how an entry's id is read: from its path under the folder —
	// basename (the file's name without its extension — the default), dir
	// (the folder holding the file, for a kind whose entry is a folder),
	// path (the path without its extension, sub-folders kept), file (the path
	// with its extension: a reference file named as it is) — or name (the
	// data layer's `name` field, for a kind addressed by what it says it is
	// whatever its layout), or name-or-file (that name, else the file's
	// basename, then written into the data as its name - a CWM declaration
	// is named by its body or its file). A file with no id is dropped.
	ID string `json:"id,omitempty" jsonschema:"description=How an entry id is read - from its path under the folder: basename (the file name without its extension; the default); dir (the folder holding the file); path (the path without its extension); file (the path with its extension) - or name (the data layer name field) or name-or-file (that name else the file one written into the data). A file with no id is dropped.,enum=basename|dir|path|file|name|name-or-file"`

	// Skip names folders whose files are part of an entry, never entries:
	// a skill's own `references`, `scripts`, `assets`. Matched against
	// every folder of a file's path under the kind's folder.
	Skip []string `json:"skip,omitempty" jsonschema:"description=Folders whose files are part of an entry and never entries (a skill references and scripts and assets)"`

	// Claims names the file that is the entry of its folder (TASK.md): the
	// folder's other files are part of that entry, never entries of their
	// own. Files elsewhere are entries as match says (a flat <name>.md task).
	Claims string `json:"claims,omitempty" jsonschema:"description=The file that is the entry of its folder (TASK.md) - the folder other files are part of it and never entries"`

	// Where keeps the files whose data layer has these field values (a
	// persona is a task file whose category is persona); the others are
	// not entries of the kind — left out, not dropped.
	Where map[string]string `json:"where,omitempty" jsonschema:"description=Keeps the files whose data layer has these field values (category persona); the others are left out and not dropped"`

	// Format is how the file is read: raw (bytes as they are), structured
	// (a YAML or JSON tree), frontmatter (a YAML header and a body) or dsl
	// (a schema document).
	Format string `json:"format" jsonschema:"description=How the file is read - raw (bytes as they are); structured (a YAML or JSON tree); frontmatter (a YAML header and a body) or dsl (a schema document),enum=raw|structured|frontmatter|dsl"`

	// Layers says what each declared layer is taken from, by layer name.
	// A layer not listed here is not read from the file.
	Layers map[string]LayerFrom `json:"layers,omitempty" jsonschema:"description=What each declared layer is taken from by layer name. A layer not listed is not read from the file."`

	// Companions are entries of other kinds read from files beside an
	// entry's own: a task's input and output schemas as data-types.
	Companions []Companion `json:"companions,omitempty" jsonschema:"description=Entries of other kinds read from files beside an entry own (a task input and output schemas)"`
}

// LayerFrom is what one layer of an entry is taken from.
type LayerFrom struct {
	// From is the part of the file the layer is: file (its bytes),
	// frontmatter (its header, as data), body (what follows the header),
	// tree (the structured document, as data), path (the entry's path under
	// the kind's folder, as data `{path}`: an entry whose data says where its
	// file is) or sibling (a file beside it, named by Suffix).
	From string `json:"from" jsonschema:"description=The part of the file the layer is - file (its bytes); frontmatter (its header as data); body (what follows the header); tree (the structured document as data); path (the entry path as data {path}; the data layer only) or sibling (a file beside it named by suffix),enum=file|frontmatter|body|tree|path|sibling"`

	// Suffix names the sibling file: the entry file's name with its
	// extension replaced by Suffix (`.i18n.yaml`, `.drawio`). Only with
	// from: sibling.
	Suffix string `json:"suffix,omitempty" jsonschema:"description=The sibling file - the entry file name with its extension replaced by this suffix. Only with from sibling."`

	// Fold also writes the layer's text into the data layer, under this
	// field: a persona's body as its `body`. The kind must declare a data
	// layer. A part the kind does not declare as a layer may still fold: it
	// is then written into data only, never published as a layer (a
	// template library's .drawio file).
	Fold string `json:"fold,omitempty" jsonschema:"description=Also writes the layer text into the data layer under this field. The kind must declare a data layer."`
}

// Companion is an entry of another kind read from a file beside an entry.
type Companion struct {
	// Kind is the companion's kind; it must be declared.
	Kind string `json:"kind" jsonschema:"description=The companion kind; it must be declared"`

	// Suffix names the companion's file, as LayerFrom.Suffix does.
	Suffix string `json:"suffix" jsonschema:"description=Names the companion file as a layer suffix does"`

	// ID is the companion's id, `{id}` standing for the entry's own:
	// `task/{id}/input`.
	ID string `json:"id" jsonschema:"description=The companion id with {id} standing for the entry own (task/{id}/input)"`
}

// The formats a kind's files may be read in.
const (
	SourceRaw         = "raw"
	SourceStructured  = "structured"
	SourceFrontmatter = "frontmatter"
	SourceDSL         = "dsl"
)

// The ways an entry's id is read from its path.
const (
	SourceIDBasename = "basename"
	SourceIDDir      = "dir"
	SourceIDPath     = "path"
	SourceIDFile     = "file"
	SourceIDName     = "name"
	SourceIDNameFile = "name-or-file"
)

// The parts of a file a layer may be taken from.
const (
	FromFile        = "file"
	FromFrontmatter = "frontmatter"
	FromBody        = "body"
	FromTree        = "tree"
	FromPath        = "path"
	FromSibling     = "sibling"
)

var (
	sourceFormats = map[string]bool{SourceRaw: true, SourceStructured: true, SourceFrontmatter: true, SourceDSL: true}
	sourceIDs     = map[string]bool{"": true, SourceIDBasename: true, SourceIDDir: true, SourceIDPath: true, SourceIDFile: true, SourceIDName: true, SourceIDNameFile: true}
	layerFroms    = map[string]bool{FromFile: true, FromFrontmatter: true, FromBody: true, FromTree: true, FromPath: true, FromSibling: true}
)

// KindRuleSourceMalformed: a kind's `source:` block cannot be read as said.
const KindRuleSourceMalformed = "kind-source-malformed"

// SourceFolder is the pack folder holding the kind's files: its source's
// folder, else its name. "" when the kind declares no source.
func (d KindDeclaration) SourceFolder() string {
	if d.Source == nil {
		return ""
	}
	if d.Source.Folder != "" {
		return d.Source.Folder
	}
	return d.Name
}

// sourceFindings reports what is wrong with the `source:` blocks of the view:
// each kind's own block, then a folder two kinds claim.
func (k Kinds) sourceFindings() []KindFinding {
	var out []KindFinding
	folders := map[string][]string{} // folder → the kinds claiming it
	for _, name := range k.Names() {
		d := k.byName[name]
		if d.Source == nil {
			continue
		}
		for _, msg := range d.sourceProblems(k) {
			out = append(out, KindFinding{Kind: name, URI: k.declaredBy[name], Message: "source: " + msg,
				Rule: KindRuleSourceMalformed, Severity: ValidateError})
		}
		f := d.SourceFolder()
		folders[f] = append(folders[f], name)
	}
	claimed := make([]string, 0, len(folders))
	for f := range folders {
		claimed = append(claimed, f)
	}
	sort.Strings(claimed)
	for _, f := range claimed {
		kinds := folders[f]
		if len(kinds) < 2 {
			continue
		}
		for _, name := range kinds {
			out = append(out, KindFinding{Kind: name, URI: k.declaredBy[name],
				Message: fmt.Sprintf("source: folder %q is claimed by %s; one folder, one kind", f, strings.Join(kinds, ", ")),
				Rule:    KindRuleSourceMalformed, Severity: ValidateError})
		}
	}
	return out
}

// sourceProblems lists what is wrong with this kind's own `source:` block.
func (d KindDeclaration) sourceProblems(k Kinds) []string {
	s := d.Source
	var out []string
	if !sourceFormats[s.Format] {
		out = append(out, fmt.Sprintf("format %q is not raw, structured, frontmatter or dsl", s.Format))
	}
	if !sourceIDs[s.ID] {
		out = append(out, fmt.Sprintf("id %q is not basename, dir, path, file, name or name-or-file", s.ID))
	}
	if s.ID == SourceIDName || s.ID == SourceIDNameFile {
		if _, ok := d.Layers[LayerData]; !ok {
			out = append(out, "id name reads the data layer's name, and the kind declares no data layer")
		}
	}
	if s.Claims != "" && (strings.ContainsAny(s.Claims, `/\*?[`) || !sourceMatches(s, s.Claims)) {
		out = append(out, fmt.Sprintf("claims %q is not one file name the kind's match accepts", s.Claims))
	}
	if len(s.Where) > 0 {
		if _, ok := d.Layers[LayerData]; !ok {
			out = append(out, "where reads the data layer, and the kind declares no data layer")
		}
	}
	for _, f := range s.Skip {
		if f == "" || strings.ContainsAny(f, `/\*?[`) || f == "." || f == ".." {
			out = append(out, fmt.Sprintf("skip %q is not one folder name", f))
		}
	}
	for _, f := range s.Former {
		if f == "" || strings.ContainsAny(f, `/\*?[`) || f == "." || f == ".." || f == d.SourceFolder() {
			out = append(out, fmt.Sprintf("former %q is not one folder name other than the kind's own", f))
		}
	}
	if strings.ContainsAny(s.Folder, `/\`) || s.Folder == "." || s.Folder == ".." {
		out = append(out, fmt.Sprintf("folder %q is not one folder name", s.Folder))
	}
	for _, g := range s.Match {
		if _, err := path.Match(g, ""); err != nil {
			out = append(out, fmt.Sprintf("match %q is not a glob: %v", g, err))
		}
	}
	_, hasData := d.Layers[LayerData]
	layers := make([]string, 0, len(s.Layers))
	for l := range s.Layers {
		layers = append(layers, l)
	}
	sort.Strings(layers)
	for _, l := range layers {
		ls := s.Layers[l]
		if _, declared := d.Layers[l]; !declared && ls.Fold == "" {
			out = append(out, fmt.Sprintf("layer %q is not declared by the kind (a part that only folds into data may be)", l))
		}
		if !layerFroms[ls.From] {
			out = append(out, fmt.Sprintf("layer %q: from %q is not file, frontmatter, body, tree, path or sibling", l, ls.From))
		}
		if ls.From == FromSibling && ls.Suffix == "" {
			out = append(out, fmt.Sprintf("layer %q: from sibling names no suffix", l))
		}
		if ls.From != FromSibling && ls.Suffix != "" {
			out = append(out, fmt.Sprintf("layer %q: a suffix is only for from sibling", l))
		}
		if (ls.From == FromFrontmatter || ls.From == FromBody) && s.Format != SourceFrontmatter {
			out = append(out, fmt.Sprintf("layer %q: from %s needs format frontmatter", l, ls.From))
		}
		if ls.From == FromTree && s.Format != SourceStructured && s.Format != SourceDSL {
			out = append(out, fmt.Sprintf("layer %q: from tree needs format structured or dsl", l))
		}
		if ls.From == FromPath && l != LayerData {
			out = append(out, fmt.Sprintf("layer %q: from path is the data layer's (the entry's {path})", l))
		}
		if ls.Fold != "" && (!hasData || l == LayerData) {
			out = append(out, fmt.Sprintf("layer %q: fold %q needs a data layer to fold into, other than itself", l, ls.Fold))
		}
	}
	for _, c := range s.Companions {
		if _, ok := k.byName[c.Kind]; !ok {
			out = append(out, fmt.Sprintf("companion kind %q is not declared", c.Kind))
		}
		if c.Suffix == "" {
			out = append(out, fmt.Sprintf("companion %q names no suffix", c.Kind))
		}
		if !strings.Contains(c.ID, "{id}") {
			out = append(out, fmt.Sprintf("companion %q: id %q does not name the entry's {id}", c.Kind, c.ID))
		}
	}
	return out
}

// ForFolder is the kind whose files sit in a pack's folder: the kind whose
// source names that folder, else the kind of that name (a kind with no
// source: block, read the generic way). ok is false when no kind claims it.
func (k Kinds) ForFolder(folder string) (KindDeclaration, bool) {
	for _, name := range k.Names() {
		if d := k.byName[name]; d.Source != nil && d.SourceFolder() == folder {
			return d, true
		}
	}
	d, ok := k.byName[folder]
	return d, ok
}

// sourceMatches says whether a file name matches one of the source's globs
// (any name when it names none).
func sourceMatches(s *SourceDecl, name string) bool {
	if len(s.Match) == 0 {
		return true
	}
	for _, g := range s.Match {
		if ok, _ := path.Match(g, name); ok {
			return true
		}
	}
	return false
}
