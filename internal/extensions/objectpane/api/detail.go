// SPDX-License-Identifier: Apache-2.0

package api

import (
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE DETAIL PANE in one round trip: the object, its type, its effective
// capabilities, the tab strip, its aspects, badges, policies, links and
// children by type — composed from CWM's reads (ObjectGet, ObjectTypeGet,
// ObjectEffectiveCapabilities, ObjectEffectiveSurface, EdgeList, ObjectList,
// ObjectEvaluate) and the catalog's declarations.

// DetailInput names the object the pane is showing.
type DetailInput struct {
	ID string `json:"id" jsonschema:"Object id; required"`
	// Body asks for the object WITH its body and fields — taken when the pane
	// will render them, not on every selection in a tree walk.
	Body bool `json:"body,omitempty" jsonschema:"The object with its body and fields; false = the resident record only"`
}

// AspectRef is one aspect an object carries, resolved against the declarations.
// An UNDECLARED aspect is returned with Declared false rather than dropped: the
// chip row shows what the file says.
type AspectRef struct {
	Name         string   `json:"name"`
	Label        string   `json:"label,omitempty"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Declared     bool     `json:"declared"`
}

// ObjectLink is one edge as the Links section reads it: the kind from THIS
// object's side (the declaration's inverse_name for an incoming edge), the
// other end, and whether it points out or in.
type ObjectLink struct {
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Direction string `json:"direction" jsonschema:"out | in"`
	// TargetID is the other end as stored; for a path or uri edge it is not
	// an object id, which is why Resolved says whether it was found.
	TargetID    string `json:"target_id"`
	TargetType  string `json:"target_type,omitempty"`
	TargetTitle string `json:"target_title,omitempty"`
	// Resolved is false for an end that is not an object in this project — a
	// path, a URI, or a dangling id. The GUI shows it as text, not a link.
	Resolved bool `json:"resolved"`
	// Ended is an unresolved end that is an ARCHIVED object (cwm
	// ObjectArchive): the link is valid, its end has left the project. The
	// GUI shows it as ended, not as a broken reference.
	Ended bool `json:"ended,omitempty"`
}

// Panel kinds (ObjectPanel.Kind).
const (
	PanelKindContent  = "content"  // a declared panel whose content is a render.Def, resolved against the object
	PanelKindSource   = "source"   // a declared panel the GUI fills by calling Source with the object id
	PanelKindFields   = "fields"   // a field-group rendered from its schema
	PanelKindChildren = "children" // the children of one type not drawn in the tree
)

// ChildrenPanelOrder places the derived children tabs after every declared
// panel of the shipped set.
const ChildrenPanelOrder = 1000

// ObjectPanel is ONE entry of the tab strip. Two ways it comes to exist:
// DECLARED — an `object-panel` whose filter admits the object, once however
// many ways it matches; DERIVED — the type's own fields section, an effective
// capability's fields that no matched panel names, a list of children not
// drawn in the tree. Sorted by Order, then Name.
type ObjectPanel struct {
	ID          string                       `json:"id"                    jsonschema:"Strip entry id; unique within the pane"`
	Name        string                       `json:"name"`
	Description string                       `json:"description,omitempty"`
	Icon        *contribapi.ContributionIcon `json:"icon,omitempty"`
	Order       int                          `json:"order"`
	Kind        string                       `json:"kind" jsonschema:"content | source | fields | children"`
	// ChildType is the child type a children tab lists (Kind children).
	ChildType string `json:"child_type,omitempty"`
	// Content is the render definition, `${...}` already resolved against
	// the object (Kind content).
	Content *render.Def `json:"content,omitempty"`
	// Source is the method the GUI calls with the object id (Kind source).
	Source string `json:"source,omitempty"`
	// Targets is the method answering a global panel's targets.
	Targets string `json:"targets,omitempty"`
	// Count is a declared panel's count method ({count: n; color?}), Global
	// strip or object tab: shown beside its name on the strip, as the tab's
	// badge on an object (the GUI calls it; a stored Badge wins).
	Count string `json:"count,omitempty"`
	// Input is a declared panel's input, laid under what its source is
	// called with.
	Input map[string]any `json:"input,omitempty"`
	// Refresh is a declared panel's refresh, its scope defaulted: which
	// object changes re-read it while it is shown.
	Refresh *PanelRefresh `json:"refresh,omitempty"`
	// Empty is a declared panel's empty state: its sentence and its action.
	Empty *PanelEmpty `json:"empty,omitempty"`
	// Badge is a declared panel's tab badge, read from the object
	// ; none when its value is empty or 0.
	Badge *cwmapi.Badge `json:"badge,omitempty"`
	// Capabilities are the capabilities this entry is about.
	Capabilities []string `json:"capabilities,omitempty"`
	// Schema is the data-type URI driving a fields entry's form.
	Schema string `json:"schema,omitempty"`
	// Verify is carried from the capability so a fields entry can offer its
	// Run check button without a second lookup.
	Verify bool `json:"verify,omitempty"`
	// Origin is "type", "object", "panel" or "children".
	Origin string `json:"origin,omitempty"`
	// Section says the entry is a section of the Information tab, not a tab.
	Section bool `json:"section,omitempty"`
	// Header says the entry is the object's header, drawn above its tabs -
	// never a tab.
	Header bool `json:"header,omitempty"`
	// More says the tab is not one of the object's main tabs: an aspect it
	// carries lists its tabs (`pane.tabs`) and this one is not listed. The
	// strip shows it behind « More… ».
	More bool `json:"more,omitempty"`
	// Body is the type's body declaration, on the type's fields section.
	Body *cwmapi.BodyDecl `json:"body,omitempty"`
}

// CapabilityInfo is one capability as the Information tab shows it.
type CapabilityInfo struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	// Source is "type", "aspect:<name>", "object" or "attached".
	Source string `json:"source,omitempty"`
}

// ObjectChildGroup is one list under an object: the children of one type.
// Drawn says the group is ALREADY in the tree, so the pane can show a count.
// A group not drawn is a tab: Plural names it and Rows say, per object, what
// the list shows beside its name.
type ObjectChildGroup struct {
	Type    string           `json:"type"`
	Label   string           `json:"label,omitempty"`
	Plural  string           `json:"plural,omitempty"`
	Icon    string           `json:"icon,omitempty"`
	Count   int              `json:"count"`
	Drawn   bool             `json:"drawn,omitempty"`
	Objects []cwmapi.Object  `json:"objects"`
	Rows    []ObjectChildRow `json:"rows,omitempty"`
}

// ObjectChildRow is one child as its parent's tab lists it, in the order of
// Objects: its state and its key fields.
type ObjectChildRow struct {
	ID string `json:"id"`
	// State is the object's lifecycle state — its workflow's initial one when
	// it never moved — coloured as the lifecycle badge colours it; nil when the
	// object follows no workflow.
	State *cwmapi.Badge `json:"state,omitempty"`
	// Badges are its key fields: every badge its type and its capabilities
	// declare whose field has a value in the resident record, the state's
	// excepted.
	Badges []cwmapi.Badge `json:"badges,omitempty"`
}

// DetailOutput is the pane (modules/gui/objects-what-the-user-sees.md §5).
type DetailOutput struct {
	Object     cwmapi.Object     `json:"object"`
	ObjectType cwmapi.ObjectType `json:"object_type"`
	// Project is the key of the project holding the object when the universe
	// serves several (cwm ObjectGetOutput.Project); empty with one. The ids
	// of the answer - the object's, its links' and its children's - stay bare,
	// as a list's: they are of this project unless written qualified.
	Project string `json:"project,omitempty"`
	// Revision is the stored version read: a save hands it back so a write
	// over a change made elsewhere since is refused (ObjectUpdate.Revision).
	Revision string `json:"revision,omitempty"`

	// Effective is the EFFECTIVE capability set — type ∪ aspects ∪ instance.
	Effective []cwmapi.CapabilityRef `json:"effective"`
	// Panels is the tab strip — see ObjectPanel.
	Panels []ObjectPanel `json:"panels,omitempty"`
	// Bare says the pane draws no object header: the type's listed tabs are
	// its whole pane (`pane.only`), their panels drawing its head.
	Bare bool `json:"bare,omitempty"`
	// Capabilities is the same set as bare ids, in the same order.
	Capabilities []string `json:"capabilities,omitempty"`
	// CapabilityInfo is the same set as the Information tab shows it.
	CapabilityInfo []CapabilityInfo `json:"capability_info,omitempty"`
	// Aspects is the chip row.
	Aspects []AspectRef `json:"aspects,omitempty"`
	// Badges are the header's badges: the object's own, then each one its type
	// and capabilities declare whose field has a value.
	Badges []cwmapi.Badge `json:"badges,omitempty"`
	// Policies are the rules that apply to the object.
	Policies []cwmapi.Policy `json:"policies,omitempty"`

	// CreatedAt and UpdatedAt are when the object was created and last
	// changed, as the journal derives them (JournalService.Times) — no object
	// stores its times; absent when not known.
	CreatedAt time.Time `json:"created_at,omitzero"`
	UpdatedAt time.Time `json:"updated_at,omitzero"`

	// QuickActions are the system defaults every object carries (chat), then
	// the actions its type and its capabilities declare quick: small buttons
	// on its first page.
	QuickActions []cwmapi.Action `json:"quick_actions"`

	Links    []ObjectLink       `json:"links,omitempty"`
	Children []ObjectChildGroup `json:"children,omitempty"`

	// Diagnostics are the object's errors as CWM read it; the object still
	// renders.
	Diagnostics []cwmapi.ObjectDiagnostic `json:"diagnostics,omitempty"`
}
