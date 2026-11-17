// SPDX-License-Identifier: Apache-2.0

package api

import "github.com/katyoti/katyoti-fwk/internal/core/render"

// THE KNOWLEDGE CAPABILITY (modules/sources/): an object carrying it — a
// study, a module — has its own sources and its own knowledge base. Its
// sources are its child `information-source` objects and those that `feeds`
// it; each sync writes into the source's module `input/<source>/`, and the
// knowledge base in the carrier's `.kb/` (ktw-kb, called by URI) indexes them.
// SourcesOf says where the carrier stands (the « Sources » tab), SyncAll
// syncs every source then reindexes the base, Reindex reindexes it alone. The
// carrier is a module: its folder holds both files.

// KnowledgeCapability is the capability the sources pack declares.
const KnowledgeCapability = "knowledge"

// OnePerConnectorCapability is the capability a holder composes to read each
// connector through one source: sources' save gate refuses a
// second active source of a connector only under a holder carrying it - the
// as-is study v2 opts in; every other holder (a module, a v1 study) may keep
// several sources of a connector.
const OnePerConnectorCapability = "one-source-per-connector"

// SubTabsCapability is the capability a holder composes for its opened
// information sources to show three sub-tabs - what each takes, « Contenu
// lu », « Réglages »: the as-is study v2
// opts in; every other holder (a module, a v1 study) keeps the one « Source »
// tab. The panels ask it through source_sub_tabs() over the source.
const SubTabsCapability = "source-sub-tabs"

// CapKnowledge is the knowledge capability's fields on its carrier (the
// cap-knowledge data-type is rendered from it): none - a generated document
// is read like any other, marked « IA » (F-020).
type CapKnowledge struct{}

// The knowledge base engine's methods, called by URI.
const (
	KBRunURI = "ktw-kb:kb/run"
)

// KBFolder and InputFolder are the carrier's folders, relative to its module.
const (
	KBFolder    = ".kb"
	InputFolder = "input"
)

// SourcesOfInput names the carrier.
type SourcesOfInput struct {
	ID string `json:"id" cli:"id" jsonschema:"The carrier's id - a module carrying the knowledge capability; required"`
}

// SourcesOfOutput is where a carrier's knowledge stands.
type SourcesOfOutput struct {
	ID        string `json:"id"`
	Folder    string `json:"folder"     jsonschema:"The carrier's folder - relative to the project"`
	KBPath    string `json:"kb_path"    jsonschema:"The knowledge base folder - absolute: the path the ktw-kb methods take"`
	InputPath string `json:"input_path" jsonschema:"The folder its sources are synced into - absolute"`
	KBExists  bool   `json:"kb_exists"  jsonschema:"Whether the knowledge base is set up (.kb/KnowledgeBase.yaml)"`
	KBSize    int64  `json:"kb_size"    jsonschema:"The knowledge base's size in bytes"`
	KBLabel   string `json:"kb_label"   jsonschema:"The knowledge base in a few words for a badge"`
	Label     string `json:"label"      jsonschema:"The sources in a few words for a badge"`
	// The knowledge-base box: the documents its sources
	// brought, the items still in progress, the two in a few words.
	Documents      int             `json:"documents"       jsonschema:"The documents its own sources brought (their last syncs)"`
	Pending        int             `json:"pending"         jsonschema:"The items held or waiting - not read yet, or a removal not done"`
	DocumentsLabel string          `json:"documents_label" jsonschema:"The documents in a few words for a chip"`
	PendingLabel   string          `json:"pending_label"   jsonschema:"en cours or à jour - for a chip"`
	Sources        []SourceSummary `json:"sources"`
	// Deposited are the documents deposited on the carrier.
	Deposited []DepositedDocument `json:"deposited,omitempty"`
	// Connectors are every source connector contributed in the universe, each
	// saying whether the carrier uses it already: the « Sources » tab offers
	// « Utiliser » on the others.
	Connectors []ConnectorUse `json:"connectors"`
}

// ConnectorUse is one contributed connector as a carrier sees it.
type ConnectorUse struct {
	Name        string `json:"name"             jsonschema:"The connector's name"`
	Label       string `json:"label"            jsonschema:"The name shown - the connector's label or its name"`
	Description string `json:"description"      jsonschema:"What it reads - one sentence"`
	Icon        string `json:"icon,omitempty"   jsonschema:"A tabler icon name"`
	Used        bool   `json:"used"             jsonschema:"Whether one of the carrier's own sources reads through it"`
	Source      string `json:"source,omitempty" jsonschema:"That source's id when used"`
	UsedLabel   string `json:"used_label"       jsonschema:"Its use in a few words for a badge"`
	// The board row the « Sources » tab draws it as: its
	// key, what is picked in a few words, its chips, its switch.
	Key   string   `json:"key"`
	Count string   `json:"count,omitempty" jsonschema:"What is picked in a few words (2 applications)"`
	Chips []string `json:"chips,omitempty"`
	On    bool     `json:"on"              jsonschema:"The « Utiliser » switch - used"`
	// The card the board draws it as when laid out as cards (
	// ): its state in a word and a colour, a meta line, the
	// titles of the items it keeps, a note on what to do, its colour.
	State *CardState `json:"state,omitempty" jsonschema:"Its state on the card - a word and a colour"`
	Meta  string     `json:"meta,omitempty"  jsonschema:"Its meta line - how many items and when it was read"`
	Items []string   `json:"items,omitempty" jsonschema:"The titles of the items its source keeps"`
	Note  string     `json:"note,omitempty"  jsonschema:"What to do - its source's error or a change to read"`
	Color string     `json:"color,omitempty" jsonschema:"The connector's declared colour"`
}

// CardState is a card's state: a word and its colour, and what it means
// when a reader must act on it - Kind, never the colour, which is display.
type CardState struct {
	Text  string `json:"text"`
	Color string `json:"color,omitempty"`
	Kind  string `json:"kind,omitempty"`
}

// CardNews is the Kind of a card proposing new values since its source's
// last save (« n propositions »), waiting on the person.
const CardNews = "news"

// UseInput names a carrier and the connector it starts reading from.
type UseInput struct {
	ID        string `json:"id"        cli:"id"        jsonschema:"The carrier's id - a module carrying the knowledge capability; required"`
	Connector string `json:"connector" cli:"connector" jsonschema:"The source connector's name; required"`
	// On "false" stops using the connector: the carrier's own source reading
	// through it is removed - the « Sources » board's switch
	// turned off.
	On string `json:"on,omitempty" cli:"on" jsonschema:"false stops using it - its source is removed; empty or true starts"`
	// Name and Scope come from the information source's create page
	// (mock-up v6 view 11): the name it is given - its
	// connector's label when empty - and, for a connector a carrier may add
	// scopes to, the first place it searches in (a Confluence space).
	Name  string `json:"name,omitempty"  cli:"name"  jsonschema:"The source's name when it is created - the connector's label when empty"`
	Scope string `json:"scope,omitempty" cli:"scope" jsonschema:"A scope the new source searches in (a space key) - for a connector a carrier may add scopes to"`
}

// DocumentsConnector is the connector of the documents deposited on a
// carrier; DocumentsFolder their folder in the carrier's.
const (
	DocumentsConnector = "documents"
	DocumentsFolder    = "documents"
)

// DepositInput is a file a person gives a carrier.
type DepositInput struct {
	ID      string  `json:"id"      cli:"id"      jsonschema:"The carrier's id; required"`
	Name    string  `json:"name"    cli:"name"    jsonschema:"The file's name - no folder in it; required"`
	Content string  `json:"content" cli:"content" jsonschema:"The file's content in base64; required"`
	By      *Keeper `json:"by,omitempty" jsonschema:"Who deposits it - a person or an agent"`
}

// DepositOutput says where the document went, in the {ok, message} shape.
type DepositOutput struct {
	OK       bool   `json:"ok"`
	Message  string `json:"message"`
	Source   string `json:"source"   jsonschema:"The carrier's « Documents » source"`
	Document string `json:"document" jsonschema:"The document's path in the carrier's folder"`
}

// DepositedDocument is one document deposited on a carrier, as the
// « Sources » tab lists it.
type DepositedDocument struct {
	Name  string `json:"name"`
	Path  string `json:"path"            jsonschema:"Its path in the carrier's folder - what « Ouvrir » opens"`
	By    string `json:"by,omitempty"    jsonschema:"Who deposited it"`
	At    string `json:"at,omitempty"    jsonschema:"When - RFC 3339"`
	State string `json:"state,omitempty" jsonschema:"read, waiting or failed - from its last sync"`
	Label string `json:"label"           jsonschema:"Who and where in a few words: déposé par … · dans la base"`
}

// DocumentsPageInput asks a carrier's « Documents » section as a page region
type DocumentsPageInput struct {
	ID string `json:"id" cli:"id" jsonschema:"The carrier's id; required"`
}

// DocumentsPageURI is the region a page holds for a carrier's « Documents ».
const DocumentsPageURI = "ktw-agents:sources/documents-page"

// RemoveInput names a source to take off its carrier.
type RemoveInput struct {
	ID string `json:"id" cli:"id" jsonschema:"The information source's id; required"`
	// Carrier is the object it feeds, when it only feeds it: the feeds edge
	// is deleted, the source left alone. Empty: the source's parent.
	Carrier string `json:"carrier,omitempty" cli:"carrier" jsonschema:"The carrier it feeds - only for a source that is not the carrier's own"`
}

// RemoveOutput says what left the carrier, in the {ok, message} shape a pane
// shows.
type RemoveOutput struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	// Removed is how many of its documents left the knowledge base.
	Removed int `json:"removed" jsonschema:"How many of its documents left the carrier"`
}

// UseOutput says which source reads through the connector now, in the {ok,
// message} shape a pane shows.
type UseOutput struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Source  string `json:"source"  jsonschema:"The carrier's source reading through the connector"`
	Created bool   `json:"created" jsonschema:"Whether it was created now - false: it was there already"`
	// Open opens the source created on its take tab: the
	// page's host follows it.
	Open *render.Navigate `json:"open,omitempty" jsonschema:"The object to open next - the source created on its take tab"`
}

// SourceSummary is one source of a carrier and what its last sync brought.
type SourceSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Connector string `json:"connector"`
	// Its connector as the views draw it: its declared
	// label, icon and colour - never its name.
	ConnectorLabel string `json:"connector_label,omitempty" jsonschema:"Its connector's declared label - its name when it declares none"`
	ConnectorIcon  string `json:"connector_icon,omitempty"`
	ConnectorColor string `json:"connector_color,omitempty"`
	State          string `json:"state"                 jsonschema:"declared or synced or stale or failed"`
	LastSynced     string `json:"last_synced,omitempty"`
	Items          int    `json:"items"`
	Picked         int    `json:"picked"                jsonschema:"How many items are picked in its selection - 0: its whole location is read"`
	PickedLabel    string `json:"picked_label,omitempty" jsonschema:"The items picked, by title, for its row"`
	Held           int    `json:"held,omitempty"         jsonschema:"How many items are held - chosen, not read until the carrier applies them"`
	Located        bool   `json:"located,omitempty"      jsonschema:"Whether its location names somewhere to read - false: declared with nothing to read yet"`
	// States says where each item applied stands.
	States map[string]ItemState `json:"states,omitempty" jsonschema:"Where each item applied stands, by its key"`
	// Kept is who kept each of its items and when, its selection's then its
	// held ones.
	Kept  []SelectedItem `json:"kept,omitempty" jsonschema:"Its items picked or held, each with who kept it and when"`
	Error string         `json:"error,omitempty"`
	Via   string         `json:"via"                   jsonschema:"child (a source under the carrier) or feeds (a source elsewhere feeding it)"`
	// From is what a source whose connector declares from follows
	// : the attribute's values on the items
	// its carrier's source of the other connector kept, each with its title.
	From *FromItems `json:"from,omitempty" jsonschema:"For a source whose connector declares from: the values it follows - none kept yet: no value"`
}

// WaitsOnPerson is the one rule saying a source waits on the person
// : its last read failed - its error set, whatever its
// lifecycle state (a source declared unreachable stays declared) - or it
// holds an item in failure. The « Sources » tab's badge counts it; a
// carrier's overview reads it, never a copy.
func WaitsOnPerson(f InformationSourceFields) bool {
	if f.Error != "" {
		return true
	}
	for _, st := range f.States {
		if st.State == ItemFailed {
			return true
		}
	}
	return false
}

// WaitingCountOutput is the « Sources » tab's badge: how many of the
// carrier's own sources wait on the person, red; no colour at 0.
type WaitingCountOutput struct {
	Count int    `json:"count"`
	Color string `json:"color,omitempty"`
}

// SyncAllInput names the carrier whose sources are synced.
type SyncAllInput struct {
	ID string `json:"id" cli:"id" jsonschema:"The carrier's id - a module carrying the knowledge capability; required"`
}

// SyncAllOutput says what syncing every source and reindexing did, in the
// {ok, message} shape a pane shows.
type SyncAllOutput struct {
	OK        bool               `json:"ok"`
	Message   string             `json:"message"`
	Sources   []SourceSyncResult `json:"sources"`
	Reindexed bool               `json:"reindexed" jsonschema:"Whether the knowledge base was reindexed"`
	Errors    []string           `json:"errors,omitempty"`
}

// SourceSyncResult is one source's sync within a SyncAll.
type SourceSyncResult struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	State string `json:"state"`
	Items int    `json:"items"`
	Error string `json:"error,omitempty"`
}

// ReindexInput names the carrier whose knowledge base is reindexed.
type ReindexInput struct {
	ID string `json:"id" cli:"id" jsonschema:"The carrier's id - a module carrying the knowledge capability; required"`
}

// ReindexOutput says whether the knowledge base was reindexed.
type ReindexOutput struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// UseSetInput turns a source-set value into a carrier's sources: each item
// with its connector.
type UseSetInput struct {
	ID    string       `json:"id"    cli:"id" jsonschema:"The carrier's id; required"`
	Items []UseSetItem `json:"items"          jsonschema:"The items picked, each with its connector - a source-set field's value"`
	Wait  bool         `json:"wait,omitempty"  jsonschema:"Read them before answering - default: in the background"`
}

// UseSetItem is one item of a source-set value.
type UseSetItem struct {
	Connector    string `json:"connector"`
	Key          string `json:"key"`
	Title        string `json:"title,omitempty"`
	WithChildren bool   `json:"with_children,omitempty"`
}

// UseSetOutput says which sources hold the choice.
type UseSetOutput struct {
	OK      bool     `json:"ok"`
	Message string   `json:"message"`
	Sources []string `json:"sources" jsonschema:"The carrier's sources holding the choice - one per connector"`
}
