// SPDX-License-Identifier: Apache-2.0

package api

// Object query — the read side, answered from the index.
//
// These verbs never rescan the object files per call. A scan builds the index
// once; writes keep it current; reads are pure data (goals.md G5 — fast display,
// behaviour only on demand). Deleting the index loses nothing: one scan rebuilds
// it from the committed files, which remain the only source of truth.

// ObjectListInput selects objects from the index. Every filter is optional and
// they AND together; an empty input lists everything.
type ObjectListInput struct {
	Module string `json:"module,omitempty" jsonschema:"Only objects owned by this module"`
	Type   string `json:"type,omitempty"   jsonschema:"Only objects of this type"`
	Parent string `json:"parent,omitempty" jsonschema:"Only direct children of this object"`
	// Capability filters on the EFFECTIVE capabilities — type ∪ instance — so
	// an instance-added group is found the same way a type-declared one is.
	Capability string `json:"capability,omitempty" jsonschema:"Only objects whose effective capabilities (type ∪ instance) include this one"`
	// Limit cuts the list after that many objects, in id order; Total and
	// Truncated say it was cut. The service lists all at 0 or less - its Go
	// callers read whole lists; the agent-facing surfaces (the CLI command and
	// the cwm-object-list tool) list ObjectListSurfaceLimit when none is given
	// (a module's list runs to megabytes), and all at a negative limit.
	Limit int `json:"limit,omitempty" default:"50" jsonschema:"Maximum objects returned in id order; the command and the agent tool list 50 when it is not given; a negative limit lists all"`
	// Fields keeps only these parts of each object, the id and the type
	// always: a core field (name, description, parent, tags, edges, body...),
	// fields or capabilities whole, or a path into them (fields.<name>,
	// capabilities.<capability>, capabilities.<capability>.<field>).
	Fields []string `json:"fields,omitempty" jsonschema:"Keep only these parts of each object (id and type always): a core field such as name or parent - or fields.<name> or capabilities.<capability>[.<field>]; empty keeps all"`
}

// ObjectListSurfaceLimit is how many objects the agent-facing surfaces of
// ObjectList (the CLI command, the cwm-object-list tool) return when no limit
// is given: a module's whole list runs to megabytes, past an agent's context.
const ObjectListSurfaceLimit = 50

// ObjectSearchInput searches objects. Two modes:
//
//   - substring (the default): a case-insensitive substring over the RESIDENT
//     metadata — id, name, title, description, tags — sorted by id;
//   - fulltext: CWM's word index over every text of the object — every
//     capability field and the body — ranked (more of the query's words first,
//     then BM25), each hit naming the field it matched with a snippet.
//
// Every filter is optional and they AND together, in both modes.
type ObjectSearchInput struct {
	Query        string   `json:"query,omitempty"        jsonschema:"What to find: a substring (substring mode) or words (fulltext mode)"`
	Mode         string   `json:"mode,omitempty"         jsonschema:"description=substring (default) or fulltext,enum=substring|fulltext"`
	Within       string   `json:"within,omitempty"       jsonschema:"Only this object and everything under it in the tree"`
	Types        []string `json:"types,omitempty"        jsonschema:"Only these types"`
	Modules      []string `json:"modules,omitempty"      jsonschema:"Only these modules"`
	Capabilities []string `json:"capabilities,omitempty" jsonschema:"Only objects whose effective capabilities include ALL of these"`
	// Filter is the ObjectFilter a panel, a policy or a reference field
	// declares (types, capabilities — any of —, incoming, outgoing, when),
	// evaluated after the index's own filters and the query, on at most
	// SearchFilterCap candidates: a picker searching with it finds exactly
	// what that declaration admits.
	Filter *ObjectFilter `json:"filter,omitempty" jsonschema:"An ObjectFilter evaluated on the candidates the other filters and the query keep (at most 2000 of them)"`
	// States keeps the objects whose lifecycle state is one of these — the
	// state an object is in: the stored one, else its workflow's initial one
	// (an object that never moved is in it).
	States []string `json:"states,omitempty" jsonschema:"Only objects whose lifecycle state (stored; else the initial state of its workflow) is one of these"`
	// WithStates answers the lifecycle state of every returned object that
	// follows a workflow, in the output's States.
	WithStates bool `json:"with_states,omitempty" jsonschema:"Answer each returned object's lifecycle state and its colour in states"`
	Limit      int  `json:"limit,omitempty"       jsonschema:"Maximum results; 0 = no limit"`
}

// ObjectState is an object's lifecycle state as a list shows it: the state it
// is in (stored, else its workflow's initial one) and the colour the
// lifecycle capability's state badge gives it ("" when it names none).
type ObjectState struct {
	State string `json:"state"`
	Color string `json:"color,omitempty"`
}

// SearchFilterCap is how many candidates ObjectSearch evaluates a Filter on:
// past it the search stops and says so (Capped) — a `when` is an expression,
// and a search must answer at typing speed.
const SearchFilterCap = 2000

// Search modes.
const (
	SearchSubstring = "substring"
	SearchFullText  = "fulltext"
)

// ObjectSearchOutput returns the matches, resident form (no body): sorted by id
// in substring mode, by rank in fulltext mode — where Hits says why each one
// matched, in the same order.
type ObjectSearchOutput struct {
	Objects []Object          `json:"objects"`
	Hits    []ObjectSearchHit `json:"hits,omitempty" jsonschema:"Fulltext mode: why each object matched; same order as objects"`
	Total   int               `json:"total" jsonschema:"Matches before Limit was applied"`
	InError []string          `json:"in_error,omitempty" jsonschema:"The ids among objects that carry errors (ObjectGet / ObjectErrorList give them); qualified <key>:<module>.<local> when several projects are served"`
	// Capped says the Filter was evaluated on the first SearchFilterCap
	// candidates only: a narrower query finds the rest.
	Capped bool `json:"capped,omitempty" jsonschema:"The filter was evaluated on the first 2000 candidates only; narrow the query"`
	// States is, with WithStates, the lifecycle state of each returned object
	// that follows a workflow, by id - by the qualified id <key>:<module>.<local>
	// when several projects are served (Projects is set): two projects may
	// hold one bare id.
	States map[string]ObjectState `json:"states,omitempty" jsonschema:"With with_states: each returned object's lifecycle state by id; by the qualified id <key>:<module>.<local> when several projects are served (objects following no workflow are absent)"`
	// Projects names, when the universe serves several projects, the project
	// holding each object - same order as Objects.
	Projects []string `json:"projects,omitempty" jsonschema:"When several projects are served: the project key of each object; same order as objects"`
}

// ObjectSearchHit is one ranked fulltext match.
type ObjectSearchHit struct {
	ID      string   `json:"id"`
	Score   float64  `json:"score"             jsonschema:"BM25 score; higher is better among hits matching as many words"`
	Matched []string `json:"matched"           jsonschema:"The query words this object matched"`
	Field   string   `json:"field"             jsonschema:"The field that matched best: id or a capability field path such as body"`
	Snippet string   `json:"snippet,omitempty" jsonschema:"The matched text around the first matched word"`
}

// ObjectListOutput returns the matching objects.
type ObjectListOutput struct {
	Objects []Object `json:"objects"`
	Total   int      `json:"total" jsonschema:"Matching objects before the limit was applied"`
	// Truncated says the limit cut the list: Total says how many match.
	Truncated bool     `json:"truncated,omitempty" jsonschema:"The limit cut the list: total objects match; narrow with type or parent or raise the limit (negative = all)"`
	InError   []string `json:"in_error,omitempty" jsonschema:"The ids among objects that carry errors (ObjectGet / ObjectErrorList give them); qualified <key>:<module>.<local> when several projects are served"`
	// Projects names, when the universe serves several projects, the project
	// holding each object - same order as Objects.
	Projects []string `json:"projects,omitempty" jsonschema:"When several projects are served: the project key of each object; same order as objects"`
}

// ObjectTreeInput reads a containment subtree. Root empty ⇒ every root object
// (one with no parent) is returned.
type ObjectTreeInput struct {
	Root  string `json:"root,omitempty"  jsonschema:"Object id to start from; empty means all roots"`
	Depth int    `json:"depth,omitempty" jsonschema:"Max levels to descend; 0 or less means unlimited"`
	// DrawnOnly keeps only objects whose TYPE is drawn (`tree: root | node`).
	//
	// It is answered HERE, not by the caller, because "is this drawn?" is a
	// property of the type and the type registry lives on this side. A GUI
	// filtering by type NAME would be the type-name check the type's `tree:`
	// exists to replace — and would have to be edited every time a pack
	// contributes a type.
	//
	// The canonical case: `acceptance-criterion` is not drawn, because
	// 215 of them would bury a tree nobody navigates by criterion.
	DrawnOnly bool `json:"drawn_only,omitempty" jsonschema:"Only objects whose type is drawn (tree: root or node)"`
}

// ObjectTreeNode is one node of the returned tree. Children are nested, so the
// caller renders without a second call per object (G5).
type ObjectTreeNode struct {
	Object Object `json:"object"`
	// HasErrors marks an object that carries errors — the tree draws it red.
	HasErrors bool `json:"has_errors,omitempty"`
	// System marks a system module, or an object in one (the module carries
	// the system-module capability): a GUI shows it to experts only
	//
	System bool `json:"system,omitempty"`
	// Level is the level its type sits at - project or module, read from the
	// level capability the type carries (ObjectType.Level): a GUI
	// asks it, never a type's name. Empty below them.
	Level    string           `json:"level,omitempty"`
	Children []ObjectTreeNode `json:"children,omitempty"`
}

// ObjectTreeOutput returns the subtree(s).
type ObjectTreeOutput struct {
	Roots []ObjectTreeNode `json:"roots"`
	Total int              `json:"total" jsonschema:"Number of objects in the returned tree"`
}

// ObjectScanInput rebuilds the index from the object files on disk and reports
// what changed since the previous index. This is ObserveActual L0: the diff is
// what drives staleness and re-check (goals.md G4).
type ObjectScanInput struct {
	Force bool `json:"force,omitempty" jsonschema:"Rebuild from scratch, ignoring the current index"`
	// Validate checks every stored object as a write would, in error mode, and
	// reports the ones that are not valid.
	Validate bool `json:"validate,omitempty" jsonschema:"Also check every stored object (type, capability groups against their schemas, edges, lifecycle) and report the invalid ones"`
}

// InvalidObject is a stored object that is not valid, with why.
type InvalidObject struct {
	ID       string   `json:"id"`
	Path     string   `json:"path"`
	Problems []string `json:"problems"`
}

// ObjectScanOutput reports the diff. The three id lists are what a caller acts
// on; Total is the resulting index size.
type ObjectScanOutput struct {
	Added    []string `json:"added,omitempty"    jsonschema:"Object ids present on disk but not in the index"`
	Modified []string `json:"modified,omitempty" jsonschema:"Object ids whose file content hash changed"`
	Removed  []string `json:"removed,omitempty"  jsonschema:"Object ids in the index whose file is gone"`
	Total    int      `json:"total"              jsonschema:"Objects in the index after the scan"`
	// Invalid lists the stored objects that are not valid (with Validate).
	Invalid []InvalidObject `json:"invalid,omitempty" jsonschema:"Stored objects that are not valid, with the reasons (validate only)"`
	// Checked is how many objects the validation read (with Validate).
	Checked int `json:"checked,omitempty" jsonschema:"Objects checked (validate only)"`
}
