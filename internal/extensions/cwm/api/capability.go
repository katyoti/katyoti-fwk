// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"errors"
	"strings"
)

// Capability declares what ONE capability id means — the single new contribution
// kind in the capability model (`modules/cwm/capability-model.md`, 2026-09-19).
//
// Before this, a capability id had no declaration at all. Its meaning was
// scattered across three places, none of which were a statement:
//
//   - its SCHEMA was found by the `cap-<id>.dsl.yaml` filename convention, which
//     NO Go code ever implemented — only the probe did
//   - its PANEL by an unwritten id match between a node type's `capabilities:`
//     list and a `capability-panel` contribution;
//   - its BEHAVIOUR by inline `CapabilityRef` fields repeated on every type.
//
// CAPABILITY ID IS NOT SCHEMA. A capability is a NAMED FIELD-GROUP CONFORMING
// TO A SCHEMA, and the schema is the reusable part: expect dozens of
// capabilities over a handful of schemas, the id being the storage key
// (`obj.Capability(id)`) — two checkable capabilities on one object would
// otherwise collide on one field-group.
//
// EVERY SLOT IS OPTIONAL. A capability declaring none of them is a TAG, which is
// legitimate: "everything marked `deprecated`" is a useful thing to select on.
type Capability struct {
	// Name is the id — and the STORAGE KEY under which an object holds this
	// capability's field-group. Lowercase kebab, matched literally.
	Name string `json:"name" yaml:"name" jsonschema:"The capability id; also the key of its field-group on an object"`

	Label       string `json:"label,omitempty"       yaml:"label,omitempty"       jsonschema:"Section header; defaults to the name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Icon is how the capability shows among an object's capabilities (a
	// badge with this icon, its description as the tooltip).
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty" jsonschema:"An icon name for the capability's badge"`

	// Schema is a data-type URI describing this capability's field-group.
	//
	// It is deliberately a REFERENCE, not an inline shape: a "field-set" is not a
	// new artefact, it is a `data-type`, which already exists as a contribution
	// kind, already carries per-field label/default/description, and already
	// drives form rendering through SchemaService.GetSchemaAST.
	//
	// Empty = this capability contributes no fields (panel-only, or a tag).
	Schema string `json:"schema,omitempty" yaml:"schema,omitempty" jsonschema:"data-type URI for this capability's field-group; empty = no fields"`

	// TypeSchema is a data-type URI describing this capability's SECTION on a
	// type: the settings a type writes, under the capability's name, to set it
	// (`context:` on a type sets the object-context capability). Writing the
	// section puts the capability in force on the type. CWM keeps the section
	// as written and checks it against this schema; the extension that owns
	// the capability reads it (TypeSection) — CWM gives it no meaning.
	//
	// A capability with a TypeSchema and no Schema is TYPE-LEVEL: it carries no
	// field-group on objects and is not listed among an object's capabilities.
	TypeSchema string `json:"type_schema,omitempty" yaml:"type_schema,omitempty" jsonschema:"data-type URI for the section a type writes under this capability's name; empty = no section"`

	// Files are the NAMED FILES this capability manages, by name: a
	// module-relative path, `${...}` allowed over the object. Read and written
	// through ObjectNamedFile*, never by a caller composing the path.
	Files map[string]NamedFile `json:"files,omitempty" yaml:"files,omitempty" jsonschema:"Named files this capability manages; each a module-relative path"`

	// Actions every object carrying this capability can run. A type may still
	// bind an action inline — that binding is the curated one and wins.
	Actions []ActionRef `json:"actions,omitempty" yaml:"actions,omitempty" jsonschema:"Actions offered by objects carrying this capability"`

	// Hooks are entries by point for every object carrying this capability —
	// the `capability:<name>` layer of the object's hook table, between the
	// `hook` contributions and the policies. A `before_*` entry answers
	// {accept, reason} (HookAccept) and may refuse the write.
	Hooks Hooks `json:"hooks,omitempty" yaml:"hooks,omitempty" jsonschema:"Hook entries by point (before_create, on_update, lint, verify, bootstrap, …) for objects carrying this capability"`

	// Agent is what a chat on an object carrying this capability receives.
	Agent *AgentDecl `json:"agent,omitempty" yaml:"agent,omitempty" jsonschema:"Agent-context declaration for objects carrying this capability"`

	// Attach puts this capability on every object its filter selects without
	// any of them declaring it (the memory every object has). A RULE put on
	// objects is not a capability but a Policy.
	//
	// The capability's AUTHOR declares who carries it, so the selector belongs
	// on the declaration.
	//
	// ATTACHMENT READS THE OBJECT'S SHAPE, and that is a rule rather than a
	// limitation (ShapeFilter): a filter over an object's own data would make
	// its shape depend on its content — circular, and unvalidatable.
	// Instance-level variation belongs on a panel or an action as a `when`
	// predicate, evaluated at render. Its `capabilities` are matched against
	// what the object carries BEFORE attachment (type, aspects, its own
	// groups): an attached capability never attaches another.
	Attach *ShapeFilter `json:"attach,omitempty" yaml:"attach,omitempty" jsonschema:"Puts this capability on every object it selects (types and capabilities only)"`

	// Badges are header chips read from this capability's field-group. They live
	// here rather than on the type because the capability OWNS the key a badge
	// reads — a badge over `status` belongs to whatever declares `status`.
	Badges []BadgeDef `json:"badges,omitempty" yaml:"badges,omitempty" jsonschema:"Header chips over this capability's fields"`

	// Uses names the capabilities whose fields this one reads. Giving it to an
	// object requires them; removing one of them is refused while this one is
	// carried.
	Uses []string `json:"uses,omitempty" yaml:"uses,omitempty" jsonschema:"Capabilities whose fields this one reads"`

	// Workflow makes this capability's own state follow a workflow — beside
	// the object's own lifecycle, never instead of it.
	Workflow *CapabilityWorkflow `json:"workflow,omitempty" yaml:"workflow,omitempty" jsonschema:"The workflow this capability's own state follows"`

	// Functions are what this capability adds to the expression language:
	// read-only questions its service answers about an object (`gate('ready')`),
	// usable in guards, gate checks, action `when:` and `${…}` inputs, panel
	// filters and views — never in a hook's filter or `when`.
	// CWM calls them and never knows what they mean.
	Functions []CapabilityFunction `json:"functions,omitempty" yaml:"functions,omitempty" jsonschema:"Read-only functions this capability adds to expressions over an object"`

	// Provider is the binary that declared the capability — its OWNER: a gate
	// entry it contributes applies to an object carrying the capability by
	// the object's own choice (ScopeOf). Set when the catalog is read; never
	// authored, never on the wire.
	Provider string `json:"-" yaml:"-"`
}

// Function result kinds a declaration may name.
const (
	FunctionReturnsBoolean = "boolean"
	FunctionReturnsNumber  = "number"
	FunctionReturnsString  = "string"
	FunctionReturnsObject  = "object"
	FunctionReturnsList    = "list"
)

// CapabilityFunction is one function a capability adds to expressions. Calling
// it runs Action — a READ-ONLY method, unless the function is Costly — through
// the ActionService with {object, args}; the answer is the function's value.
type CapabilityFunction struct {
	Name        string   `json:"name"                  yaml:"name"                  jsonschema:"The name expressions call; a lowercase identifier unique in the universe"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"What the function answers"`
	Action      string   `json:"action"                yaml:"action"                jsonschema:"The read-only method that answers it (an action URI)"`
	Args        []string `json:"args,omitempty"        yaml:"args,omitempty"        jsonschema:"The arguments by name; each passed as given"`
	Returns     string   `json:"returns,omitempty"     yaml:"returns,omitempty"     jsonschema:"boolean or number or string or object or list; empty = any"`
	Timeout     string   `json:"timeout,omitempty"     yaml:"timeout,omitempty"     jsonschema:"How long a call may take (a Go duration); 5s when empty"`
	// Costly marks a function too heavy for a preview — a pane, an action
	// list, a list of possible moves: there CWM does not run it and the call
	// answers ErrDecidedAtMove. A costly function may record what it finds
	// (verify() records verdicts), so its action need not be read-only.
	Costly bool `json:"costly,omitempty" yaml:"costly,omitempty" jsonschema:"Too heavy for a preview: a pane or a list of possible moves does not run it and leaves it to the move itself"`
}

// FunctionCall is what a function's action receives. In a guard, By / From /
// To say who would move the object and between which states. NoCostly says
// the call belongs to a preview: a function that evaluates expressions of its
// own (gate()) evaluates them with ObjectEvaluateInput.NoCostly.
type FunctionCall struct {
	Function string `json:"function"`
	Object   string `json:"object"`
	Args     []any  `json:"args,omitempty"`
	By       *Actor `json:"by,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	NoCostly bool   `json:"no_costly,omitempty"`
}

// FunctionAnswer is what one call of an evaluation answered: the value, or
// why it could not answer. A refusal carries them, so its reader sees what
// the service said without asking again. AtMove marks a call a preview did
// not make — a costly function, or one whose own evaluation met one —
// undecided now, decided at the move.
type FunctionAnswer struct {
	Function string `json:"function"`
	Args     []any  `json:"args,omitempty"`
	Answer   any    `json:"answer,omitempty"`
	Error    string `json:"error,omitempty"`
	AtMove   bool   `json:"at_move,omitempty"`
}

// ErrDecidedAtMove is what a costly function answers in a preview: it was not
// run, and the expression calling it is decided when the move is made.
var ErrDecidedAtMove = errors.New("costly: not run for a preview, decided at the move")

// IsDecidedAtMove reports an error — or the text of one, as it crosses a
// process — that is ErrDecidedAtMove.
func IsDecidedAtMove(err error) bool {
	return err != nil && (errors.Is(err, ErrDecidedAtMove) || strings.Contains(err.Error(), ErrDecidedAtMove.Error()))
}

// AnyAtMove reports an answer among calls left to the move.
func AnyAtMove(calls []FunctionAnswer) bool {
	for _, c := range calls {
		if c.AtMove {
			return true
		}
	}
	return false
}

// AnswerOf finds the answer of the call `function(args...)` among answers.
func AnswerOf(answers []FunctionAnswer, function string, args ...any) (FunctionAnswer, bool) {
	key := func(a []any) string {
		if a == nil {
			a = []any{}
		}
		b, _ := json.Marshal(a)
		return string(b)
	}
	want := key(args)
	for _, a := range answers {
		if a.Function == function && key(a.Args) == want {
			return a, true
		}
	}
	return FunctionAnswer{}, false
}

// CapabilityWorkflow is a capability's workflow and the fields of its group
// that hold the state and the history. The capability names them: it knows
// what its fields mean.
type CapabilityWorkflow struct {
	Name    string `json:"name"              yaml:"name"              jsonschema:"The workflow contribution's name"`
	State   string `json:"state"             yaml:"state"             jsonschema:"The field of this capability's group that holds the state"`
	History string `json:"history,omitempty" yaml:"history,omitempty" jsonschema:"The field that holds the transition history; empty keeps none"`
}

// IsTypeLevel reports a capability that only a type sets: a section on the
// type and no field-group on objects.
func (c Capability) IsTypeLevel() bool { return c.TypeSchema != "" && c.Schema == "" }

// IsAttached reports a capability that attaches itself to types rather than
// waiting to be declared by one.
func (c Capability) IsAttached() bool {
	return c.Attach != nil && !c.Attach.IsZero()
}

// AttachesTo reports whether this capability attaches itself to an object of
// type typ carrying caps before attachment, and when not, why.
func (c Capability) AttachesTo(typ string, caps []string) (bool, string) {
	if !c.IsAttached() {
		return false, "attaches to nothing"
	}
	return c.Attach.Matches(FilterSubject{Type: typ, Capabilities: caps})
}

// AsRef renders this capability as the CapabilityRef the resolver returns.
func (c Capability) AsRef() CapabilityRef {
	return CapabilityRef{ID: c.Name, Schema: c.Schema, Label: c.Label, Verify: len(c.Hooks[HookVerify]) > 0}
}
