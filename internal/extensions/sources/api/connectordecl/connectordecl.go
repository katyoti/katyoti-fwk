// SPDX-License-Identifier: Apache-2.0

// Package connectordecl is the shape of a `source-connector` entry: one kind
// of place a module can read from, as a connector declares it. It imports
// nothing of the CWM, so the kind's leaf (sources/contrib/connectorkind) can
// publish its schema from the type and the built-in kind reader import it
// without a cycle; the sources api names its types by alias.
package connectordecl

import (
	"strings"
	"time"
)

// Kind is the contribution kind a ConnectorDecl is an entry of.
const Kind = "source-connector"

// ConnectorDecl is a `source-connector` entry: one kind of place a module can
// read from.
type ConnectorDecl struct {
	Name        string `json:"name"               jsonschema:"description=The connector's name in kebab-case - what a source's connector field holds"`
	Label       string `json:"label,omitempty"    jsonschema:"description=The name shown in a picker"`
	Description string `json:"description"        jsonschema:"description=What it reads - one sentence"`
	Icon        string `json:"icon,omitempty"     jsonschema:"description=A tabler icon name"`
	// System is the connector's short name - the system it reads (LeanIX) -
	// as a sentence names it: a take tab's head reads « ce que <source>
	// prend dans <system> ».
	System string `json:"system,omitempty" jsonschema:"description=The short name of the system it reads (LeanIX) - as a sentence names it: ce que la source prend dans LeanIX; the sentence names none when empty"`
	// Mark is the system's short mark (LX) drawn as its badge - a take
	// tab's link band and its groups (mock-up v6); the
	// system's name is drawn when empty.
	Mark string `json:"mark,omitempty" jsonschema:"description=The short mark of the system it reads (LX) - drawn as its badge on a take tab's link band and groups; the system's name when empty"`
	// Take names an opened source's « what it takes » tab by what the
	// connector calls its items once taken (mock-up v6).
	Take     string `json:"take,omitempty" jsonschema:"description=The name of an opened source's tab of what it takes - its items as the connector calls them once taken (« Pages prises »); « Ce qu'elle prend » when empty"`
	Location string `json:"location,omitempty" jsonschema:"description=The data-type of its location and selection form - the shape of a source's location and selection fields"`
	Sync     string `json:"sync"               jsonschema:"description=The method that syncs a source: it answers ConnectorSyncInput with ConnectorSyncOutput"`
	Test     string `json:"test,omitempty"     jsonschema:"description=The method that checks a location can be read - same input and an answer with ok and message"`
	// Check is what the connector refuses at save: sources' own save gate
	// asks it, never a gate of the connector's pack on a type it
	// does not own.
	Check    string `json:"check,omitempty" jsonschema:"description=The method that checks a source of this connector before it is saved - it answers a before hook's payload (the source as it is about to be) with accept or a reason; a source of a connector without one saves as it is"`
	Browse   string `json:"browse,omitempty"   jsonschema:"description=The method that lists a location's candidates level by level - it answers ConnectorBrowseInput with ConnectorBrowseOutput; a connector without one syncs its whole location"`
	TimeoutS int    `json:"timeout_s,omitempty" jsonschema:"description=How long a sync may take in seconds - 600 when empty"`
	// Search, Picker and Propose let the generic picker choose the items
	//
	Search  string           `json:"search,omitempty"  jsonschema:"description=The method that searches the connector's items by free text - it answers ConnectorSearchInput with ConnectorSearchOutput; optional - a connector without one is still offered in a picker when it browses or reads from another source"`
	Picker  *ConnectorPicker `json:"picker,omitempty"  jsonschema:"description=How its items are picked - the unit; a list or a tree; the badges; what is read with each"`
	Propose string           `json:"propose,omitempty" jsonschema:"description=The method that proposes items from a context - it answers ConnectorProposeInput with ConnectorProposeOutput; optional"`
	// Configure says where the connector is configured: a picker offers it
	// when the connector is not.
	Configure *ConnectorConfigure `json:"configure,omitempty" jsonschema:"description=Where the connector is configured - the link a picker offers when it is not"`
	// THE SOURCE MODEL: who the source is (its colour beside
	// label and icon), the scopes it searches in, the fetch operation that
	// opens one item, and what it reads from another source. Every view
	// reads them - never a connector's name.
	Color  string           `json:"color,omitempty"  jsonschema:"description=Its colour on cards and chips and progress lines - a Mantine colour"`
	Scopes *ConnectorScopes `json:"scopes,omitempty" jsonschema:"description=The places it searches in (a Confluence space) - where the default ones come from and whether a carrier adds its own"`
	Fetch  string           `json:"fetch,omitempty"  jsonschema:"description=The method that opens one item - it answers ConnectorFetchInput with ConnectorFetchOutput; optional"`
	From   *ConnectorFrom   `json:"from,omitempty"   jsonschema:"description=The other source it reads from - the connectors (or the role of those) whose kept items it follows and the attribute of theirs it uses"`
	// Roles are what the connector's items are to a source following a
	// role (application): a from or propose_from naming the role follows
	// it with every other connector declaring it.
	Roles []string `json:"roles,omitempty" jsonschema:"description=The roles it plays for a source following a role (application) - a from or propose_from naming one follows every connector declaring it"`
	// ProposeFrom feeds the proposal only: the items the
	// carrier's source of another connector kept are handed to propose
	// beside the keywords - a Confluence page titled after a kept LeanIX
	// application is proposed - and nothing else; From keeps its meaning.
	ProposeFrom *ConnectorFrom `json:"propose_from,omitempty" jsonschema:"description=Another source whose kept items it also proposes from - the connector and the attribute of theirs handed to propose beside the keywords; it only feeds the proposal (from follows a source)"`
	// Items types what an item brings: an item is data
	// typed by the data-type its connector declares for its kind, rich
	// markdown text with its images, or both.
	Items map[string]string `json:"items,omitempty" jsonschema:"description=The data-type of each item kind it brings data for - by kind (application or entity); an item of a kind with none brings text only"`
	// Kinds says how each kind of item is drawn: its
	// label and the slots its cards draw - an attribute of the item (an
	// application's code) at the sizes named. Handed as kinds to the picker
	// node and on each answer of a multi-source search; data, never code
	// per kind.
	Kinds map[string]ItemKind `json:"kinds,omitempty" jsonschema:"description=How each item kind is drawn - by kind (application or page) its label and the slots its cards draw"`
	// Read lays the connector's own columns on an opened source's « Contenu
	// lu » table, after the title, kind, state and read
	// date every source shows: each an attribute of the item's data - a
	// LeanIX interface's source and target, a Confluence page's parent.
	// Data, never GUI code per connector.
	Read []ReadColumn `json:"read,omitempty" jsonschema:"description=The columns an opened source's « Contenu lu » table adds to the title and kind and state and read date - each an attribute of the item's data and its header"`
	// Connection names the connection a source reads through on its
	// « Réglages » (mock-up v6 « Environnement »): the
	// row's label and the method listing the connections to choose from,
	// kept in the location's reserved connection field. A connector
	// without one has no such row.
	Connection *ConnectorConnection `json:"connection,omitempty" jsonschema:"description=The connection a source reads through - the label of its row on an opened source's settings and the method listing the connections to choose from; no row when empty"`
}

// ConnectorConnection is how an opened source's « Réglages » chooses the
// connection it reads through: the row's label and the method answering
// the choices; with none, the one choice is the universe's configuration.
type ConnectorConnection struct {
	Label string `json:"label"          jsonschema:"description=The row's label (Environnement)"`
	List  string `json:"list,omitempty" jsonschema:"description=The method answering the connections to choose from as options - each a value (the connection's id - empty for the universe's configuration) and a label; the universe's configuration alone when empty"`
}

// ReadColumn is one column a connector adds to « Contenu lu »: the
// attribute of the item's data it shows and its header.
type ReadColumn struct {
	Attr  string `json:"attr"            jsonschema:"description=The attribute of the item's data shown (a top-level field of its data file)"`
	Label string `json:"label,omitempty" jsonschema:"description=The column's header - the attribute when empty"`
}

// ItemKind is how one kind of item is drawn on its cards: its label and
// its slots.
type ItemKind struct {
	Label string     `json:"label,omitempty" jsonschema:"description=The kind's name on a card"`
	Slots []ItemSlot `json:"slots,omitempty" jsonschema:"description=The attributes its cards draw - each its attribute and label and sizes"`
}

// ItemSlot is one attribute an item card draws: the attribute's name in the
// item's attrs, its label (a tooltip) and the card sizes it shows at -
// every size when it names none.
type ItemSlot struct {
	Attr  string   `json:"attr"            jsonschema:"description=The item attribute drawn (code)"`
	Label string   `json:"label,omitempty" jsonschema:"description=Its label - shown as a tooltip"`
	Sizes []string `json:"sizes,omitempty" jsonschema:"description=The card sizes it shows at (search or browse or selection or summary) - every size when empty"`
}

// ConnectorScopes is where a connector searches: the unit of a scope, the
// method answering the default scopes from its configuration, and whether a
// carrier adds scopes of its own.
type ConnectorScopes struct {
	Unit string `json:"unit"           jsonschema:"description=What one scope is in the connector's words (espace or projet)"`
	List string `json:"list,omitempty" jsonschema:"description=The method answering its default scopes from its configuration - it answers ConnectorScopesInput with ConnectorScopesOutput"`
	Add  bool   `json:"add,omitempty"  jsonschema:"description=A carrier may add a scope of its own"`
}

// ConnectorFrom is the other source a connector reads from: the connector
// whose kept items it follows and the attribute of theirs it uses
// (Dynatrace from LeanIX applications by their code).
//
// It follows one connector, several, or every connector playing a role
// (application): a source following applications by their code follows
// the carrier's application sources whichever system they read. Readers
// ask Names - the role is resolved into Connectors when the connectors
// are listed - so one connector reads as a list of one.
type ConnectorFrom struct {
	Connector  string   `json:"connector,omitempty"  jsonschema:"description=The connector whose kept items it follows - one connector; connectors or role name several"`
	Connectors []string `json:"connectors,omitempty" jsonschema:"description=The connectors whose kept items it follows - beside connector"`
	Role       string   `json:"role,omitempty"       jsonschema:"description=The role of the connectors whose kept items it follows (application) - every connector declaring it in its roles"`
	Attr       string   `json:"attr"                 jsonschema:"description=The attribute of those items it uses (code)"`
}

// Names is every connector it follows, each once: connector first, then
// connectors (a role's connectors once the connectors are listed).
func (f *ConnectorFrom) Names() []string {
	if f == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, n := range append([]string{f.Connector}, f.Connectors...) {
		if n = strings.TrimSpace(n); n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// Follows says whether it follows the connector of that name.
func (f *ConnectorFrom) Follows(name string) bool {
	for _, n := range f.Names() {
		if n == name {
			return true
		}
	}
	return false
}

// First is the first connector it follows - what names the other source
// while the carrier holds none of them; empty when it follows none.
func (f *ConnectorFrom) First() string {
	if names := f.Names(); len(names) > 0 {
		return names[0]
	}
	return ""
}

// WithRole is the declaration with its role resolved: the connectors
// among decls declaring that role are added to Connectors, in their order.
// The receiver is left as it is (a copy is answered); nil stays nil.
func (f *ConnectorFrom) WithRole(decls []ConnectorDecl) *ConnectorFrom {
	if f == nil || strings.TrimSpace(f.Role) == "" {
		return f
	}
	out := *f
	out.Connectors = append([]string(nil), f.Connectors...)
	for _, d := range decls {
		if d.Plays(f.Role) {
			out.Connectors = append(out.Connectors, d.Name)
		}
	}
	return &out
}

// Plays says whether the connector declares that role.
func (d ConnectorDecl) Plays(role string) bool {
	for _, r := range d.Roles {
		if strings.EqualFold(strings.TrimSpace(r), strings.TrimSpace(role)) && r != "" {
			return true
		}
	}
	return false
}

// ConnectorConfigure is where a connector is configured: the link's label,
// what to set and the keys it needs - never an installation's own names.
type ConnectorConfigure struct {
	Label string   `json:"label"          jsonschema:"description=The link's label (Configurer LeanIX)"`
	Hint  string   `json:"hint,omitempty" jsonschema:"description=What to set and where - one sentence"`
	Keys  []string `json:"keys,omitempty" jsonschema:"description=The keys it needs (set through the universe's config sets)"`
}

// SyncTimeout is how long a sync of this connector may take.
func (d ConnectorDecl) SyncTimeout() time.Duration {
	if d.TimeoutS > 0 {
		return time.Duration(d.TimeoutS) * time.Second
	}
	return 10 * time.Minute
}

// ConnectorPicker is how a connector's items are picked.
type ConnectorPicker struct {
	Unit string `json:"unit,omitempty"    jsonschema:"description=What one item is in the connector's words (application or page or tag) - the picker counts them by it"`
	// Gender is the unit's French grammatical gender, declared - never
	// guessed from the word's ending: what the picker's words agree with
	// (« une autre application », « choisies »).
	Gender  string                 `json:"gender,omitempty" jsonschema:"description=The unit's French grammatical gender - feminine or masculine (the default) - what the picker's words agree with (une autre application)"`
	Layout  string                 `json:"layout,omitempty"  jsonschema:"description=How the selection shows: list (the default) or tree (by each item's parent)"`
	Badges  map[string]PickerBadge `json:"badges,omitempty"  jsonschema:"description=The badges its items may carry - by name"`
	Derived string                 `json:"derived,omitempty" jsonschema:"description=What is read with each item without being picked (an application's interfaces) - shown beside the selection"`
}

// PickerBadge is a badge an item may carry: its label, colour and meaning.
type PickerBadge struct {
	Label       string `json:"label"                 jsonschema:"description=The badge's text"`
	Color       string `json:"color,omitempty"       jsonschema:"description=A Mantine colour"`
	Description string `json:"description,omitempty" jsonschema:"description=What it means - shown on hover"`
	// Unticked draws the badge on a proposed row only while that row is
	// unticked (a « nouvelle » pill the tick hides).
	Unticked bool `json:"unticked,omitempty" jsonschema:"description=Drawn on a proposed row only while that row is unticked"`
}

// Feminine says whether the picker's unit is declared feminine.
func (p *ConnectorPicker) Feminine() bool {
	return p != nil && strings.EqualFold(strings.TrimSpace(p.Gender), GenderFeminine)
}

// Selects says whether the picker draws a selection that replaces the
// source's location: it declares how its selection shows - a layout, badges
// or what is read with each item. A picker declaring only its unit and
// gender gives the connector its words (« une entité », « choisies ») and
// leaves the location form where it is.
func (p *ConnectorPicker) Selects() bool {
	return p != nil && (strings.TrimSpace(p.Layout) != "" || len(p.Badges) > 0 || strings.TrimSpace(p.Derived) != "")
}

// The genders a picker's unit declares.
const (
	GenderFeminine  = "feminine"
	GenderMasculine = "masculine"
)
