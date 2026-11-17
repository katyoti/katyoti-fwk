// SPDX-License-Identifier: Apache-2.0

// Package api is the sources service contract. A SOURCE is a document outside
// the workspace — a specification, a page, a file — recorded as a
// `source-document` object: where it is (`source_uri`), which section is
// anchored (`section_path`) and a hash of what was read (`content_hash`). An
// object produced from it carries a `derived-from` edge to it. Ingest reads
// the source and records the document; Reanchor reads it again and says whether
// the section still matches, drifted or disappeared — and which objects were
// derived from it; Stale lists a module's documents left stale or missing.
// Both read and hash with one computation, and a `file://` source is read only
// inside the project.
package api

import (
	"context"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// The type and the edge the sources pack declares.
const (
	SourceDocumentType = "source-document"
	DerivedFromEdge    = "derived-from"
)

// SourcesServiceDef describes the service for KTSP/CLI auto-generation.
var SourcesServiceDef = meta.ServiceDef{
	Name:    "sources",
	Short:   "Outside documents a workspace depends on: read a source again and report what drifted",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Ingest":   meta.Method("Read a source's section, hash and snapshot it, and create or update the source-document recording it"),
		"Reanchor": meta.Method("Read a source-document's source again, record match / drift / missing, and list the objects derived from it"),
		"Stale":    {Short: "The source-documents of a module whose anchor is stale or missing, with the objects derived from each", ReadOnly: true},
		"SourceSync": {Short: "Read an information source again through its connector: its items written into the module and anchored",
			Long: "Calls the sync method of the source's connector (a source-connector contribution) with its location and selection; the connector writes into <module folder>/input/<source>/; each item is anchored as a source-document under the source. The source's facts (last synced, items, folder, error) are written and its state moves to synced, or to failed with the error. Answers {ok, message}. Long: from the action surface it answers its run and its panel at once, the sync and each item anchored reported into the panel.", TimeoutS: 660, LongRunning: true},
		"SourceCheck": {Short: "Check an information source before it is saved: its connector's check", Long: "The before_create and before_update hook of information-source (sources owns the type): given the source as it is about to be, refuses a source about to be created under a holder that asks for one source per connector (it carries the capability one-source-per-connector, as the as-is study v2) and already reads its connector through another active source (« <source> est déjà dans l'étude », « dans le module » under a module; any other holder is never refused; an archived source frees its connector, a source written again is never refused), then asks its connector's check method when it declares one and answers its accept or its reason; a connector without one, or a source naming none, is accepted.", ReadOnly: true, TimeoutS: 30},
		"SourceTest":  {Short: "Check that a connector can read a location", Long: "Calls the connector's test method with the location - or, given a saved information source (id), with its own connector, location and selection: the source's « Tester la connexion » action; answers {ok, message}. A connector without one answers that it cannot be checked before a sync.", ReadOnly: true, TimeoutS: 75},
		"Connectors":  {Short: "The source connectors contributed in the universe", ReadOnly: true},
		"FolderSync":  {Short: "The local-folder connector: mirror a folder's files into dest", Long: "A source-connector's sync method: copies the files of location.path (absolute, or relative to the project) matching selection.include (default every file) into dest, and answers each as an item.", TimeoutS: 600},
		"SourcesOf": {Short: "Where a carrier's knowledge stands: its information sources and its knowledge base", ReadOnly: true,
			Long: "The sources of a module carrying the knowledge capability - its child information sources and those that feed it - with their state, last sync and items, and its knowledge base: the path the ktw-kb methods take, whether it is set up and its size. The « Sources » tab's source."},
		"WaitingCount": {Short: "How many of a carrier's own sources wait on the person - the « Sources » tab's badge", ReadOnly: true,
			Long: "The carrier's own information sources (its children; a source that only feeds it waits at its own carrier) that WaitsOnPerson: its last read failed (its error set) or it holds an item in failure. Answers {count, color: red}; none at 0 - the panel's count, no badge then."},
		"SyncAll": {Short: "Sync every source of a carrier, then reindex its knowledge base",
			Long: "SourceSync on each source of the carrier (a failing one is reported and the others go on), then ktw-kb kb/run on the carrier's .kb. A knowledge base that is not set up is reported, never created. Answers {ok, message}. Long: from the action surface it answers its run and its panel at once, each source and the reindex reported into the panel.", TimeoutS: 1800, LongRunning: true},
		"Reindex": {Short: "Reindex a carrier's knowledge base", Long: "ktw-kb kb/run on the carrier's .kb: what its sources brought into input/ is indexed. Answers {ok, message}. Long: from the action surface it answers its run and its panel at once, the reindex reported into the panel.", TimeoutS: 660, LongRunning: true},
		"Use": {Short: "Start reading a carrier's knowledge from a contributed connector",
			Long: "Creates under the carrier one information source reading through the connector - declared, its location and selection chosen next - unless one of the carrier's own sources reads through it already, which is then answered. With on false it stops using it: that source is removed (Remove). Answers {ok, message, source, created}."},
		"Deposit": {Short: "Deposit a document on a carrier: it joins its « Documents » source and its knowledge base",
			Long: "The file (name, base64 content) is written into the carrier's documents/ folder, picked in its « Documents » source (the documents connector, made on the first deposit) with who deposited it and when, then read and the knowledge base reindexed. It never replaces a document: a taken name goes under « X (2).ext », said in the message. A name that is no plain local file name (a folder, * ? \" < > |, a Windows-reserved name), a content not base64 or a file over 5 MB is refused. Answers {ok, message, source, document}.", TimeoutS: 660},
		"DocumentsPage": {Short: "A carrier's « Documents » section as a page region", ReadOnly: true,
			Long: "The section any page of a carrier holds (the « Sources » tab, a study's « Schémas et documents »): « Déposer un document » - a file chosen on the machine handed to Deposit with who presses it - then each deposited document, who deposited it and whether it is in the base, opened on the files panel. Answers {dslTree, data, title}."},
		"Remove": {Short: "Take a source off its carrier: what it brought leaves the knowledge base",
			Long: "A carrier's own source: every file it mirrored into input/ is deleted, its source-documents and then the source itself archived, and the carrier's knowledge base reindexed - its documents leave it. A source that only feeds the carrier (carrier named): the feeds edge is deleted and the source left alone. Answers {ok, message, removed}.", TimeoutS: 660},
		"Pickers": {Short: "A carrier's source connectors as generic pickers", ReadOnly: true,
			Long: "Every contributed connector that picks - by a search, by browsing, or from what another source kept - as the display library's picker draws it: its label and icon, its unit, list or tree, badges and derived note, its scopes, and the search and the browse to call where it declares them - sources' Search and SourceBrowse with the connector in args. The connectors method of a source-set field."},
		"Search": {Short: "Search a connector's items for a picker", ReadOnly: true, TimeoutS: 60,
			Long: "Calls the connector's search method with the free text and the carrier's own source location for it (none: the connector's whole reach); answers {items}, each with its key, title, parent, badges - the generic picker's items."},
		"Propose": {Short: "A connector's proposal from a context, for a carrier", ReadOnly: true, TimeoutS: 120,
			Long: "Calls the connector's propose method with the terms and the free text and the carrier's own source location for it; answers {items}, each with why and a score."},
		"ProposalWizard": {Short: "The wizard proposing items from every chosen source", ReadOnly: true, TimeoutS: 600,
			Long: "A page DSL wizard: one step per chosen source (every connector that can propose when none is named), its picker pre-ticked with the connector's proposal from the terms and the text; the person adds, removes or presses « Suivant ». A source whose proposal failed shows its picker empty and is named in errors."},
		"ProposalPage": {Short: "The proposal wizard as a page region, finishing with Apply", ReadOnly: true, TimeoutS: 600,
			Long: "ProposalWizard drawn as a region's page: one step per chosen source proposing from the terms and the text - a list a page edits -, a proposal that failed said above it; the wizard's finish runs Apply on the carrier, so its knowledge base is built from what it holds. Answers {dslTree, data, title}."},
		"SearchSources": {Short: "Search every source in use at once, each answering as it comes with its state", ReadOnly: true, TimeoutS: 600, LongRunning: true,
			Long: "Every connector that picks (or those named) proposes from the terms and the text in parallel - its propose, else its search or browse once per term -, each source emitted as it moves: searching, waiting on the source it reads from, n found, nothing new, unreachable with why, not configured with where. A connector declaring from waits for that source's answer and follows what it proposed beside what the carrier kept. With a carrier what it already holds is left out (« Chercher encore »); with none nothing is held or created. only searches one source again alone (« Réessayer »). Long: from the action surface it answers its run and its panel at once, each answer laid over the panel's source-search node. Answers {sources}."},
		"Hold": {Short: "Keep a connector's chosen items for a carrier - and read them at once with read", TimeoutS: 1800,
			Long: "Writes the items as the held choice of the carrier's own source for the connector - created, declared, when it has none -: nothing is read; a sync reads the selection, never them, until the carrier applies them. The whole choice: an item left out is no longer held. With read (a picker's « Enregistrer et lire ») the source takes the choice at once and is synced - what it adds read, what it drops removed - and the knowledge base reindexed; the message says how many items were read and, for those that could not be (no credential, the source unreachable), why and that the next sync of the source reads them. Answers {ok, message, source, held, result}."},
		"Apply": {Short: "Apply every held choice of a carrier",
			Long: "Each of the carrier's own sources holding a choice takes it as its selection, its held choice emptied, and is synced: what it adds is read, what it drops leaves the knowledge base; each item records whether it is read, removed or failed and why; the knowledge base is then reindexed. Answers {ok, message, sources, results}.", TimeoutS: 1800},
		"SyncDue": {Short: "Re-read a carrier's sources whose re-reading mode is due",
			Long: "The sources of the carrier whose sync mode is the one named - daily (not read for 20 hours) or on-open (not read for 10 minutes) - are synced, then the knowledge base reindexed. The knowledge capability's timer runs it daily; opening the « Sources » tab starts it for on-open. Answers {ok, message, synced}.", TimeoutS: 1800},
		"Status": {Short: "Whether a connector is configured and reachable, and where it is configured", ReadOnly: true, TimeoutS: 75,
			Long: "Calls the connector's test with no location: configured (its settings and keys are set) and reachable (it answers), its message, and the link a picker offers when it is not (configure: label, hint, keys)."},
		"UseSet": {Short: "Make a source-set value the carrier's sources, read in the background", TimeoutS: 1800,
			Long: "Each connector of the value becomes one of the carrier's sources (one per connector) holding its items, then the choice is applied - what it adds read, what it drops removed - in the background unless wait: what a creation form's source-set field does at creation. Answers {ok, message, sources}."},
		"ConnectorPanel": {Short: "One connector's panel for a carrier - the Sources board's region", ReadOnly: true, TimeoutS: 60,
			Long: "The page the « Sources » board draws beside the selected connector: its head - label, what is picked -, a banner and « Utiliser » while it is not used, then the generic picker - search, « Parcourir », its status banner, the derived note, the selection with each item's state, « Enregistrer et lire » keeping the choice and reading it at once - as the approved mock-up shows it."},
		"SourceDetail": {Short: "An information source's detail as a page region - what an opened source card shows", ReadOnly: true,
			Long: "The same for every source, whatever its connector: its state in a word (à jour, changée, n erreurs, injoignable, pas encore lue, à configurer), when it was last read and how many items it brought, what to do when it failed, then each item it brought or keeps with its state - synchronisé, à lire (kept, not read yet), à retirer, en échec with why -, an item read opening its source-document. The connector panel of a used source holds it. Answers {dslTree, data, title}."},
		"SourceTab": {Short: "One of an opened source's three sub-tabs as a page region - what it takes, « Contenu lu », « Réglages »", ReadOnly: true, TimeoutS: 60,
			Long: "The same whatever its connector, drawn by the sub-tab panels under a holder carrying source-sub-tabs: tab take is what the source takes - the connector's generic picker bound to the source's module, its selection with each item's state, « Enregistrer et lire » keeping the choice and reading it at once; tab read is « Contenu lu » - the source's detail, its state and each item it brought with its state, an item read opening its source-document; tab settings is « Réglages » - how it searches and reads (mock-up v6): one form read in rows, each value edited in place and kept at once by SettingsSave (what it reads, its scopes when its connector declares some, the holder's keywords - its pencil opening the holder -, its own keywords and its proposal bound when it proposes, what it takes when it picks - its pencil opening the take tab -, the source it reads from, the connection it reads through when its connector names one - its choices from the connector's list -, the location fields its connector's location data-type lets it edit (its ui-form settings), when it is read again, its connection with « Tester »; the scopes row's choices from ScopeOptions), « Supprimer la source » under it - a quiet trash button, what removing it does said beside it -, then the connector's location form kept by LocationSave for a connector without a picker - a picker's selection is never asked again in a location field. Answers {dslTree, data, title}."},
		"SourceTakeFunction": {Short: "source_take(): the name of an opened source's « what it takes » tab - its connector's take label, « Ce qu'elle prend » when it declares none", ReadOnly: true,
			Long: "The knowledge capability's function, evaluated over an information source: the take label its connector declares (« Pages prises », « Fichiers déposés »), « Ce qu'elle prend » when the connector declares none or the object is no information source. The take panel's name is ${source_take()}."},
		"SourceSubTabsFunction": {Short: "source_sub_tabs(): whether an opened source shows its three sub-tabs - its holder carries source-sub-tabs - or the one « Source » tab", ReadOnly: true,
			Long: "The knowledge capability's function, evaluated over an information source: true when its holder carries the capability source-sub-tabs (the as-is study v2 opts in) - the three sub-tabs take, « Contenu lu » and « Réglages » show -, false under any other holder - a module, an as-is study v1 - or when the object is no information source or its holder cannot be read: the one « Source » tab shows."},
		"NewSourcePage": {Short: "An information source's create page under its holder - each contributed connector a choice, one the holder has greyed with its reason", ReadOnly: true, TimeoutS: 60,
			Long: "The information-source type's create_task: given the holder (parent), a module carrier, one row per contributed connector - its label, icon and description - whose « Ajouter » runs sources/use on the holder with that connector; under a holder carrying one-source-per-connector, a connector it already has a source of is shown disabled with its reason (« déjà dans l'étude » - one source per connector: open it to change what it takes). Answers {dslTree, data, title}."},
		"KnowledgeBase": {Short: "A holder's knowledge base - every item read from all its sources once, with the source it came from, filtered by source, kind and state", ReadOnly: true, TimeoutS: 60,
			Long: "The knowledge base of a holder carrying the knowledge capability, the same for a v1 or a v2 study or any holder: each source-document its information sources brought, once, with the source it came from (its name and its connector's label and colour), its kind (document - text - or record - fields) and its state (read, or stale - changed since it was read); counts by kind and state, filters by source, kind and state (each a query over the panel's input), the sources with stale items to update, an item opening its source-document in a drawer. Answers a page {kind, dslTree, data, title}."},
		"LocationSave": {Short: "Keep where a source reads, from its connector's location form",
			Long: "The location a connector panel's form submits - drawn from the connector's location data-type, its ui-form groups, secrets and folded advanced fields included - becomes the information source's location; the source's save gate (its connector's check) refuses one it cannot read, with its reason. Answers {ok, message}."},
		"SettingsSave": {Short: "Keep an opened source's « Réglages » as its form edits them in place",
			Long: "The value the « Réglages » form keeps at each change: the source's own keywords (searched beside its holder's), its proposal bound (1 to 100, the default when 0), its scopes and the connection it reads through and the location fields its connector lets it edit (kept in its location - the rest of it kept) and its re-reading mode (manual, on-open, daily, or daily-on-open - due both every day and at each opening); the rows it only reads are ignored. Through the source's save gate. Answers {ok, message}."},
		"SettingsTest": {Short: "Test an opened source's connection and record when - « Tester » of its « Réglages »", TimeoutS: 75,
			Long: "Calls the source's connector test with its location and selection, records on the source when it was tested and whether it answered, and answers in the check grammar a read row draws: {ok, text} when it answered, {ok: false, message} when it did not."},
		"SourceFetch": {Short: "Open one item of a source - its roles, its data and its text", ReadOnly: true, TimeoutS: 75,
			Long: "Calls the fetch method of the source's connector (or of a connector named with a location) with the item's key: the item by its roles, its data - typed by the data-type the connector declares for its kind, named in data_type - and its markdown text, either or both. Through a source only an item it keeps opens - one a sync brought under it, picked or held -, any other key refused saying why; a connector named with a location opens any key. A connector declaring no fetch answers that it cannot open an item."},
		"DocumentDetail": {Short: "A source-document opened - the item it records, its data, its text and the objects derived from it", ReadOnly: true, TimeoutS: 75,
			Long: "What a source-document's pane draws through the display library's item-detail node: a document a sync brought under an information source with its item's key is opened through that source's fetch (SourceFetch) - the item by its roles, its data and its data-type, its text; an anchor with no key, or one whose fetch fails (said in note), answers what is stored - its text file or its content snapshot, its data file and data-type, the item from its name, kind and source URI. used lists the objects carrying a derived-from edge to it, each with its name and type. Answers {item, data, data_type, text, used, note}."},
		"DocumentPage": {Short: "A source-document opened as a knowledge-base item - the drawer an item of the knowledge base opens", ReadOnly: true, TimeoutS: 75,
			Long: "What a source-document's « Élément » tab draws, the same for every connector: « Base de connaissance · élément », the item's title, its kind (document or record) and state (read, or stale) and what it is; « Vient de » - the information source it came from as a card opening it (its connector, its name, its connector's label, its id and when it was read); for an item of a connector reading from another source, how the link found it; a record's fields as rows (« Champs »), a document's start (« Extrait ») with a note when its source changed it since it was read; the objects reading it; « Ouvrir dans <système> » (the item's address in its system) and « Ouvrir la source ». Built on DocumentDetail. Answers a page {kind, dslTree, data, title}."},
		"Scopes": {Short: "A carrier's scopes for a connector - the configuration's defaults and those it added", ReadOnly: true, TimeoutS: 60,
			Long: "Calls the connector's scopes list method with the carrier's own source location for it and the project root: the scopes its configuration names (default) and those the carrier added (kept in the location's scopes field) - what a picker's scope chips list. A connector declaring no scopes answers none. Answers {scopes}."},
		"ScopeOptions": {Short: "An opened source's scopes as the choices of its « Réglages » scopes row", ReadOnly: true, TimeoutS: 60,
			Long: "Calls the scopes list method of the source's connector with the source's location and the project root and answers each scope as a choice: its key the value and its key and title the label. A connector declaring no scopes list answers none. Answers {options}."},
		"ScopeAdd": {Short: "Keep a scope a carrier adds for a connector",
			Long: "Appends the scope's key, once, to the scopes field of the carrier's own source location for the connector, through the source's save gate; the next Scopes answers it. Refused when the connector lets no carrier add a scope, the key is blank, or the carrier has no source of its own for the connector. Answers {ok, message}."},
		"SourceBrowse": {Short: "List a source's candidates one level at a time - « Parcourir » of its picker", ReadOnly: true, TimeoutS: 90,
			Long: "Calls the browse method of the source's connector (or of a connector and its location) with the parent and the query, and answers one level of candidates, those the source already picked ticked. A connector's level can be slow - a few calls an item - so a caller waits beyond the connector's own bound."},
		"SourcePicked": {Short: "A source's choice as a picker holds it", ReadOnly: true,
			Long: "The items picked in an information source's selection (and those held), each with its state once applied - the source's « Choisir » tab's source, its picker's selection."},
	},
}

// SourcesService ingests, re-anchors and reports source documents.
type SourcesService interface {
	Ingest(ctx context.Context, in IngestInput) (IngestOutput, error)
	Reanchor(ctx context.Context, in ReanchorInput) (ReanchorOutput, error)
	Stale(ctx context.Context, in StaleInput) (StaleOutput, error)
	// Information sources (connectors.go).
	SourceSync(ctx context.Context, in SourceSyncInput) (SourceSyncOutput, error)
	SourceTest(ctx context.Context, in SourceTestInput) (ConnectorTestOutput, error)
	// SourceCheck is sources' save gate on an information source: its
	// connector's check, when it declares one.
	SourceCheck(ctx context.Context, in SourceCheckInput) (cwmapi.HookAccept, error)
	Connectors(ctx context.Context, in ConnectorsInput) (ConnectorsOutput, error)
	// FolderSync is the local-folder connector's sync method.
	FolderSync(ctx context.Context, in ConnectorSyncInput) (ConnectorSyncOutput, error)
	// Browsing and picking (connectors.go).
	SourceBrowse(ctx context.Context, in SourceBrowseInput) (ConnectorBrowseOutput, error)
	SourceSelect(ctx context.Context, in SourceSelectInput) (SourceSelectOutput, error)
	// FolderBrowse is the local-folder connector's browse method.
	FolderBrowse(ctx context.Context, in ConnectorBrowseInput) (ConnectorBrowseOutput, error)
	// The knowledge capability (knowledge.go).
	SourcesOf(ctx context.Context, in SourcesOfInput) (SourcesOfOutput, error)
	// WaitingCount is the « Sources » tab's badge.
	WaitingCount(ctx context.Context, in SourcesOfInput) (WaitingCountOutput, error)
	SyncAll(ctx context.Context, in SyncAllInput) (SyncAllOutput, error)
	Reindex(ctx context.Context, in ReindexInput) (ReindexOutput, error)
	Use(ctx context.Context, in UseInput) (UseOutput, error)
	// Deposit gives a carrier a document.
	Deposit(ctx context.Context, in DepositInput) (DepositOutput, error)
	// Remove takes a source off its carrier.
	Remove(ctx context.Context, in RemoveInput) (RemoveOutput, error)
	// Pickers and Search bind the generic picker to the connectors
	//
	Pickers(ctx context.Context, in PickersInput) (PickersOutput, error)
	Search(ctx context.Context, in SearchInput) (ConnectorSearchOutput, error)
	// Propose and ProposalWizard propose items from a context
	//
	Propose(ctx context.Context, in ProposeInput) (ConnectorProposeOutput, error)
	ProposalWizard(ctx context.Context, in ProposalWizardInput) (ProposalWizardOutput, error)
	// SearchSources searches every source in use at once.
	SearchSources(ctx context.Context, in SearchSourcesInput) (SearchSourcesOutput, error)
	// DocumentsPage is the « Documents » section as a page region.
	DocumentsPage(ctx context.Context, in DocumentsPageInput) (ConnectorPanelOutput, error)
	// ProposalPage is ProposalWizard as a page region.
	ProposalPage(ctx context.Context, in ProposalPageInput) (ConnectorPanelOutput, error)
	// Hold and Apply keep a choice without reading it, then apply it
	//
	Hold(ctx context.Context, in HoldInput) (HoldOutput, error)
	Apply(ctx context.Context, in ApplyInput) (ApplyOutput, error)
	// SyncDue runs the daily and on-open re-reading modes.
	SyncDue(ctx context.Context, in SyncDueInput) (SyncDueOutput, error)
	// Status says whether a connector is configured and reachable
	//
	Status(ctx context.Context, in StatusInput) (StatusOutput, error)
	// SourcePicked is a source's choice as its picker holds it
	//
	SourcePicked(ctx context.Context, in SourcePickedInput) (SourcePickedOutput, error)
	// UseSet makes a source-set value the carrier's sources.
	UseSet(ctx context.Context, in UseSetInput) (UseSetOutput, error)
	// ConnectorPanel is one connector's panel for a carrier.
	ConnectorPanel(ctx context.Context, in ConnectorPanelInput) (ConnectorPanelOutput, error)
	// SourceDetail is one source's detail as a page region.
	SourceDetail(ctx context.Context, in SourceDetailInput) (ConnectorPanelOutput, error)
	// NewSourcePage is an information source's create page under its holder
	//
	NewSourcePage(ctx context.Context, in NewSourcePageInput) (ConnectorPanelOutput, error)
	// KnowledgeBase is a holder's knowledge base as a page.
	KnowledgeBase(ctx context.Context, in KnowledgeBaseInput) (render.Def, error)
	// SourceTab is one of an opened source's three sub-tabs as a page region
	//
	SourceTab(ctx context.Context, in SourceTabInput) (ConnectorPanelOutput, error)
	// SourceTakeFunction is `source_take()`: the name of an opened source's
	// take tab, from its connector.
	SourceTakeFunction(ctx context.Context, in cwmapi.FunctionCall) (string, error)
	// SourceSubTabsFunction is `source_sub_tabs()`: whether an opened source
	// shows its three sub-tabs, its holder asking for them.
	SourceSubTabsFunction(ctx context.Context, in cwmapi.FunctionCall) (bool, error)
	// LocationSave keeps a source's location from its connector's form
	//
	LocationSave(ctx context.Context, in LocationSaveInput) (LocationSaveOutput, error)
	// SettingsSave and SettingsTest keep an opened source's « Réglages »
	// and record its connection.
	SettingsSave(ctx context.Context, in SettingsSaveInput) (SettingsSaveOutput, error)
	SettingsTest(ctx context.Context, in SettingsTestInput) (SettingsTestOutput, error)
	// SourceFetch opens one item of a source.
	SourceFetch(ctx context.Context, in SourceFetchInput) (SourceFetchOutput, error)
	// DocumentDetail is a source-document opened, what its pane's
	// item-detail node draws.
	DocumentDetail(ctx context.Context, in DocumentDetailInput) (DocumentDetailOutput, error)
	// DocumentPage is a source-document opened as a knowledge-base item, a
	// page (mock-up v6 view 17).
	DocumentPage(ctx context.Context, in DocumentDetailInput) (render.Def, error)
	// Scopes and ScopeAdd bind a picker's scope chips to a carrier
	//
	Scopes(ctx context.Context, in ScopesInput) (ConnectorScopesOutput, error)
	ScopeAdd(ctx context.Context, in ScopeAddInput) (ScopeAddOutput, error)
	// ScopeOptions answers a source's scopes as its « Réglages » row's
	// choices.
	ScopeOptions(ctx context.Context, in ScopeOptionsInput) (ScopeOptionsOutput, error)
}

// IngestInput names the source section to anchor and the source-document that
// records it.
type IngestInput struct {
	SourceDocID string   `json:"source_doc_id"          jsonschema:"The source-document's module-qualified id (module.local); created when absent and updated when present; required"`
	SourceURI   string   `json:"source_uri"             jsonschema:"Where the source lives; file://./path reads a file of the project; required"`
	SectionPath []string `json:"section_path,omitempty" jsonschema:"Heading breadcrumb of the anchored section; empty = the whole document"`
	Parent      string   `json:"parent,omitempty"       jsonschema:"On create the parent object id; empty = the module"`
	Name        string   `json:"name,omitempty"         jsonschema:"On create the document's name; empty = the last heading of section_path or the uri"`
	NewContent  string   `json:"new_content,omitempty"  jsonschema:"The source's content when the caller has it (a scheme with no fetcher); empty = read it from source_uri"`
	IngestedBy  string   `json:"ingested_by,omitempty"  jsonschema:"Persona or process recording the source"`
	Generated   bool     `json:"generated,omitempty"    jsonschema:"The document carries a generator's mark (recorded on the anchor)"`
	Generator   string   `json:"generator,omitempty"    jsonschema:"What produced it - from its mark"`
	// What a synced item brings: its kind, its data file
	// and its text file, recorded on the anchor so a view opens them.
	// ItemKey is the key of the item picked it was brought for: a source's
	// detail matches its items to their documents by it.
	ItemKey  string `json:"item_key,omitempty"  jsonschema:"The key of the item picked a sync brought it for - empty when its connector does not say"`
	ItemKind string `json:"item_kind,omitempty" jsonschema:"What the item is in its connector's words - its data is typed by the data-type the connector declares for it"`
	DataFile string `json:"data_file,omitempty" jsonschema:"Its data file - relative to the project"`
	DataType string `json:"data_type,omitempty" jsonschema:"The data-type its data is shaped by"`
	TextFile string `json:"text_file,omitempty" jsonschema:"Its text file (rich markdown) - relative to the project"`
}

// IngestOutput is the recorded anchor.
type IngestOutput struct {
	SourceDocID   string `json:"source_doc_id"`
	Created       bool   `json:"created"        jsonschema:"true when the source-document was created; false when an existing one was anchored afresh"`
	Changed       bool   `json:"changed,omitempty" jsonschema:"An existing document read with another content than its anchor's"`
	ContentHash   string `json:"content_hash"`
	SnapshotChars int    `json:"snapshot_chars" jsonschema:"Characters recorded in content_snapshot (at most 3500; fewer when the document would exceed its type's max_size_bytes)"`
}

// StaleInput names the module whose anchors are listed.
type StaleInput struct {
	Module string `json:"module" jsonschema:"The module whose source-documents are listed; required"`
}

// StaleOutput lists the module's source-documents whose last reanchor left them
// stale or missing, sorted by id.
type StaleOutput struct {
	Anchors []StaleAnchor `json:"anchors"`
}

// StaleAnchor is one source-document to reconcile and what depends on it.
type StaleAnchor struct {
	SourceDocID    string   `json:"source_doc_id"`
	AnchorStatus   string   `json:"anchor_status"              jsonschema:"stale | missing"`
	SourceURI      string   `json:"source_uri"`
	SectionPath    []string `json:"section_path,omitempty"`
	DerivedItemIDs []string `json:"derived_item_ids,omitempty" jsonschema:"Objects carrying a derived-from edge to the document"`
}

// ReanchorInput names the source-document to read again.
type ReanchorInput struct {
	SourceDocID   string `json:"source_doc_id"             jsonschema:"The source-document object id; required"`
	NewContent    string `json:"new_content,omitempty"     jsonschema:"The source's content when the caller has it; empty = read it from source_uri"`
	UpdateOnDrift bool   `json:"update_on_drift,omitempty" jsonschema:"On drift record the new content as the anchor; otherwise mark it stale"`
}

// ReanchorOutput is what the reanchor found. The source-document records the
// outcome (anchor_status); the derived objects are reported, not written.
type ReanchorOutput struct {
	OldHash        string   `json:"old_hash"`
	NewHash        string   `json:"new_hash"`
	Status         string   `json:"status"                     jsonschema:"match | drift | missing"`
	StaleItemIDs   []string `json:"stale_item_ids,omitempty"   jsonschema:"Objects derived from the document (derived-from edges) whose section drifted"`
	MissingItemIDs []string `json:"missing_item_ids,omitempty" jsonschema:"Objects derived from the document whose section is no longer found"`
}
