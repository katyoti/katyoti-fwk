// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// ObjectType declares what one `Object.Type` value MEANS — the Go side of the
// `object-type` contribution (decided 2026-09-18, D1/D2/D4).
//
// Before this, Type was a free string: ObjectCreate accepted anything while the
// 41 node/item declarations carrying the real meaning had no reader.
//
// THE CENTRAL FIELD IS Capabilities. A type is a NAMED CAPABILITY BUNDLE, so
// everything downstream reads the bundle rather than the name: the GUI selects
// panels by capability, and policies attach by capability instead of by a
// growing list of type names.
//
// IT IS ALSO THE UNION OF NodeType AND ItemTypeRegistration, and that is not
// incidental — merging two types means carrying both, not keeping the shorter
// one. Every field below arrived from a recorded disposition in
// object_type_coverage.go, which is what stops the cut from quietly deleting a
// surface the GUI already renders. Add a field here only with its ledger entry;
// the guard fails otherwise.
type ObjectType struct {
	// --- identity -----------------------------------------------------------

	Name  string `json:"name"                   yaml:"name"                   jsonschema:"The value that appears in an object's type field"`
	Label string `json:"label,omitempty"        yaml:"label,omitempty"        jsonschema:"Human name; Name is matched literally, Label is displayed"`
	// LabelPlural names a list of objects of the type — the tab of its
	// parent's pane that lists them when the type is not drawn in the tree
	// (Goals, Features). Empty = Label.
	LabelPlural string `json:"label_plural,omitempty" yaml:"label_plural,omitempty" jsonschema:"Human name of a list of objects of this type (the parent pane's tab); empty = label"`
	Description string `json:"description,omitempty"  yaml:"description,omitempty"`
	Icon        string `json:"icon,omitempty"         yaml:"icon,omitempty"`
	IconName    string `json:"icon_name,omitempty"    yaml:"icon_name,omitempty"    jsonschema:"Named icon for renderers that resolve by name rather than by glyph"`
	Abstract    bool   `json:"abstract,omitempty"     yaml:"abstract,omitempty"     jsonschema:"When true the type may not be instantiated"`

	// --- composition --------------------------------------------------------

	// Capabilities is THE list: every capability this type composes, by id.
	//
	// One list, one namespace. It used to be three — this one, `Aspects` (the
	// same concept at higher fidelity) and `Surfaces` (GUI panel bundles) — which
	// were the same kind of thing differing only in what is attached to the id.
	// Phase 1 proved that by declaring all four panel-only ids as capabilities:
	// `method-core` has a panel and no schema, `checkable` has a schema and a
	// check, and nothing stops one having both.
	//
	// What a capability MEANS — its schema, label, panels, actions, checks and
	// hooks — lives in the `capability` declaration, written once, instead of
	// being repeated inline on every type that composes it.
	Capabilities []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty" jsonschema:"The capability ids this type composes"`

	// Folder is where NEW objects of this type are written, relative to the
	// module. Empty = `objects`.
	//
	// A WRITE-SIDE DEFAULT, not a read rule: the `.cwm.yaml` suffix scan finds
	// objects anywhere, so nothing DEPENDS on the folder and the path stays
	// non-semantic. It exists so a `goal` type can declare `folder: goals` and
	// have its objects land together instead of being moved by hand.
	//
	// Changing it does NOT move existing objects, and it does not need to — they
	// are still found. Anyone "fixing" the resulting split by moving files is
	// doing cosmetics, not repair.
	//
	// A `_`-prefixed folder is REFUSED (see ValidateFolder): `_`-prefixed paths
	// are never scanned, so such a type would write objects that are invisible
	// from the moment they are created.
	Folder string `json:"folder,omitempty" yaml:"folder,omitempty" jsonschema:"Where new objects of this type are written, relative to the module; empty = objects"`

	// Runtime marks a type whose objects a RUN produces (approval-request,
	// records): they are filed in the project's CWM store
	// (`.katyoti/cwm/objects/`, gitignored, recreatable) rather than under the
	// modules folder, and indexed beside it (concepts.md §5, decided 2026-09-25).
	Runtime bool `json:"runtime,omitempty" yaml:"runtime,omitempty" jsonschema:"Objects of this type are produced by runs and live in the project's CWM store, not in the committed modules folder"`

	// Tree is where objects of this type show: "root" (drawn in the tree and may
	// sit at its top), "node" (drawn under a parent), "none" or empty (not drawn:
	// the objects are listed by the pane of their parent). A display fact of the
	// TYPE — an object does not carry it, so two objects of one type are never
	// drawn differently.
	//
	// Deliberately NOT derived from `accept_parents` / `allowed_children`: those
	// are VALIDATION rules about what may be created, and using them for display
	// would make "declared nothing" mean the same as "deliberately a leaf".
	Tree string `json:"tree,omitempty" yaml:"tree,omitempty" jsonschema:"Where objects of this type show: root (drawn and may sit at the top) | node (drawn under a parent) | none (listed by the parent's pane); empty = none"`

	// Load is the ONE override of the load policy: "with-tree" | "on-demand" |
	// "never-eager". Empty = derived from Tree — a drawn type comes with the
	// tree, anything else is read on demand.
	Load string `json:"load,omitempty" yaml:"load,omitempty" jsonschema:"Load policy override: with-tree | on-demand | never-eager; empty = derived from tree (a drawn type loads with the tree)"`

	// IDPrefix is the readable id scheme for objects created with a title and
	// no id (phase 7c): `<id_prefix>-<NNN>`, zero-padded, next in the module.
	// A type with none gets a UUIDv7 — the runtime kinds created concurrently,
	// where a sequence would need a lock.
	IDPrefix string `json:"id_prefix,omitempty" yaml:"id_prefix,omitempty" jsonschema:"Readable id prefix for minted ids (OD → OD-001); empty = UUIDv7"`

	// Fields is the data-type of the type's own fields (Object.Fields) — what
	// this kind of thing is, beyond the core every object has. NodeType's
	// `type_specific_data_type` and ItemTypeRegistration's `schema_data_type`
	// were always this. Empty = the type has no fields of its own.
	Fields string `json:"fields,omitempty" yaml:"fields,omitempty" jsonschema:"Data-type URI of the type's own fields; empty = none"`

	// Body says what this type's body is. CWM stores the body the same way
	// for every type — the markdown half of a .cwm.md file, indexed, never
	// parsed for data — but what it MEANS is the type's: its name (Slides,
	// Reasoning), its format, the panel that shows it. Shown and edited with
	// the type's own fields. Nil = "Text", markdown.
	Body *BodyDecl `json:"body,omitempty" yaml:"body,omitempty" jsonschema:"What this type's body is: its label and format; empty = Text in markdown"`

	// --- containment --------------------------------------------------------

	// AllowedChildren and AcceptParents are the two SIDES of one question and
	// the effective rule is their UNION (cwm-v3 §2.1). NodeType's
	// `allowed_item_types` folds into AllowedChildren, because an item is an
	// object now — that fold must be a union at migration time, or the 10 types
	// declaring both lose their node children.
	AllowedChildren []string `json:"allowed_children,omitempty" yaml:"allowed_children,omitempty" jsonschema:"Type names allowed as children; empty = no restriction. Effective compatibility is the UNION with the child's accept_parents"`
	AcceptParents   []string `json:"accept_parents,omitempty"   yaml:"accept_parents,omitempty"   jsonschema:"Parent type names that may host this type; '*' = any; empty = no restriction from this side"`

	// AcceptWhen is a condition over the parent - an el predicate in the
	// object vocabulary, `current` being the parent - under which the create
	// picker offers this type: `type!= 'module' or 'planned' in
	// current.capabilities` keeps planning types off a study. A create is not
	// refused by it; empty = offered wherever the names allow.
	AcceptWhen string `json:"accept_when,omitempty" yaml:"accept_when,omitempty" jsonschema:"A condition over the parent (an el predicate - current is the parent) under which the create picker offers this type; empty = wherever the names allow"`

	DescriptionMaxChars int `json:"description_max_chars,omitempty" yaml:"description_max_chars,omitempty" jsonschema:"Override the default description budget; 0 = use default"`

	// --- runnables ----------------------------------------------------------

	// Actions is []ActionRef, never []string. A bare URI still decodes (ActionRef
	// accepts a scalar), but the struct form is what carries label, icon,
	// confirm, when, target, session, bind and pre-bound input — all of which a
	// []string would have dropped from every binding site without a word.
	Actions []ActionRef `json:"actions,omitempty" yaml:"actions,omitempty" jsonschema:"Actions every object of this type can run; bare URI string or {uri, label, icon, confirm, when, input}"`

	// Files are named files every object of this type has (2026-09-26) --
	// between a capability's files and the object's own, closer wins.
	Files map[string]NamedFile `json:"files,omitempty" yaml:"files,omitempty" jsonschema:"Named files every object of this type has; each a module-relative path"`

	// Workflow names the `workflow` an object of this type moves through; an
	// object may override it in its lifecycle. Empty = the type has no lifecycle.
	Workflow string `json:"workflow,omitempty" yaml:"workflow,omitempty" jsonschema:"The workflow objects of this type move through"`

	// Hooks are hook entries every object of this type has, by point. Closer
	// than a capability's, farther than an object's own.
	Hooks Hooks `json:"hooks,omitempty" yaml:"hooks,omitempty" jsonschema:"Hook entries by point for every object of this type"`

	// Agent is what a chat on every object of this type receives. Closer
	// than a capability's, farther than an object's own.
	Agent *AgentDecl `json:"agent,omitempty" yaml:"agent,omitempty" jsonschema:"Agent-context declaration for every object of this type"`

	// --- creation ---------------------------------------------------------

	// CreateTask REPLACES the default create form with a guided wizard that
	// calls create itself. It is not a hook: a check that may refuse the create
	// is a `before_create` entry in Hooks.
	CreateTask string `json:"create_task,omitempty" yaml:"create_task,omitempty" jsonschema:"Task/action URI run INSTEAD of the default create form — a guided wizard that calls create itself"`

	// --- display (what tells the types apart) ------------------------------

	// Color is the type's tone in the tree, the lists and a pane's header: a
	// named palette colour (indigo, teal…) or #rgb / #rrggbb. Empty = the
	// renderer's neutral tone.
	Color string `json:"color,omitempty" yaml:"color,omitempty" jsonschema:"The type's tone in the tree and lists: a named palette colour or #rgb / #rrggbb; empty = neutral"`

	// ChildOrder is the order the tree groups a project's or a module's
	// children by type in: the type names first to last; a child type not
	// named comes after, in name order. Within allowed_children when that is
	// set.
	ChildOrder []string `json:"child_order,omitempty" yaml:"child_order,omitempty" jsonschema:"Child type names in the order the tree groups them; one not named comes after"`

	// Solution is the solution a project of this type needs (a project type):
	// opening it in a universe without that solution says so, naming it.
	Solution string `json:"solution,omitempty" yaml:"solution,omitempty" jsonschema:"The solution a project of this type needs; empty = none"`

	// Opens is how a project of this type opens (a project type): the action
	// URI run to open it; empty = it opens as any project does.
	Opens string `json:"opens,omitempty" yaml:"opens,omitempty" jsonschema:"The action URI that opens a project of this type; empty = the ordinary opening"`

	// --- storage --------------------------------------------------------------

	Side         string `json:"side,omitempty"              yaml:"side,omitempty"              jsonschema:"Informational tag: requirements | implementation | delivery | both | root"`
	MaxSizeBytes int    `json:"max_size_bytes,omitempty"    yaml:"max_size_bytes,omitempty"    jsonschema:"Per-write size budget; enforced at the write path"`

	// --- sections ---------------------------------------------------------

	// Sections are the type's settings that a capability owns, by capability
	// name: every top-level key of the declaration that is not a field above
	// (`context:`, `pane:`, `verification:`). Writing one puts its capability
	// in force on the type. CWM keeps each as written and checks it against
	// its capability's `type_schema`; the capability's extension reads it with
	// TypeSection. Inline on the wire and in the file — see type_sections.go.
	Sections map[string]map[string]any `json:"-" yaml:"-"`

	// Provider is the binary that declared the type — its OWNER: a gate entry
	// it contributes applies to the type's objects (ScopeOf). Set when the
	// catalog is read; never authored, never on the wire.
	Provider string `json:"-" yaml:"-"`
}

// Tree values of an object type.
const (
	TreeRoot = "root"
	TreeNode = "node"
	TreeNone = "none"
)

// Drawn reports whether objects of this type are drawn in the tree.
func (t ObjectType) Drawn() bool { return t.Tree == TreeRoot || t.Tree == TreeNode }

// Level is the level a type sits at - project or module - read from the
// level capability it carries, never from its name: any type
// carrying `project` is a project, any carrying `module` a module. Empty for
// an object type below them.
func (t ObjectType) Level() string {
	switch {
	case slices.Contains(t.Capabilities, CapProject):
		return CapProject
	case slices.Contains(t.Capabilities, CapModule):
		return CapModule
	}
	return ""
}

// DefaultTypeIcon is the icon of a type that declares none.
const DefaultTypeIcon = "file"

// DisplayIcon is the icon the type is drawn with: its own, else the default.
func (t ObjectType) DisplayIcon() string {
	if t.Icon != "" {
		return t.Icon
	}
	return DefaultTypeIcon
}

// ChildRank is where a child type comes in the tree's grouping: its place in
// ChildOrder, or after every named one.
func (t ObjectType) ChildRank(child string) int {
	for i, c := range t.ChildOrder {
		if c == child {
			return i
		}
	}
	return len(t.ChildOrder)
}

var typeColor = regexp.MustCompile(`^(#[0-9a-fA-F]{3}|#[0-9a-fA-F]{6}|[a-z][a-z0-9-]*)$`)

// ValidateDisplay refuses a declaration whose display facts cannot be drawn:
// a colour that is neither a palette name nor #rgb / #rrggbb, a child order
// naming a type twice or one its allowed_children leaves out.
func (t ObjectType) ValidateDisplay() error {
	if t.Color != "" && !typeColor.MatchString(t.Color) {
		return fmt.Errorf("%w: object-type %q declares colour %q: a palette name or #rgb / #rrggbb", ErrInvalidInput, t.Name, t.Color)
	}
	seen := map[string]bool{}
	for _, c := range t.ChildOrder {
		if seen[c] {
			return fmt.Errorf("%w: object-type %q names %q twice in child_order", ErrInvalidInput, t.Name, c)
		}
		seen[c] = true
		if len(t.AllowedChildren) > 0 && !slices.Contains(t.AllowedChildren, c) {
			return fmt.Errorf("%w: object-type %q orders child %q its allowed_children leaves out", ErrInvalidInput, t.Name, c)
		}
	}
	return nil
}

// IsParentAllowed reports whether an object of this type may sit under a
// parent of the given type, from the CHILD side only: an accept_parents entry
// names the parent's type, a capability its type carries - a level (module,
// project, system-module), never a type's name standing for it -
// or '*', any parent. An undeclared parent type is passed by its name alone.
//
// Empty AcceptParents means "no restriction from this side", NOT "nothing
// allowed" — the effective rule is the UNION with the parent's AllowedChildren
// (cwm-v3 §2.1), so a type that declares neither is unconstrained. Reading an
// empty list as a denial would refuse every write in a workspace that has not
// declared containment.
func (t *ObjectType) IsParentAllowed(parent ObjectType) bool {
	if t == nil || len(t.AcceptParents) == 0 {
		return true
	}
	for _, p := range t.AcceptParents {
		if p == "*" || p == parent.Name || slices.Contains(parent.Capabilities, p) {
			return true
		}
	}
	return false
}

// NamesParentType reports whether an accept_parents entry names this parent's
// type itself. '*' and a capability name a class of parents, not this one: a
// parent curating its children offers its list plus only the types naming its
// type (the create picker) - a system module, which carries the
// module level, is not offered every type accepting any module.
func (t *ObjectType) NamesParentType(parentType string) bool {
	return t != nil && slices.Contains(t.AcceptParents, parentType)
}

// AllowsChild reports whether this type admits a child of the given type, from
// the PARENT side only. Empty AllowedChildren means no restriction.
func (t *ObjectType) AllowsChild(childTypeName string) bool {
	if t == nil || len(t.AllowedChildren) == 0 {
		return true
	}
	for _, c := range t.AllowedChildren {
		if c == "*" || c == childTypeName {
			return true
		}
	}
	return false
}

// Type-validation modes (D5). The migration runs at Warn; the flip to Error is
// the migration's terminal condition and MUST actually happen — left at Warn
// forever, the validation is decorative and an undeclared type stays as silent
// as it was before.
const (
	// TypeValidationOff — no checking. Also the effective mode when no
	// object-types are declared at all, so a workspace that has not migrated
	// behaves exactly as before rather than refusing everything.
	TypeValidationOff = "off"
	// TypeValidationWarn — an undeclared type is logged and accepted.
	TypeValidationWarn = "warn"
	// TypeValidationError — an undeclared type is refused.
	TypeValidationError = "error"
)

// ObjectsFolder returns where objects of this type are written.
func (t ObjectType) ObjectsFolder() string {
	if t.Folder == "" {
		return "objects"
	}
	return t.Folder
}

// PlanningFolder is a module's planning space — `modules/<m>/.ktw/`, where its
// plans and planned todos live, committed with the module. It is the ONE hidden
// folder the object index admits, and only directly under a module; every
// other `.`- or `_`-prefixed folder stays invisible.
const PlanningFolder = ".ktw"

// ValidateFolder rejects a storage folder that would hide its own objects.
//
// A folder is ONE segment, with one exception: a type filed in the planning
// space declares `.ktw` or `.ktw/<segment>` (`.ktw/plans`).
//
// A `_`-prefixed folder is never scanned — that is how `_generated`, `_archive`
// and `_backup` are excluded with one convention. A type declaring one would
// write objects nothing can ever find, so this is refused at DECLARATION rather
// than discovered when an object goes missing.
func (t ObjectType) ValidateFolder() error {
	f := t.Folder
	if f == "" {
		return nil
	}
	if err := CheckFolder(f); err != nil {
		return fmt.Errorf("%w: object-type %q declares folder %q: %v", ErrInvalidInput, t.Name, f, err)
	}
	return nil
}

// CheckFolder is the storage-folder rule, shared by the declaration check and
// the path resolution so the two can never disagree: one visible segment, or
// the planning space followed by at most one visible segment.
func CheckFolder(f string) error {
	rest := f
	if f == PlanningFolder {
		return nil
	}
	if strings.HasPrefix(f, PlanningFolder+"/") {
		rest = strings.TrimPrefix(f, PlanningFolder+"/")
	}
	switch {
	case rest == "":
		return fmt.Errorf("an empty segment")
	case strings.ContainsAny(rest, `/\`), strings.Contains(rest, ".."):
		return fmt.Errorf("a storage folder is ONE segment (or %s/<segment>), not a path", PlanningFolder)
	case strings.HasPrefix(rest, "_"), strings.HasPrefix(rest, "."):
		return fmt.Errorf("a `_`- or `.`-prefixed folder is never scanned, so its objects would be invisible from the moment they are created")
	}
	return nil
}

// Body formats a type may declare.
const (
	BodyMarkdown = "markdown"
	BodyText     = "text"
	BodyHTML     = "html"
)

// BodyDecl is what a type says its body is.
type BodyDecl struct {
	Label       string `json:"label,omitempty"       yaml:"label,omitempty"       jsonschema:"What the body is called on this type (Slides, Reasoning); empty = Text"`
	Description string `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"What the body holds"`
	Format      string `json:"format,omitempty"      yaml:"format,omitempty"      jsonschema:"markdown (default), text, html, or a format a panel owns (deck)"`
	Panel       string `json:"panel,omitempty"       yaml:"panel,omitempty"       jsonschema:"The panel that shows the body, when one does (deck); the type's fields section then names it instead of showing the text"`
}

// BodyOf is the type's body declaration, with its defaults filled.
func (t ObjectType) BodyOf() BodyDecl {
	var d BodyDecl
	if t.Body != nil {
		d = *t.Body
	}
	if d.Label == "" {
		d.Label = "Text"
	}
	if d.Format == "" {
		d.Format = BodyMarkdown
	}
	return d
}
