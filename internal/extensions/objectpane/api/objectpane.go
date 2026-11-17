// SPDX-License-Identifier: Apache-2.0

// Package api is the object-pane service contract: what a person sees of ONE
// object and what they may do with it, composed from CWM's reads. It stores
// nothing and decides nothing — CWM keeps the objects and guards every move;
// the pane asks.
//
// Detail is the pane in one round trip (detail.go).
//
// TransitionOptions answers where an object may move: every lifecycle it has,
// the transitions from each current state, each judged for `by` by CWM's own
// dry-run of the move.
package api

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ObjectPaneServiceDef describes the service for KTSP/CLI auto-generation.
var ObjectPaneServiceDef = meta.ServiceDef{
	Name:    "object-pane",
	Short:   "One object as a person sees it: the detail pane, where it may move and what stands in the way",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Detail":            {Short: "The detail pane in one round trip: the object, its type, its effective capabilities, the tab strip, aspects, badges, policies, links and children by type", ReadOnly: true},
		"TransitionOptions": {Short: "Where an object may move: per lifecycle, the transitions from its state, each judged for who would move by CWM's dry-run", ReadOnly: true},
		"GlobalPanels":      {Short: "The panels of the Global strip: the declared panels marked global, by order then name — each called there with a target (every module, a module, a release)", ReadOnly: true},
		"ResolvePanel":      {Short: "What a panel draws made self-contained: every schema named by data-type URI or by an action input ({action: uri}) replaced by its AST, every ui block laid over it, the fields options computed", ReadOnly: true},
		"ResolvePart":       {Short: "One specification part by its reference: the schema a component of a kind and variant draws (a data-type URI or an action input), resolved as ResolvePanel does, with its hash - nothing but the hash when the one held is current", ReadOnly: true},
		"Contents":          {Short: "Everything under an object (a module's whole subtree; a project's whole index) as a page: by type, each object with its state and the object it sits under, filtered by type, state and text", ReadOnly: true},
		"AttachedFiles":     {Short: "The files an object attaches (its attaches edges, paths in its module): name, folder, size, media type, a missing file flagged — the Files panel's source", ReadOnly: true},
		"Search":            {Short: "The objects of the universe (or of one module) an ObjectFilter, states and text keep, as a page: the panel's saved views, a search form, the results by type", ReadOnly: true},
		"Fixture":           {Short: "A panel's dummy data: the JSON reference <fixture>.<state>.json an extension ships, for the state asked (or read from the object); nothing when the state or the file is unknown - a mock-up's source until the real one exists", ReadOnly: true},
	},
}

// ObjectPaneService composes the pane's answers over CWM.
type ObjectPaneService interface {
	Detail(ctx context.Context, in DetailInput) (DetailOutput, error)
	TransitionOptions(ctx context.Context, in TransitionOptionsInput) (TransitionOptionsOutput, error)
	GlobalPanels(ctx context.Context, in GlobalPanelsInput) (GlobalPanelsOutput, error)
	ResolvePanel(ctx context.Context, in ResolvePanelInput) (ResolvePanelOutput, error)
	// ResolvePart answers one specification part by its reference and hash:
	// what a host whose cache misses asks.
	ResolvePart(ctx context.Context, in ResolvePartInput) (ResolvePartOutput, error)
	// Contents is the Contents panel's source: what an object holds.
	Contents(ctx context.Context, in ContentsInput) (render.Def, error)
	// Search is the Objects panel's source: a search across the universe.
	Search(ctx context.Context, in SearchInput) (render.Def, error)
	// AttachedFiles is the Files panel's source: the files an object's
	// attaches edges name, in its module.
	AttachedFiles(ctx context.Context, in AttachedFilesInput) (AttachedFilesOutput, error)
	// Fixture is the fixture source: a panel's dummy data read from a JSON
	// file its extension ships for a named state, any application's.
	Fixture(ctx context.Context, in FixtureInput) (json.RawMessage, error)
}

// FixtureInput is what the fixture source is called with: the object (or the
// target) and the language as for any source, and under them the panel's
// `input` naming the fixture and how its state is chosen. The file read is
// the reference `<fixture>.<state>.json`; the state is State, else the
// object's value at StateField, else `default`.
type FixtureInput struct {
	ID         string `json:"id,omitempty"          jsonschema:"The object the panel shows; needed only with state_field"`
	Target     string `json:"target,omitempty"      jsonschema:"The Global strip's target; unused"`
	Language   string `json:"language,omitempty"    jsonschema:"The person's language; unused"`
	Fixture    string `json:"fixture"               jsonschema:"The fixture's reference URI without its state: <provider>:/reference/<path>/<panel>; required"`
	State      string `json:"state,omitempty"       jsonschema:"The state whose file to read (letters and digits and - or _); wins over state_field"`
	StateField string `json:"state_field,omitempty" jsonschema:"A dotted path into the object (fields.mock or capabilities.<cap>.mock) whose value is the state"`
}

// AttachedFilesInput names the object, and the file to show first (a page
// navigating to the Files panel with a query sets it).
type AttachedFilesInput struct {
	ID       string `json:"id"                 jsonschema:"Object id; required"`
	Language string `json:"language,omitempty" jsonschema:"The person's language (en or fr); empty = English"`
	File     string `json:"file,omitempty"     jsonschema:"The file to show first: module-relative or as the edge stores it (project-relative)"`
}

// AttachedFile is one file an object attaches.
type AttachedFile struct {
	// Path is module-relative: what ObjectFileRead (and the GUI's document
	// source) takes with the object's id.
	Path        string `json:"path"`
	ProjectPath string `json:"project_path" jsonschema:"The edge's target, relative to the project"`
	Name        string `json:"name"`
	Folder      string `json:"folder"       jsonschema:"The module-relative folder; empty at the module root"`
	SizeBytes   int64  `json:"size_bytes"`
	MediaType   string `json:"media_type"   jsonschema:"From the file extension; the display engine shows it by this type"`
	Kind        string `json:"kind"         jsonschema:"The edge kind (attaches) or named for a named file"`
	// Named is the file's name when it is one of the object's own named
	// files, not an edge's target.
	Named   string `json:"named,omitempty" jsonschema:"The named file's name; empty for an edge's file"`
	Missing bool   `json:"missing,omitempty" jsonschema:"The edge names a file that is not there"`
	// Outside is set when the edge names a file outside the object's module:
	// listed, never read through the module's file API.
	Outside bool `json:"outside,omitempty"`
}

// NamedFileKind is AttachedFile.Kind for a named file.
const NamedFileKind = "named"

// AttachedFilesOutput lists an object's attached files, by folder then name.
type AttachedFilesOutput struct {
	ID       string         `json:"id"`
	Module   string         `json:"module"`
	Files    []AttachedFile `json:"files"`
	Selected string         `json:"selected,omitempty" jsonschema:"The module-relative path to show first: the one asked, else the first index.md / README, else the first file"`
}

// ContentsInput names the object whose contents to show, and the page's
// filter (a query action of the page sets it).
type ContentsInput struct {
	ID       string `json:"id"                 jsonschema:"Object id; required"`
	Language string `json:"language,omitempty" jsonschema:"The person's language (en or fr); empty = English"`
	Type     string `json:"type,omitempty"     jsonschema:"Only objects of this type"`
	State    string `json:"state,omitempty"    jsonschema:"Only objects in this lifecycle state"`
	Text     string `json:"text,omitempty"     jsonschema:"Only objects whose id, name, description or tags hold this text"`
}

// SearchInput is a search across the universe: the panel's saved views (its
// declared input), the view chosen, or the criteria typed in its form.
type SearchInput struct {
	Target   string      `json:"target,omitempty"   jsonschema:"Only this module's objects; empty = every module"`
	Language string      `json:"language,omitempty" jsonschema:"The person's language (en or fr); empty = English"`
	Views    []SavedView `json:"views,omitempty"    jsonschema:"The saved views the page offers (the panel's declared input)"`
	View     string      `json:"view,omitempty"     jsonschema:"The saved view to run; its criteria replace the others"`
	// Level is the person's display level: at essential the Advanced form is
	// not offered.
	Level string `json:"level,omitempty" jsonschema:"The person's display level (essential or standard or expert); essential: no Advanced form"`
	SearchCriteria
}

// QuickSearch is the Objects panel's quick box: a code or words, and the types
// it keeps.
type QuickSearch struct {
	Text  string   `json:"text,omitempty"  jsonschema:"A code (F-031 or m.F-031) or words"`
	Types []string `json:"types,omitempty" jsonschema:"Of one of these types"`
}

// SearchCriteria is what a search keeps: every criterion optional, all of
// them together.
type SearchCriteria struct {
	Text         string `json:"text,omitempty"         jsonschema:"Words in the id or name or description or tags"`
	Types        List   `json:"types,omitempty"        jsonschema:"Of one of these types"`
	Capabilities List   `json:"capabilities,omitempty" jsonschema:"Carrying one of these capabilities"`
	States       List   `json:"states,omitempty"       jsonschema:"In one of these lifecycle states"`
	Incoming     List   `json:"incoming,omitempty"     jsonschema:"Pointed at by an edge of one of these kinds"`
	Outgoing     List   `json:"outgoing,omitempty"     jsonschema:"Pointing at something with an edge of one of these kinds"`
}

// List is a criterion's values. A form field bound to one select sends a
// string - one value, values separated by commas, or empty - where a
// multi-select sends a list: both read as the same values.
type List []string

// UnmarshalJSON reads a list, or a string of comma-separated values.
func (l *List) UnmarshalJSON(b []byte) error {
	var many []string
	if err := json.Unmarshal(b, &many); err == nil {
		*l = many
		return nil
	}
	var one string
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	*l = nil
	for _, v := range strings.Split(one, ",") {
		if v = strings.TrimSpace(v); v != "" {
			*l = append(*l, v)
		}
	}
	return nil
}

// Empty reports whether no criterion is set: a search with none searches
// nothing.
func (c SearchCriteria) Empty() bool {
	return strings.TrimSpace(c.Text) == "" && len(c.Types)+len(c.Capabilities)+len(c.States)+len(c.Incoming)+len(c.Outgoing) == 0
}

// SavedView is a named search a panel offers as a chip: its label (in each
// language it is given in), what it is for, and its criteria.
type SavedView struct {
	Name        string            `json:"name"                  jsonschema:"Its id; required"`
	Label       string            `json:"label,omitempty"       jsonschema:"Its label; empty = the name"`
	Labels      map[string]string `json:"labels,omitempty"      jsonschema:"Its label by language (fr: …)"`
	Description string            `json:"description,omitempty" jsonschema:"What it shows"`
	SearchCriteria
}

// ResolvePanelInput is what a panel is about to draw: a render-def (the
// answer of its source, its declared content) or any tree of components (a
// ui-component document's content).
type ResolvePanelInput struct {
	Def any `json:"def" jsonschema:"description=What the panel draws: a render-def (a page tree included) or any tree of components"`
}

// ResolvePanelOutput is the same display, self-contained: a renderer reads
// only inline schemas (render.ResolveComponents).
type ResolvePanelOutput struct {
	Def any `json:"def"`
}

// ResolvePartInput names one specification part: the schema a component of
// Kind draws (a data-type URI, or {action: <uri>} for an action's input) in
// its Variant (the ui-<kind> extension's name; empty = default), and the Hash
// the host holds for it, if any.
type ResolvePartInput struct {
	Kind    string `json:"kind"              jsonschema:"description=The component kind the part is drawn by (form or view or table or wizard or timeline-lanes); required"`
	Schema  any    `json:"schema"            jsonschema:"description=The part's reference: a data-type URI or {action: <uri>}; required"`
	Variant string `json:"variant,omitempty" jsonschema:"description=The ui variant's name; empty = the default"`
	Hash    string `json:"hash,omitempty"    jsonschema:"description=The hash the host holds for the part; the same as the current one answers no part"`
}

// ResolvePartOutput is the part's current Hash (render.SpecHash of Part), and
// Part itself unless the host already holds that hash. Cacheable is false for
// a part whose fields compute their choices per call (options_from): a host
// never keeps it.
type ResolvePartOutput struct {
	Part      map[string]any `json:"part,omitempty"`
	Hash      string         `json:"hash"`
	Cacheable bool           `json:"cacheable"`
}

// GlobalPanelsInput asks for the Global strip's declared panels.
type GlobalPanelsInput struct{}

// GlobalPanelsOutput is the Global strip's declared panels, by order then
// name. A source panel is called with {target}; a content panel's `${...}`
// strings are left as declared (there is no object to resolve them against).
type GlobalPanelsOutput struct {
	Panels []ObjectPanel `json:"panels"`
}

// TransitionOptionsInput asks where an object may move.
type TransitionOptionsInput struct {
	ID string `json:"id" jsonschema:"The object; required"`
	// By is who would make the move: a guard may depend on it. Empty = a person.
	By cwmapi.Actor `json:"by,omitempty" jsonschema:"Who would move it; empty = a person"`
}

// TransitionOptionsOutput is one entry per lifecycle the object has: its own
// first, then each capability that follows a workflow, by name.
type TransitionOptionsOutput struct {
	Lifecycles []LifecycleOptions `json:"lifecycles"`
}

// Guard verdicts of a transition option.
const (
	GuardNone   = "none"    // the transition has no guard
	GuardHolds  = "holds"   // the move would pass for `by`
	GuardFails  = "fails"   // the move would be refused
	GuardAtMove = "at-move" // a costly function (verify, lint), or one that cannot answer now: decided at the move
)

// LifecycleOptions is one lifecycle of an object: where it is, how it got
// there, and where it may go.
type LifecycleOptions struct {
	Capability    string                  `json:"capability,omitempty" jsonschema:"The capability whose workflow this is; empty = the object's own lifecycle"`
	Workflow      string                  `json:"workflow"`
	WorkflowLabel string                  `json:"workflow_label,omitempty"`
	State         string                  `json:"state"`
	Terminal      bool                    `json:"terminal,omitempty" jsonschema:"No transition leaves the state"`
	States        []cwmapi.WorkflowState  `json:"states"             jsonschema:"Every state of the workflow, in declaration order"`
	History       []cwmapi.LifecycleEntry `json:"history,omitempty"  jsonschema:"The moves oldest first - the journal's transitioned records"`
	// StateColors is how the lifecycle's capability colours its states (its
	// badge on the state field); a state it does not name takes "*".
	StateColors map[string]string  `json:"state_colors,omitempty" jsonschema:"State to colour - the capability's badge colours"`
	Transitions []TransitionOption `json:"transitions"        jsonschema:"The transitions from the current state"`
}

// TransitionOption is one transition from the current state, judged for `by`.
type TransitionOption struct {
	Name  string `json:"name"`
	To    string `json:"to"`
	Guard string `json:"guard,omitempty" jsonschema:"The el guard"`
	// GuardStatus is none | holds | fails | at-move.
	GuardStatus string `json:"guard_status"`
	// Calls is what the functions the guard called answered (gate('…') gives
	// its checklist); empty when the guard calls none or is left to the move.
	Calls []cwmapi.FunctionAnswer `json:"calls,omitempty"`
	// Allowed is true when the move would pass now, or is decided at the move.
	Allowed bool             `json:"allowed"`
	Reason  string           `json:"reason,omitempty" jsonschema:"Why it would be refused"`
	Force   cwmapi.ForceRule `json:"force,omitempty"  jsonschema:"Who may force it: anyone or person (a person with a note)"`
}
