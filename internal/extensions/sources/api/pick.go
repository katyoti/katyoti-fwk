// SPDX-License-Identifier: Apache-2.0

package api

// PICKING A SOURCE'S ITEMS: a connector declares how its
// items are searched (`search`), how they are picked (`picker`: the unit, a
// list or a tree, the badges, what is read with each) and optionally what it
// proposes from a context (`propose`). The display library's generic picker
// draws them; the sources module binds it to the connectors - Pickers lists a
// carrier's connectors as pickers, Search runs one connector's search - so no
// screen holds code per connector.

import "strings"

// PickItem is one item a search or a proposal answers - the generic picker's
// item.
type PickItem struct {
	Key         string   `json:"key"                   jsonschema:"What a selection names it by - the connector's own key"`
	Title       string   `json:"title"`
	Parent      string   `json:"parent,omitempty"      jsonschema:"The key of the item it sits under - a tree picker nests it there"`
	Kind        string   `json:"kind,omitempty"        jsonschema:"What it is in the connector's words"`
	URI         string   `json:"uri,omitempty"         jsonschema:"Where it is"`
	Badges      []string `json:"badges,omitempty"      jsonschema:"The names of the declared badges it carries"`
	HasChildren bool     `json:"has_children,omitempty" jsonschema:"It holds items below it"`
	// Below is how many items sit under it, at every depth: a tree
	// picker's « avec n sous-pages ».
	Below       int  `json:"below,omitempty" jsonschema:"How many items sit under it at every depth - 0 when unknown"`
	Unreachable bool `json:"unreachable,omitempty" jsonschema:"Its source cannot be reached - shown, never added"`
	// The rest of the item roles: where it is, one line on
	// it, its attributes and when it changed.
	Path     []string          `json:"path,omitempty"     jsonschema:"The titles above it - where it is"`
	Detail   string            `json:"detail,omitempty"   jsonschema:"One line saying what it is"`
	Attrs    map[string]string `json:"attrs,omitempty"    jsonschema:"Its attributes by name (an application's code) - read by another source and by displays and agents"`
	Modified string            `json:"modified,omitempty" jsonschema:"When it last changed at its source - RFC 3339"`
	// Connection is the connection it was found through:
	// empty, the universe's configuration. A selection's connection is its
	// source location's (ConnectionOf), never a field of its own.
	Connection string `json:"connection,omitempty" jsonschema:"The connection it was found through - empty: the universe's configuration"`
}

// ConnectorSearchInput is what every connector's search method takes.
type ConnectorSearchInput struct {
	Location map[string]any `json:"location,omitempty" jsonschema:"Where to search - the carrier's source location for this connector; empty: the connector's whole reach"`
	Query    string         `json:"query"              jsonschema:"Free text - titles and (where the connector can) content"`
	Root     string         `json:"root,omitempty"     jsonschema:"The project's root: absolute; what a relative location is relative to"`
	Scopes   []string       `json:"scopes,omitempty"   jsonschema:"The keys of the scopes to search in - empty: the connector's default ones"`
	From     *FromItems     `json:"from,omitempty"     jsonschema:"What the other source it reads from kept - for a connector declaring from"`
}

// ConnectorSearchOutput is what every connector's search method answers.
type ConnectorSearchOutput struct {
	Items []PickItem `json:"items"`
}

// ConnectorProposeInput is what a connector's propose method takes: a
// context - terms and free text - to propose items from.
type ConnectorProposeInput struct {
	Location map[string]any `json:"location,omitempty"`
	Terms    []string       `json:"terms,omitempty" jsonschema:"Terms of the context (an application or a process or a domain)"`
	Text     string         `json:"text,omitempty"  jsonschema:"The context in free text (a question)"`
	Root     string         `json:"root,omitempty"`
	From     *FromItems     `json:"from,omitempty"  jsonschema:"What the other source it reads from kept or proposed - for a connector declaring from"`
	// ProposeFrom is what the carrier's source of the connector its
	// propose_from names kept: proposed from beside the
	// terms, never followed.
	ProposeFrom *FromItems `json:"propose_from,omitempty" jsonschema:"What the other source it proposes from kept - for a connector declaring propose_from"`
	// Max is the carrier's source's proposal bound: 0 for
	// ProposeMax.
	Max int `json:"max,omitempty" jsonschema:"How many items to answer at most - ProposeMax when 0"`
}

// ConnectorProposeOutput is what a connector's propose method answers.
type ConnectorProposeOutput struct {
	Items []ProposedItem `json:"items"`
	// Errors names the searches that failed: the items the others found are
	// kept.
	Errors []string `json:"errors,omitempty" jsonschema:"The searches that failed - the items the others found are kept"`
}

// ProposedItem is an item proposed from a context, with why and how well it
// matches.
type ProposedItem struct {
	PickItem
	Why   string  `json:"why,omitempty"   jsonschema:"Why it is proposed - in one line"`
	Score float64 `json:"score,omitempty" jsonschema:"How well it matches - 0 to 1"`
	// By and ByItem: it was proposed from another source's kept item, not
	// from a term (propose_from) - the connector of that source and the
	// item's title; the take tab draws that connector's pill.
	By     string `json:"by,omitempty"      jsonschema:"The connector of the other source whose kept item proposed it - empty when a term or the text found it"`
	ByItem string `json:"by_item,omitempty" jsonschema:"The title of the other source's kept item that proposed it"`
}

// PickersURI is the method listing the connectors as pickers: a source-set
// field's `connectors`, a source-search node's `pickers`.
const PickersURI = "ktw-agents:sources/pickers"

// PickersInput names the carrier whose connectors are drawn as pickers.
type PickersInput struct {
	ID string `json:"id,omitempty" cli:"id" jsonschema:"The carrier's id - a module carrying the knowledge capability; empty: every connector with no location of a carrier"`
}

// PickersOutput is every contributed connector that can be searched, as the
// generic picker draws it - the `connectors` method of a source-set field.
type PickersOutput struct {
	Connectors []ConnectorPickerView `json:"connectors"`
}

// ConnectorPickerView is one connector as a picker: its search is the
// sources module's Search, called with Args (the connector) laid over the
// picker's input.
type ConnectorPickerView struct {
	Name   string `json:"name"`
	Label  string `json:"label,omitempty"`
	Icon   string `json:"icon,omitempty"`
	Search string `json:"search,omitempty"  jsonschema:"The method the picker calls - sources' Search; none for a connector picking by browsing or from another source only"`
	// Browse is the method a picker browses with - sources' SourceBrowse,
	// with Args and the carrier.
	Browse  string                 `json:"browse,omitempty" jsonschema:"The method the picker browses with - sources' SourceBrowse"`
	Args    map[string]any         `json:"args"              jsonschema:"Laid over every call the picker makes - the connector"`
	Layout  string                 `json:"layout,omitempty"`
	Unit    string                 `json:"unit,omitempty"`
	Gender  string                 `json:"gender,omitempty"`
	Badges  map[string]PickerBadge `json:"badges,omitempty"`
	Derived string                 `json:"derived,omitempty"`
	Propose bool                   `json:"propose,omitempty" jsonschema:"The connector can propose items from a context"`
	// Status is the method saying whether the connector is configured and
	// reachable (sources' Status, with Args); Configure where it is
	// configured.
	Status    string              `json:"status,omitempty"`
	Configure *ConnectorConfigure `json:"configure,omitempty"`
	// The source model the views read.
	Color  string              `json:"color,omitempty"`
	Scopes *ConnectorScopes    `json:"scopes,omitempty"`
	From   *ConnectorFrom      `json:"from,omitempty"`
	Fetch  bool                `json:"fetch,omitempty" jsonschema:"An item can be opened - the connector declares a fetch"`
	Kinds  map[string]ItemKind `json:"kinds,omitempty" jsonschema:"How each item kind is drawn - its label and slots"`
}

// SourceBrowseURI is the method a picker browses a connector with.
const SourceBrowseURI = "ktw-agents:sources/source-browse"

// StatusURI is the method a picker calls for its connector's status.
const StatusURI = "ktw-agents:sources/status"

// StatusInput names the connector whose status is asked.
type StatusInput struct {
	Connector string `json:"connector,omitempty" cli:"connector" jsonschema:"The connector's name - or a source"`
	// Source names an information source: its connector's status
	// (a source's own picker).
	Source string `json:"source,omitempty" cli:"source" jsonschema:"An information source: its connector's status"`
}

// StatusOutput says whether a connector is configured and reachable, with no
// location, and where it is configured.
type StatusOutput struct {
	Connector  string              `json:"connector"`
	Configured bool                `json:"configured"`
	Reachable  bool                `json:"reachable"`
	Message    string              `json:"message"`
	Configure  *ConnectorConfigure `json:"configure,omitempty"`
}

// SearchURI is the method a connector picker calls: sources' Search.
const SearchURI = "ktw-agents:sources/search"

// SearchInput is a picker's search through one connector, for a carrier.
type SearchInput struct {
	// Source, instead of a connector and a carrier, searches in one
	// information source: its connector, in its location.
	Source    string   `json:"source,omitempty" cli:"source"    jsonschema:"An information source: its connector searched in its location - instead of connector and object"`
	Connector string   `json:"connector,omitempty" cli:"connector" jsonschema:"The connector's name - required without a source"`
	Object    string   `json:"object,omitempty" cli:"object"    jsonschema:"The carrier - its own source for the connector gives the location searched"`
	Query     string   `json:"query"            cli:"query"     jsonschema:"Free text; required"`
	Scopes    []string `json:"scopes,omitempty" jsonschema:"The keys of the scopes to search in - empty: the connector's default ones"`
}

// SourcePickedInput names the information source whose choice a picker shows.
type SourcePickedInput struct {
	ID string `json:"id" cli:"id" jsonschema:"The information source; required"`
}

// SourcePickedOutput is a source's choice as a picker holds it: each item
// picked (or held), its state once applied.
type SourcePickedOutput struct {
	ID        string      `json:"id"`
	Connector string      `json:"connector"`
	Selected  []PickState `json:"selected"`
}

// PickState is one item of a source's choice, as the picker draws it.
type PickState struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	State string `json:"state,omitempty" jsonschema:"read or waiting or removed or failed - once applied"`
	Why   string `json:"why,omitempty"`
	Held  bool   `json:"held,omitempty"  jsonschema:"Held - chosen, not read yet"`
	// Parent and WithChildren place it in a tree picker's selection: under
	// the item it sits under, taken with its descendants or alone.
	Parent       string `json:"parent,omitempty"        jsonschema:"The key of the item it sits under"`
	WithChildren bool   `json:"with_children,omitempty" jsonschema:"Its descendants come with it"`
}

// ConnectorPanelInput asks the panel of one connector for a carrier: what
// the « Sources » board draws beside the selected row.
type ConnectorPanelInput struct {
	ID        string `json:"id"        cli:"id"        jsonschema:"The carrier's id; required"`
	Connector string `json:"connector" cli:"connector" jsonschema:"The connector's name; required"`
}

// ConnectorPanelOutput is a region's page: its tree and its data.
type ConnectorPanelOutput struct {
	DSLTree map[string]any `json:"dslTree"`
	Data    map[string]any `json:"data,omitempty"`
	Title   string         `json:"title,omitempty"`
}

// ConnectorPanelURI is the method the « Sources » board's region calls.
const ConnectorPanelURI = "ktw-agents:sources/connector-panel"

// SourceDetailInput asks an information source's detail as a page region
// : what an opened source card shows.
type SourceDetailInput struct {
	ID string `json:"id" cli:"id" jsonschema:"The information source's id; required"`
}

// SourceDetailURI is the region a page holds for one source's detail.
const SourceDetailURI = "ktw-agents:sources/source-detail"

// NewSourcePageInput asks an information source's create page under its
// holder.
type NewSourcePageInput struct {
	Parent string `json:"parent" cli:"parent" jsonschema:"The holder the source is created under - a module carrier; required"`
}

// NewSourcePageURI is the information-source type's create_task: the page the
// create dialog draws under the holder.
const NewSourcePageURI = "ktw-agents:sources/new-source-page"

// SourceTabInput asks one of an opened source's three sub-tabs
type SourceTabInput struct {
	ID  string `json:"id" cli:"id" jsonschema:"The information source's id; required"`
	Tab string `json:"tab" cli:"tab" jsonschema:"The sub-tab: take (what it takes) or read (Contenu lu) or settings (Réglages); required"`
}

// The three sub-tabs of an opened source.
const (
	SourceTabTake     = "take"
	SourceTabRead     = "read"
	SourceTabSettings = "settings"
)

// SourceTakePanel is the object panel of an opened source's take tab - where
// a source created from its create page opens.
const SourceTakePanel = "information-source-take"

// SourceTakeDefault names an opened source's take tab when its connector
// declares no take label.
const SourceTakeDefault = "Ce qu'elle prend"

// SourceTabURI is the region each sub-tab of an opened source holds.
const SourceTabURI = "ktw-agents:sources/source-tab"

// KnowledgeBaseInput asks a holder's knowledge base: every
// item read from all its sources, filtered by source, kind and state - each
// filter empty for all.
type KnowledgeBaseInput struct {
	ID     string `json:"id"               cli:"id"     jsonschema:"The holder's id - a module carrying the knowledge capability; required"`
	Source string `json:"source,omitempty" cli:"source" jsonschema:"Only the items of this information source - its id; empty for every source"`
	Kind   string `json:"kind,omitempty"   cli:"kind"   jsonschema:"Only the items of this kind: document (text) or record (fields); empty for both"`
	State  string `json:"state,omitempty"  cli:"state"  jsonschema:"Only the items in this state: read or stale (changed since it was read); empty for both"`
}

// The kinds and states of a knowledge base's items: a
// record brings fields (a data file), a document text; an item is stale when
// its source changed it since it was read.
const (
	KBKindDocument = "document"
	KBKindRecord   = "record"
	KBStateRead    = "read"
	KBStateStale   = "stale"
)

// KnowledgeBaseURI is the knowledge base's page - a holder's panel source.
const KnowledgeBaseURI = "ktw-agents:sources/knowledge-base"

// LocationSaveInput is a source's location as its connector's form shapes it
// : the form a connector panel draws from the connector's
// location data-type submits it.
type LocationSaveInput struct {
	ID       string         `json:"id"       cli:"id" jsonschema:"The information source's id; required"`
	Location map[string]any `json:"location"          jsonschema:"Where it reads - the connector's location data-type shapes it"`
}

// LocationSaveOutput says whether the location was kept.
type LocationSaveOutput struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// LocationSaveURI is the method a connector panel's location form submits to.
const LocationSaveURI = "ktw-agents:sources/location-save"

// SourceFetchInput names one item to open: the source it is
// of - or a connector and a location, before a source is saved - and its key.
type SourceFetchInput struct {
	ID        string         `json:"id,omitempty"        cli:"id"        jsonschema:"The information source; or connector and location"`
	Connector string         `json:"connector,omitempty" cli:"connector" jsonschema:"The connector's name - with location when no source is named"`
	Location  map[string]any `json:"location,omitempty"                  jsonschema:"The location - with connector when no source is named"`
	Key       string         `json:"key"                 cli:"key"       jsonschema:"The item's key; required"`
}

// SourceFetchOutput is one item opened: its roles, its data and the
// data-type that shapes it, its markdown text - either or both.
type SourceFetchOutput struct {
	Item     PickItem       `json:"item"`
	Data     map[string]any `json:"data,omitempty"      jsonschema:"The item's data"`
	DataType string         `json:"data_type,omitempty" jsonschema:"The data-type its connector declares for the item's kind - what shapes its data"`
	Text     string         `json:"text,omitempty"      jsonschema:"The item's text - rich markdown"`
}

// SourceFetchURI opens one item of a source.
const SourceFetchURI = "ktw-agents:sources/source-fetch"

// DocumentDetailInput names the source-document to open.
type DocumentDetailInput struct {
	ID string `json:"id" cli:"id" jsonschema:"The source-document's id; required"`
}

// DocumentDetailOutput is a source-document opened - what the display
// library's item-detail node draws: the item by its roles, its data and the
// data-type shaping it, its text, and the objects derived from it. Note says
// why it answers what is stored when its source's fetch failed.
type DocumentDetailOutput struct {
	Item     PickItem       `json:"item"`
	Data     map[string]any `json:"data,omitempty"      jsonschema:"The item's data"`
	DataType string         `json:"data_type,omitempty" jsonschema:"The data-type its data is shaped by"`
	Text     string         `json:"text,omitempty"      jsonschema:"The item's text - rich markdown"`
	Used     []DocumentUse  `json:"used"                jsonschema:"The objects derived from it (a derived-from edge to it)"`
	Note     string         `json:"note,omitempty"      jsonschema:"Why it answers what is stored - its source could not be opened"`
}

// DocumentUse is an object derived from a source-document: opened on a click.
type DocumentUse struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// DocumentDetailURI opens a source-document - the item by its roles, its
// data, its text and what derives from it.
const DocumentDetailURI = "ktw-agents:sources/document-detail"

// DocumentPageURI opens a source-document as a knowledge-base item - its
// pane's « Élément » tab's source.
const DocumentPageURI = "ktw-agents:sources/document-page"

// LocationScopes is the location field a carrier's added scope keys are kept
// in: a connector declaring scopes.add reads them there, its
// scopes method answering them beside its configuration's defaults.
const LocationScopes = "scopes"

// LocationConnection is the location field a source's connection id is kept
// in: the one connection the whole source reads through
// absent or empty for the universe's configuration of its connector. It is
// read only through ConnectionOf.
const LocationConnection = "connection"

// reservedLocationKeys are the location fields sources keeps itself: a
// connector's location form never drops them (LocationSave keeps them).
var reservedLocationKeys = []string{LocationScopes, LocationConnection}

// ConnectionOf is the connection a source location reads through: its
// LocationConnection field trimmed, "" (the universe's configuration) when
// absent or not a string.
func ConnectionOf(location map[string]any) string {
	v, _ := location[LocationConnection].(string)
	return strings.TrimSpace(v)
}

// KeepReservedLocation is the location a form saved, keeping the reserved
// fields (scopes, connection) the stored one holds and the form left out.
func KeepReservedLocation(stored, saved map[string]any) map[string]any {
	out := make(map[string]any, len(saved)+len(reservedLocationKeys))
	for k, v := range saved {
		out[k] = v
	}
	for _, k := range reservedLocationKeys {
		if _, set := out[k]; set {
			continue
		}
		if v, ok := stored[k]; ok {
			out[k] = v
		}
	}
	return out
}

// ScopesInput asks a carrier's scopes for one connector: what a picker's
// scope chips list.
type ScopesInput struct {
	Connector string `json:"connector" cli:"connector" jsonschema:"The connector's name; required"`
	Object    string `json:"object"    cli:"object"    jsonschema:"The carrier - its own source for the connector holds the scopes it added; required"`
}

// ScopesURI answers a carrier's scopes for a connector.
const ScopesURI = "ktw-agents:sources/scopes"

// ScopeAddInput is a scope a carrier adds for one connector.
type ScopeAddInput struct {
	Connector string `json:"connector" cli:"connector" jsonschema:"The connector's name; required"`
	Object    string `json:"object"    cli:"object"    jsonschema:"The carrier; its own source for the connector keeps the scope; required"`
	Scope     string `json:"scope"     cli:"scope"     jsonschema:"The scope's key (a space key); required"`
}

// ScopeAddOutput says whether the scope was kept.
type ScopeAddOutput struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// ScopeAddURI keeps a scope a carrier adds.
const ScopeAddURI = "ktw-agents:sources/scope-add"

// ScopeOptionsInput names the source whose « Réglages » scopes row asks its
// choices; a row sends its value beside it.
type ScopeOptionsInput struct {
	ID    string `json:"id"              cli:"id" jsonschema:"The information source's id; required"`
	Value any    `json:"value,omitempty" jsonschema:"The row's value - ignored"`
}

// ScopeOptionsOutput is a source's scopes as a row's choices: each scope's
// key its value, its key and title its label.
type ScopeOptionsOutput struct {
	Options []ScopeOption `json:"options"`
}

// ScopeOption is one choice of a row: its value, its label, what it is.
type ScopeOption struct {
	Value       string `json:"value"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

// ScopeOptionsURI answers a source's scopes as its « Réglages » scopes
// row's choices.
const ScopeOptionsURI = "ktw-agents:sources/scope-options"
