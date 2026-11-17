// SPDX-License-Identifier: Apache-2.0

package api

// INFORMATION SOURCES (modules/sources/): a place a module reads from - a
// Confluence page, a Dynatrace tag, a LeanIX application, a web site, a
// folder - recorded as an `information-source` object. Which places exist is
// not a list in this package: each is a `source-connector` contribution a
// sidecar publishes (ConnectorDecl), naming the form of its location and the
// method that syncs it. SourceSync calls that method by its URI with one
// contract (ConnectorSyncInput / ConnectorSyncOutput): the connector writes
// what it brings into a folder of the module, and every item it brought is
// anchored as a `source-document` under the source, so a later change is
// noticed (Reanchor, Stale).

import (
	"fmt"
	"slices"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api/connectordecl"
)

// The kind, the type, the workflow and the edge the sources pack declares.
const (
	ContributionTypeSourceConnector = connectordecl.Kind
	InformationSourceType           = "information-source"
	InformationSourceWorkflow       = "information-source"
	FeedsEdge                       = "feeds"
	// FolderConnector is the connector the sources extension serves itself:
	// a folder of files mirrored into the module.
	FolderConnector = "local-folder"
)

// The states of an information source.
const (
	SourceStateDeclared = "declared"
	SourceStateSynced   = "synced"
	SourceStateStale    = "stale"
	SourceStateFailed   = "failed"
)

// The shape of a `source-connector` entry lives in connectordecl, a package
// free of the CWM (the kind's leaf publishes its schema from it); the api
// names it here.
type (
	ConnectorDecl       = connectordecl.ConnectorDecl
	ConnectorPicker     = connectordecl.ConnectorPicker
	PickerBadge         = connectordecl.PickerBadge
	ConnectorConfigure  = connectordecl.ConnectorConfigure
	ConnectorScopes     = connectordecl.ConnectorScopes
	ConnectorFrom       = connectordecl.ConnectorFrom
	ConnectorConnection = connectordecl.ConnectorConnection
	ItemKind            = connectordecl.ItemKind
	ItemSlot            = connectordecl.ItemSlot
	ReadColumn          = connectordecl.ReadColumn
)

// The genders a picker's unit declares.
const (
	GenderFeminine  = connectordecl.GenderFeminine
	GenderMasculine = connectordecl.GenderMasculine
)

// ConnectorScopesInput is what a connector's scopes method takes.
type ConnectorScopesInput struct {
	Location map[string]any `json:"location,omitempty" jsonschema:"The carrier's source location for this connector - empty: the configuration's scopes"`
	Root     string         `json:"root,omitempty"`
}

// ConnectorScopesOutput is what a connector's scopes method answers.
type ConnectorScopesOutput struct {
	Scopes []Scope `json:"scopes"`
}

// Scope is one place a connector searches in.
type Scope struct {
	Key     string `json:"key"               jsonschema:"What a search names it by (a space key)"`
	Title   string `json:"title,omitempty"`
	Default bool   `json:"default,omitempty" jsonschema:"It comes from the configuration - searched unless the carrier says otherwise"`
	// Connection is the connection it is reached through:
	// empty, the universe's one configuration of its connector.
	Connection string `json:"connection,omitempty" jsonschema:"The connection it is reached through - empty: the universe's configuration"`
}

// FromItems is what a connector reading from another source receives
// : the other connector, the attribute of its items it reads
// and that attribute's values on the items the carrier's source of that
// connector brought - Dynatrace's entities by the codes of the LeanIX
// applications kept - and each value's item title, so the connector can name
// what it follows (a Dynatrace group titled by its LeanIX application).
type FromItems struct {
	Connector string            `json:"connector"        jsonschema:"The connector whose kept items it follows"`
	Attr      string            `json:"attr"             jsonschema:"The attribute of those items it reads (code)"`
	Values    []string          `json:"values"           jsonschema:"That attribute's values on the items kept - none while nothing is kept"`
	Titles    map[string]string `json:"titles,omitempty" jsonschema:"Each value's item title by the value - absent for a value whose item has no title"`
	// Connectors is each value's own connector by the value:
	// following several application sources, a value kept by the second is
	// that second connector's, never the first's.
	Connectors map[string]string `json:"connectors,omitempty" jsonschema:"Each value's connector by the value - absent: Connector"`
}

// SetConnector records the connector whose source kept a value, once: the
// first source keeping it names it; an empty name records nothing.
func (f *FromItems) SetConnector(value, name string) {
	if name == "" || f.Connectors[value] != "" {
		return
	}
	if f.Connectors == nil {
		f.Connectors = map[string]string{}
	}
	f.Connectors[value] = name
}

// ConnectorOf is the connector whose source kept a value, else Connector.
func (f *FromItems) ConnectorOf(value string) string {
	if f == nil {
		return ""
	}
	if n := f.Connectors[value]; n != "" {
		return n
	}
	return f.Connector
}

// Followed is every connector its values come from, each once: Connector
// first, then the values' own connectors in the values' order.
func (f *FromItems) Followed() []string {
	if f == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	add(f.Connector)
	for _, v := range f.Values {
		add(f.Connectors[v])
	}
	return out
}

// SetTitle records the title of the item carrying a value, once: the first
// item carrying it names it; an empty title records nothing.
func (f *FromItems) SetTitle(value, title string) {
	if title == "" || f.Titles[value] != "" {
		return
	}
	if f.Titles == nil {
		f.Titles = map[string]string{}
	}
	f.Titles[value] = title
}

// Title is the title of the item carrying a value, else the value itself.
func (f *FromItems) Title(value string) string {
	if f != nil && f.Titles[value] != "" {
		return f.Titles[value]
	}
	return value
}

// AddedSince is the values kept since a save that saw seen, in their
// order: none when seen is empty - a source never saved
// has nothing new.
func (f *FromItems) AddedSince(seen []string) []string {
	if f == nil || len(seen) == 0 {
		return nil
	}
	at := make(map[string]bool, len(seen))
	for _, v := range seen {
		at[v] = true
	}
	var out []string
	for _, v := range f.Values {
		if !at[v] {
			out = append(out, v)
		}
	}
	return out
}

// ConnectorFetchInput is what a connector's fetch method takes: one item.
type ConnectorFetchInput struct {
	Location map[string]any `json:"location,omitempty"`
	Key      string         `json:"key" jsonschema:"The item's key"`
	Root     string         `json:"root,omitempty"`
}

// ConnectorFetchOutput is what a connector's fetch method answers: the item
// by its roles, and what it brings - its data, typed by the data-type its
// connector declares for its kind, its markdown text, or both
type ConnectorFetchOutput struct {
	Item PickItem       `json:"item"`
	Data map[string]any `json:"data,omitempty" jsonschema:"The item's data - shaped by the data-type its connector declares for its kind"`
	Text string         `json:"text,omitempty" jsonschema:"The item's text - rich markdown"`
}

// SelectionItems is the key of a source's selection holding the items picked
// one by one: a list of SelectedItem. The selection takes this one shape for
// every connector; a connector reads it, never a selection of its own.
const SelectionItems = "items"

// SelectedItem is one item picked in a source's selection.
type SelectedItem struct {
	Key          string `json:"key"                     jsonschema:"The item's key as the connector's browse answered it"`
	Title        string `json:"title,omitempty"         jsonschema:"Its title - shown in the source's selection"`
	WithChildren bool   `json:"with_children,omitempty" jsonschema:"Its descendants come with it"`
	// Parent is the key of the item it sits under, as the picker's tree
	// placed it: a tree picker shows the selection nested by it.
	Parent string `json:"parent,omitempty" jsonschema:"The key of the item it sits under - a tree picker nests it there"`
	// Who kept it and when: the person who first held it
	// kept through the apply - the one record of who kept each item, read
	// by any consumer (SourcesOf, the source's fields).
	KeptBy *Keeper `json:"kept_by,omitempty" jsonschema:"Who kept it - the person who held it first"`
	KeptAt string  `json:"kept_at,omitempty" jsonschema:"When it was kept - RFC 3339"`
}

// Keeper is who kept an item: an actor, a person or an agent.
type Keeper struct {
	ID   string `json:"id"   jsonschema:"Their handle"`
	Kind string `json:"kind" jsonschema:"person or agent"`
}

// SelectedItems reads the items picked in a selection: none when the
// selection holds none (the whole location is read).
func SelectedItems(selection map[string]any) []SelectedItem {
	raw, ok := selection[SelectionItems].([]any)
	if !ok {
		return nil
	}
	var out []SelectedItem
	for _, x := range raw {
		m, ok := x.(map[string]any)
		if !ok {
			continue
		}
		key, _ := m["key"].(string)
		if key == "" {
			continue
		}
		title, _ := m["title"].(string)
		with, _ := m["with_children"].(bool)
		parent, _ := m["parent"].(string)
		it := SelectedItem{Key: key, Title: title, WithChildren: with, Parent: parent}
		if kb, ok := m["kept_by"].(map[string]any); ok {
			id, _ := kb["id"].(string)
			kind, _ := kb["kind"].(string)
			if id != "" {
				it.KeptBy = &Keeper{ID: id, Kind: kind}
			}
		}
		it.KeptAt, _ = m["kept_at"].(string)
		out = append(out, it)
	}
	return out
}

// ConnectorBrowseInput is what every connector's browse method takes.
type ConnectorBrowseInput struct {
	Location map[string]any `json:"location"         jsonschema:"Where to look: the source's location as its connector's form shapes it"`
	Parent   string         `json:"parent,omitempty" jsonschema:"The key of the item whose children are listed; empty = the top level"`
	Query    string         `json:"query,omitempty"  jsonschema:"Only the candidates whose title holds it - searched below parent"`
	Root     string         `json:"root,omitempty"   jsonschema:"The project's root: absolute; what a relative location is relative to"`
	From     *FromItems     `json:"from,omitempty"   jsonschema:"What the other source it reads from kept - for a connector declaring from"`
}

// ConnectorBrowseOutput is what every connector's browse method answers.
type ConnectorBrowseOutput struct {
	Items []BrowseItem `json:"items"`
	// Match is how a connector reading from another source finds the items
	// of one of that source's values (« valeur du tag appcode »): the line a
	// take tab's link band draws under the arrow. Optional.
	Match string `json:"match,omitempty" jsonschema:"How it finds the items of a value of the source it reads from - one line shown under the link band's arrow"`
}

// BrowseItem is one candidate a browse lists.
type BrowseItem struct {
	Key         string `json:"key"                    jsonschema:"What a selection names it by - the connector's own key"`
	Title       string `json:"title"`
	Kind        string `json:"kind,omitempty"         jsonschema:"What it is in the connector's words (page or space or folder or file …)"`
	URI         string `json:"uri,omitempty"          jsonschema:"Where it is"`
	HasChildren bool   `json:"has_children,omitempty" jsonschema:"It holds candidates a browse with it as parent lists"`
	// The item roles a browse shares with a search.
	Parent   string            `json:"parent,omitempty"   jsonschema:"The key of the item it sits under"`
	Path     []string          `json:"path,omitempty"     jsonschema:"The titles above it - where it is"`
	Detail   string            `json:"detail,omitempty"   jsonschema:"One line saying what it is"`
	Badges   []string          `json:"badges,omitempty"   jsonschema:"The names of the declared badges it carries"`
	Attrs    map[string]string `json:"attrs,omitempty"    jsonschema:"Its attributes by name (an application's code) - read by another source and by displays and agents"`
	Modified string            `json:"modified,omitempty" jsonschema:"When it last changed at its source - RFC 3339"`
	// Open: a connector's default scope (Confluence's default space) - the
	// picker's « Parcourir » opens it on its first draw.
	Open bool `json:"open,omitempty" jsonschema:"The picker opens it on its first draw - the connector's default scope"`
	// Picked and WithChildren are the source's saved choice, set by
	// SourceBrowse when it browses a source (a connector leaves them unset):
	// the pick tree opens with what was picked ticked.
	Picked       bool `json:"picked,omitempty"        jsonschema:"It is in the source's saved selection"`
	WithChildren bool `json:"with_children,omitempty" jsonschema:"It was picked with its descendants"`
	// Generated and Generator: the connector recognised a generator's mark
	// on it - a document KaTyoti or another tool produced.
	Generated bool   `json:"generated,omitempty" jsonschema:"It carries a generator's mark"`
	Generator string `json:"generator,omitempty" jsonschema:"What produced it - from its mark"`
}

// ConnectorSyncInput is what every connector's sync (and test) method takes.
type ConnectorSyncInput struct {
	Location  map[string]any `json:"location"            jsonschema:"Where to read: the source's location as its connector's form shapes it"`
	Selection map[string]any `json:"selection,omitempty" jsonschema:"What to take from there: the source's selection - items picked one by one under items"`
	Dest      string         `json:"dest"                jsonschema:"The folder to write into: absolute; the connector owns what is under it"`
	Root      string         `json:"root,omitempty"      jsonschema:"The project's root: absolute; what a relative location is relative to"`
	From      *FromItems     `json:"from,omitempty"      jsonschema:"What the other source it reads from kept - for a connector declaring from"`
}

// ConnectorSyncOutput is what every connector's sync method answers.
type ConnectorSyncOutput struct {
	Items  []ConnectorItem `json:"items"`
	Errors []string        `json:"errors,omitempty" jsonschema:"What could not be read; the items read are kept"`
}

// ConnectorItem is one thing a sync brought: data typed by the data-type its
// connector declares for its kind, rich markdown text with its images, or
// both - each named by its file. The files follow the names
// the knowledge base reads: the data <stem>.data.json, the
// text <stem>.md, the two side by side under one stem when the item brings
// both - a sync refuses an item named otherwise (CheckNames).
type ConnectorItem struct {
	Path string `json:"path,omitempty"  jsonschema:"The file written - relative to dest; empty: its text file, else its data file"`
	// Kind, Data and Text say what the item brings.
	Kind  string `json:"kind,omitempty" jsonschema:"What it is in the connector's words (application or page) - its data is typed by the data-type the connector declares for it"`
	Data  string `json:"data,omitempty" jsonschema:"Its data file (JSON) named <stem>.data.json - relative to dest; beside its text file <stem>.md when it brings both"`
	Text  string `json:"text,omitempty" jsonschema:"Its text file (rich markdown with its images beside it) named <stem>.md - relative to dest"`
	URI   string `json:"uri"             jsonschema:"Where it comes from (confluence://… or https://… or file://…)"`
	Title string `json:"title,omitempty" jsonschema:"Its title"`
	// Key is the selection key it was brought for (a page's id, a fact
	// sheet's) - each item applied says whether it was read.
	Key string `json:"key,omitempty" jsonschema:"The key of the item picked it was brought for - empty when the connector does not say"`
	// Generated and Generator: the connector recognised a generator's mark.
	Generated bool   `json:"generated,omitempty" jsonschema:"It carries a generator's mark"`
	Generator string `json:"generator,omitempty" jsonschema:"What produced it - from its mark"`
}

// The names of an item's files: what the knowledge base
// reads - an item's text <stem>.md and, with no text, its data
// <stem>.data.json; a data file beside its text is the same item.
const (
	ItemDataSuffix = ".data.json"
	ItemTextSuffix = ".md"
)

// CheckNames says why the item's data and text files are not named as the
// knowledge base reads them: a data file not ending .data.json, a text file
// not ending .md, or both not under one stem. Nil when they are.
func (it ConnectorItem) CheckNames() error {
	if it.Data != "" && !strings.HasSuffix(strings.ToLower(it.Data), ItemDataSuffix) {
		return fmt.Errorf("its data file %q is not named <stem>%s", it.Data, ItemDataSuffix)
	}
	if it.Text != "" && !strings.HasSuffix(strings.ToLower(it.Text), ItemTextSuffix) {
		return fmt.Errorf("its text file %q is not named <stem>%s", it.Text, ItemTextSuffix)
	}
	if it.Data != "" && it.Text != "" {
		stem := it.Data[:len(it.Data)-len(ItemDataSuffix)]
		if want := stem + ItemTextSuffix; !strings.EqualFold(it.Text, want) {
			return fmt.Errorf("its text file %q is not beside its data file %q - want %q", it.Text, it.Data, want)
		}
	}
	return nil
}

// MainFile is the item's file the anchor reads: its path, else its text
// file, else its data file.
func (it ConnectorItem) MainFile() string {
	switch {
	case it.Path != "":
		return it.Path
	case it.Text != "":
		return it.Text
	}
	return it.Data
}

// Files are the files an item names, each once.
func (it ConnectorItem) Files() []string {
	var out []string
	for _, f := range []string{it.Path, it.Text, it.Data} {
		if f != "" && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// ConnectorTestOutput says whether a location can be read.
type ConnectorTestOutput struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	// NotConfigured says the connector lacks its settings or its keys -
	// not a location it cannot reach.
	NotConfigured bool `json:"not_configured,omitempty"`
}

// InformationSourceFields are the fields of an information source.
type InformationSourceFields struct {
	Connector  string         `json:"connector"             jsonschema:"title=Connector,description=The kind of place it reads: a source-connector's name"`
	Location   map[string]any `json:"location,omitempty"    jsonschema:"title=Location,description=Where it reads - shaped by its connector's form"`
	Selection  map[string]any `json:"selection,omitempty"   jsonschema:"title=Selection,description=What it takes from there"`
	Sync       string         `json:"sync,omitempty"        jsonschema:"title=Sync,description=When it is read again,enum=manual|on-open|daily|daily-on-open"`
	LastSynced string         `json:"last_synced,omitempty" jsonschema:"title=Last synced,description=When it was last read (written by a sync)"`
	Items      int            `json:"items,omitempty"       jsonschema:"title=Items,description=How many items the last sync brought (written by a sync)"`
	Folder     string         `json:"folder,omitempty"      jsonschema:"title=Folder,description=Where its items are - relative to the project (written by a sync)"`
	Error      string         `json:"error,omitempty"       jsonschema:"title=Error,description=Why the last sync failed (written by a sync)"`
	// Held is a choice kept without being read: the items
	// proposed or picked, waiting for the carrier to apply them all; a sync
	// reads the selection, never them.
	Held []SelectedItem `json:"held,omitempty" jsonschema:"title=Held,description=The items proposed or picked and kept without being read until the carrier applies them - a sync never reads them"`
	// States says where each item applied stands, by its key
	//
	States map[string]ItemState `json:"states,omitempty" jsonschema:"title=States,description=Where each item applied stands - read or waiting or removed or failed - by its key"`
	// Its own search settings, edited in its « Réglages »:
	// the keywords it searches by beside its holder's, and how many items a
	// proposal of it answers at most.
	Keywords   []string `json:"keywords,omitempty"    jsonschema:"title=Keywords,description=Keywords it searches by beside its holder's"`
	ProposeMax int      `json:"propose_max,omitempty" jsonschema:"title=Proposal bound,description=How many items a proposal of it answers at most - ProposeMax when unset"`
	// Its connection as last tested from its « Réglages ».
	TestedAt  string `json:"tested_at,omitempty" jsonschema:"title=Tested at,description=When its connection was last tested"`
	Reachable bool   `json:"reachable,omitempty" jsonschema:"title=Reachable,description=Whether its connection answered when last tested"`
	// FromSeen is, for a source whose connector reads from another source,
	// the values that other source kept at this source's last save
	// : a value kept since is new to it - its take tab marks
	// it.
	FromSeen []string `json:"from_seen,omitempty" jsonschema:"title=From values seen,description=The values the source it reads from kept at its last save - a value kept since is new to it"`
	// IDColumns is, by item kind, the attribute « Contenu lu » leads that
	// kind's table and filter with, IDColumnNone for no
	// id column, or IDColumnUID for the entity's uid chosen explicitly - a
	// choice a carrier's default never replaces; a kind it does not name is
	// led by the entity's uid too, until a carrier gives it a default.
	IDColumns map[string]string `json:"id_columns,omitempty" jsonschema:"title=Id columns,description=By item kind the attribute its read table leads with and filters on - none for no id column"`
}

// IDColumnNone is the id column a kind's read table draws none of.
const IDColumnNone = "none"

// IDColumnUID is the id column « Par défaut (identifiant) » chooses: the
// entity's uid, kept as a choice of its own so that a carrier's default does
// not come back over it (an unnamed kind is one a carrier may still fill).
const IDColumnUID = "default"

// ItemState is where an item applied stands: read, waiting, removed or
// failed, and why.
type ItemState struct {
	State string `json:"state" jsonschema:"description=read or waiting or removed or failed"`
	Why   string `json:"why,omitempty" jsonschema:"description=Why - an error or what is awaited"`
}

// The states of an item applied.
const (
	ItemRead    = "read"
	ItemWaiting = "waiting"
	ItemRemoved = "removed"
	ItemFailed  = "failed"
)

// SourceSyncInput names the source to read again.
type SourceSyncInput struct {
	ID string `json:"id" cli:"id" jsonschema:"The information source's id; required"`
}

// SourceSyncOutput says what the sync did, in the {ok, message} shape a pane
// shows.
type SourceSyncOutput struct {
	OK      bool     `json:"ok"`
	Message string   `json:"message"`
	State   string   `json:"state"            jsonschema:"synced or failed"`
	Folder  string   `json:"folder,omitempty" jsonschema:"Where the items are - relative to the project"`
	Items   int      `json:"items"`
	Anchors []string `json:"anchors,omitempty" jsonschema:"The source-documents recording what it brought"`
	Removed []string `json:"removed,omitempty" jsonschema:"The source-documents archived: what an earlier sync brought and this one did not (an item unpicked; a file gone)"`
	Keys    []string `json:"keys,omitempty"    jsonschema:"The keys of the items picked it brought - when its connector says them"`
	Changed []string `json:"changed,omitempty" jsonschema:"The items read again with another content than before - each raises source.changed on the carriers"`
	Errors  []string `json:"errors,omitempty"`
}

// SourceCheckInput is a before hook's default payload: the information source
// as it is about to be.
type SourceCheckInput struct {
	Object *cwmapi.Object `json:"object,omitempty" jsonschema:"description=The source as it is about to be"`
}

// SourceTestInput is a location to check before a source is saved, or a
// saved source whose connector, location and selection are checked - the
// source's « Tester la connexion » action.
type SourceTestInput struct {
	ID        string         `json:"id,omitempty"       cli:"id"        jsonschema:"A saved information source - its connector and location are checked; or connector and location"`
	Connector string         `json:"connector,omitempty" cli:"connector" jsonschema:"The connector's name - with location when no source is named"`
	Location  map[string]any `json:"location,omitempty"                 jsonschema:"The location to check"`
	Selection map[string]any `json:"selection,omitempty"                jsonschema:"The selection to check"`
}

// SourceBrowseInput lists the candidates of a source's location, or of a
// location being chosen (connector and location, before a source is saved).
type SourceBrowseInput struct {
	ID        string         `json:"id,omitempty"        cli:"id"        jsonschema:"The information source; or connector and location"`
	Connector string         `json:"connector,omitempty" cli:"connector" jsonschema:"The connector's name - with location when no source is named"`
	Location  map[string]any `json:"location,omitempty"                  jsonschema:"The location - with connector when no source is named"`
	Parent    string         `json:"parent,omitempty"    cli:"parent"    jsonschema:"The key of the item whose children are listed; empty = the top level"`
	Query     string         `json:"query,omitempty"     cli:"query"     jsonschema:"Only the candidates whose title holds it"`
	// Object is the carrier a picker browses for: its own
	// source for the connector gives the location, and what its source of
	// the connector read from was kept.
	Object string `json:"object,omitempty" cli:"object" jsonschema:"The carrier - its own source for the connector gives the location browsed"`
}

// SourceSelectInput records the items picked on a source.
type SourceSelectInput struct {
	ID    string         `json:"id"    cli:"id" jsonschema:"The information source; required"`
	Items []SelectedItem `json:"items"          jsonschema:"The items picked - the whole choice: an item left out is unpicked"`
	// By is who picks them: the items it newly keeps
	// record them; an item already kept keeps its keeper.
	By *Keeper `json:"by,omitempty" jsonschema:"Who picks them - a person or an agent"`
}

// SourceSelectOutput says what the source now holds, in the {ok, message}
// shape a pane shows.
type SourceSelectOutput struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Items   int    `json:"items"`
}

// ConnectorsInput is empty: the connectors of the universe.
type ConnectorsInput struct{}

// ConnectorsOutput lists the connectors contributed.
type ConnectorsOutput struct {
	Connectors []ConnectorDecl `json:"connectors"`
}
