// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/reflect"
)

// A page (Def.Kind "page") is a tree of nodes in Def.DSLTree: its grammar is
// the display library's (app/ktw-display-lib/src/ui-builders/page/
// ui-page.types.ts, documented in modules/display/page-dsl.md). A producer
// that builds a page in Go — a panel's source answering a page instead of raw
// data — writes it with the builders below, so every sidecar emits the same
// blocks and ValidatePage holds them to the grammar.

// Node is one node of a page: its kind and its props, as the page renderer
// reads them. It marshals as the plain JSON object the GUI takes.
type Node map[string]any

// Segment is one part of a stacked gauge: how many, its color, what it
// counts - and the items it counts, listed when it is hovered, each opening
// its object.
type Segment struct {
	Value float64       `json:"value"`
	Color string        `json:"color,omitempty"`
	Label string        `json:"label,omitempty"`
	Items []SegmentItem `json:"items,omitempty"`
}

// SegmentItem is an item a segment counts: its object id and its title.
type SegmentItem struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

// Action is a declarative action on a node (a button, a list or table row, a
// KPI, a link, a badge). It does EXACTLY ONE thing, named by its verb field:
//
//   - URI runs a method through the shared action path with Args; an answer
//     following the result convention ({ok, message, open?}) is shown as
//     written, `open` navigated to; a task opens its run;
//   - Navigate opens an object, on one of its panels when Panel is set;
//   - Chat opens a chat running Task on Target (an object id);
//   - Create opens the host's new-object dialog (Type preset, under Parent —
//     the panel's object when empty);
//   - Query re-reads the panel's source with its Args laid over the source's
//     own input (a filter, a saved view): each query replaces the last one, and
//     a form's submit without Args lays the form's whole value.
//
// The host resolves the templates of Args and of the verb fields
// (`${item.<path>}`, `${value.<path>}`, `${id}`), confirms when asked, does
// the verb and re-reads the panel. Build one with RunURI, NavigateTo, ChatOn,
// CreateUnder or QueryWith.
type Action struct {
	ID       string         `json:"id,omitempty"`
	Label    string         `json:"label,omitempty"`
	Icon     string         `json:"icon,omitempty"`
	Color    string         `json:"color,omitempty"`
	URI      string         `json:"uri,omitempty"`
	Args     map[string]any `json:"args,omitempty"`
	Navigate *Navigate      `json:"navigate,omitempty"`
	Chat     *Chat          `json:"chat,omitempty"`
	Create   *Create        `json:"create,omitempty"`
	Query    *Query         `json:"query,omitempty"`
	// Tab shows a tab of this page (its panel id): run by the page itself,
	// never handed to the host.
	Tab string `json:"tab,omitempty"`
	// Select sets a key of the page's selection: run by the page itself,
	// never handed to the host; the sections reading that key re-read.
	Select *Select `json:"select,omitempty"`
	// Drawer opens a side drawer: a page node drawn by the page itself (an
	// action's form, render.ActionForm), a panel of an object the host draws
	// , or a file of an object read in place - a section, a
	// diagram, a document - the host draws.
	Drawer *Drawer `json:"drawer,omitempty"`
	// Modal opens a centred modal holding a page node drawn by the page
	// itself, an action run from it closing it.
	Modal *Modal `json:"modal,omitempty"`
	// Panel opens a display panel by its id (dp-…) in a tab of its own
	// (: a conversation of « À faire » opens its chat): handed to
	// the host, which follows it there.
	Panel string `json:"panel,omitempty"`
	// Screen opens one of the host's own screens by its name (HostScreens):
	// `todo`, « À faire » - what the rail's To do opens.
	Screen string `json:"screen,omitempty"`
	// Close closes what the page is drawn in - the host's dialog (the create
	// dialog's « Annuler »), the page's own drawer or modal; handed to the
	// host, which ignores it where the page fills a pane.
	Close     bool   `json:"close,omitempty"`
	Confirm   string `json:"confirm,omitempty"`
	Dangerous bool   `json:"dangerous,omitempty"`
}

// Drawer is what a drawer action opens: Node (a page node - a form, a
// section - drawn by the page, an action run from it closing the drawer),
// Panel (a panel of Object, the page's object when empty, drawn by the host)
// or File (a file of Object's module, module-relative, read in place by the
// host - a markdown section, a draw.io diagram, a document -).
type Drawer struct {
	Title  string `json:"title,omitempty"`
	Node   Node   `json:"node,omitempty"`
	Panel  string `json:"panel,omitempty"`
	File   string `json:"file,omitempty"`
	Object string `json:"object,omitempty"`
}

// Modal is what a modal action opens: Node, centred, with its Title; Size
// one of xs, sm, md, lg, xl.
type Modal struct {
	Title string `json:"title,omitempty"`
	Node  Node   `json:"node"`
	Size  string `json:"size,omitempty"`
}

// Navigate opens an object, on one of its panels when Panel is set.
type Navigate struct {
	Object string `json:"object"`
	Panel  string `json:"panel,omitempty"`
	// Query opens Panel as a query action would: its source read with these
	// args over its input (the Files tab with one file selected).
	Query map[string]any `json:"query,omitempty"`
}

// Chat opens a chat running Task (a task name or `<provider>:tasks/<name>`)
// on Target (an object id; the panel's object when empty).
type Chat struct {
	Task   string `json:"task"`
	Target string `json:"target,omitempty"`
}

// Create opens the new-object dialog: Type preset (the type picker when
// empty), under Parent (the panel's object when empty).
type Create struct {
	Type   string `json:"type,omitempty"`
	Parent string `json:"parent,omitempty"`
}

// Query re-reads the panel's source with Args over its input (`${value.*}`,
// `${item.*}` resolved by the host); no Args on a form's submit = the form's
// whole value. A source panel only: a page with no source to re-read shows it
// disabled.
type Query struct {
	Args map[string]any `json:"args,omitempty"`
}

// Select sets Key of the page's selection to Value (`${item.id}` resolved
// against the element the action is on).
type Select struct {
	Key   string `json:"key"`
	Value any    `json:"value,omitempty"`
}

// RunURI is an action running a method with its args.
func RunURI(label, uri string, args map[string]any) Action {
	return Action{Label: label, URI: uri, Args: args}
}

// NavigateTo is an action opening an object (on a panel when panel is set).
func NavigateTo(label, object, panel string) Action {
	return Action{Label: label, Navigate: &Navigate{Object: object, Panel: panel}}
}

// OpenPanel is an action opening a display panel by its id in a tab of its
// own.
func OpenPanel(label, panelID string) Action {
	return Action{Label: label, Panel: panelID}
}

// OpenScreen is an action opening one of the host's own screens by its name
// (HostScreens): ScreenTodo, « À faire ».
func OpenScreen(label, screen string) Action {
	return Action{Label: label, Screen: screen}
}

// CloseHere is an action closing what the page is drawn in (« Annuler »).
func CloseHere(label string) Action {
	return Action{Label: label, Close: true}
}

// ScreenTodo is « À faire », everything waiting on the person: the screen the
// rail's To do opens.
const ScreenTodo = "todo"

// HostScreens are the host's own screens a screen action opens.
var HostScreens = []string{ScreenTodo}

// ShowTab is an action showing a tab of this page: the id of its panel.
func ShowTab(label, panel string) Action {
	return Action{Label: label, Tab: panel}
}

// SelectOn is an action setting key of the page's selection to value: the
// sections whose load args read `${selection.<key>}` re-read.
func SelectOn(label, key string, value any) Action {
	return Action{Label: label, Select: &Select{Key: key, Value: value}}
}

// ChatOn is an action opening a chat running task on target.
func ChatOn(label, task, target string) Action {
	return Action{Label: label, Chat: &Chat{Task: task, Target: target}}
}

// CreateUnder is an action opening the new-object dialog for typ under parent.
func CreateUnder(label, typ, parent string) Action {
	return Action{Label: label, Create: &Create{Type: typ, Parent: parent}}
}

// QueryWith is an action re-reading the panel's source with args over its
// input (nil args on a form's submit: the form's whole value).
func QueryWith(label string, args map[string]any) Action {
	return Action{Label: label, Query: &Query{Args: args}}
}

// Result is the convention a method run from a page answers with: OK false is
// a refusal the host shows as such, Message is shown, Open (an object id,
// `<id>#<panel>`) is navigated to afterwards, Undo is the action that takes
// the change back - its args written out, no template - shown with the
// message as « Undo ». Project (a project folder) asks the host to make the
// universe serve that project: the hub switches it and the host restarts the
// universe (« Ouvrir un dossier »). Panel (a display panel
// id) is the panel the work runs in: the screen that ran the action shows it
// in place - a `panel` node - with « Ouvrir dans un onglet ».
// A method answering anything else is a plain success.
type Result struct {
	OK      bool    `json:"ok"`
	Message string  `json:"message,omitempty"`
	Open    string  `json:"open,omitempty"`
	Undo    *Action `json:"undo,omitempty"`
	Project string  `json:"project,omitempty"`
	Panel   string  `json:"panel,omitempty"`
}

// PanelNode embeds the display panel id: the screen follows
// it and draws it as its events build it, with « Ouvrir dans un onglet ».
func PanelNode(id, title string) Node {
	n := Node{"kind": "panel", "panelId": id}
	if title != "" {
		n["title"] = title
	}
	return n
}

// With sets a prop and returns the node, so a builder's result is refined in
// one expression: Badge("ok", "green").With("variant", "filled").
func (n Node) With(key string, v any) Node {
	n[key] = v
	return n
}

// VBox stacks children vertically.
func VBox(children ...Node) Node {
	return Node{"kind": "vbox", "gap": "md", "children": nodes(children)}
}

// HBox lines children up in a row (wrapping).
func HBox(children ...Node) Node {
	return Node{"kind": "hbox", "gap": "xs", "children": nodes(children)}
}

// Card frames children in a bordered card.
func Card(children ...Node) Node {
	return Node{"kind": "card", "withBorder": true, "p": "md", "radius": "sm", "gap": "md", "children": nodes(children)}
}

// Section is a titled block: an icon, the title, an optional count badge and
// nodes at its right (Aside), its children below — or Empty when it has none.
func Section(title, icon string, children ...Node) Node {
	return Node{"kind": "section", "title": title, "icon": icon, "children": nodes(children)}
}

// Aside sets the nodes drawn at the right of a section's heading row.
func (n Node) Aside(nodes ...Node) Node { return n.With("aside", nodes) }

// Empty sets what a section shows when it has no children, in a color
// ("" = dimmed; green for an all-clear).
func (n Node) Empty(text, color string) Node {
	n["empty"] = text
	if color != "" {
		n["emptyColor"] = color
	}
	return n
}

// Heading is a title of a level (1-6).
func Heading(text string, level int) Node {
	return Node{"kind": "heading", "text": text, "level": level}
}

// SectionLabel is a section's small label - mono, upper case, muted (the
// planning views' « SHIPS IN THIS RELEASE (8) »).
func SectionLabel(text string) Node {
	return Node{"kind": "heading", "text": text, "level": 4, "variant": "label"}
}

// Legend is the planning palette's legend: a small square in each status
// word's tone and its label.
func Legend(words ...string) Node {
	items := make([]any, len(words))
	for i, w := range words {
		items[i] = w
	}
	return Node{"kind": "legend", "items": items}
}

// Text is a line of text.
func Text(content string) Node { return Node{"kind": "text", "content": content} }

// Clamp holds a text to n lines, an ellipsis ending the last.
func (n Node) Clamp(lines int) Node { return n.With("lineClamp", lines) }

// Mono draws a text in the monospace font, its case kept (an id, a path).
func (n Node) Mono() Node { return n.With("mono", true) }

// Expandable puts under a clamped text a toggle « Afficher plus » / « Afficher
// moins », drawn only when the text overflows its lines.
func (n Node) Expandable() Node { return n.With("expandable", true) }

// Dimmed is a small muted line.
func Dimmed(content string) Node {
	return Node{"kind": "text", "content": content, "size": "xs", "color": "dimmed"}
}

// Badge is a colored pill; tone is a Mantine color (green, red, orange, gray…).
func Badge(text, tone string) Node {
	n := Node{"kind": "badge", "text": text, "size": "sm", "variant": "light"}
	if tone != "" {
		n["color"] = tone
	}
	return n
}

// Gauge is a value against its maximum, as a bar.
func Gauge(label string, value, max float64) Node {
	return Node{"kind": "gauge", "label": label, "value": value, "max": max}
}

// Stacked is a bar of parts (pass / deferred / failing of a total), its parts
// listed on hover and as colored chips under it. Zero parts are not drawn.
func Stacked(label string, parts ...Segment) Node {
	return Node{"kind": "gauge", "label": label, "segments": parts, "legend": true}
}

// Markdown is GFM text; Collapsible folds it past a height.
func Markdown(content string) Node { return Node{"kind": "markdown", "content": content} }

// Drawio is a view-only draw.io drawing (mxGraph XML) fitted to its box, at
// least height pixels high (0 = the display's default).
func Drawio(xml string, height int) Node {
	n := Node{"kind": "drawio", "xml": xml}
	if height > 0 {
		n["height"] = height
	}
	return n
}

// Collapsible folds a markdown node past maxHeight pixels behind a toggle.
func (n Node) Collapsible(maxHeight int) Node { return n.With("maxHeight", maxHeight) }

// KPI is a headline figure in a tinted card.
func KPI(label string, value any, tone, icon string) Node {
	n := Node{"kind": "kpi", "label": label, "value": value}
	if tone != "" {
		n["color"] = tone
	}
	if icon != "" {
		n["icon"] = icon
	}
	return n
}

// Button runs one action.
func Button(label string, a Action) Node {
	return Node{"kind": "button", "label": label, "action": a, "size": "xs", "variant": "light"}
}

// IconAfter draws a button's icon after its label, not before:
// « Suivante › » beside « ‹ Précédente ».
func (n Node) IconAfter() Node { return n.With("iconPosition", "right") }

// THE PAGE FILTERS: each sets a key of the page selection
// a node's `filter` keeps the rows passing it.

// Chips is a chip group setting the selection's key to the value of the chip
// clicked; all names a first chip clearing it (no filter), "" for none.
func Chips(key, all string, options ...ChipOption) Node {
	n := Node{"kind": "chips", "key": key, "options": options}
	if all != "" {
		n["all"] = all
	}
	return n
}

// ChipOption is a chip: the value it sets, its label, its count.
type ChipOption struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count *int   `json:"count,omitempty"`
	Color string `json:"color,omitempty"`
}

// Search is a search box setting the selection's key to what is typed.
func Search(key, placeholder string) Node {
	return Node{"kind": "search", "key": key, "placeholder": placeholder}
}

// Switch sets the selection's key to true or false.
func Switch(key, label string) Node {
	return Node{"kind": "switch", "key": key, "label": label}
}

// ChoiceCards is a set of choice cards: one card per option
// one chosen at a time, the chosen card setting the selection's key to its
// value; a disabled card cannot be chosen.
func ChoiceCards(key, label string, options ...ChoiceCard) Node {
	return Node{"kind": "choice-cards", "key": key, "label": label, "options": options}
}

// ChoiceCard is one option of ChoiceCards: the value it sets, its label and
// description, its badge (letters, else the label's initials; or an icon) in
// its colour, and, disabled, the pill beside its label and why (tooltip).
type ChoiceCard struct {
	Value       string `json:"value"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
	Badge       string `json:"badge,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
	Pill        string `json:"pill,omitempty"`
	Tooltip     string `json:"tooltip,omitempty"`
	// Sets are other selection keys the card sets when chosen, each only
	// while the person has not changed it (the Nom prefilled).
	Sets map[string]string `json:"sets,omitempty"`
}

// TextField is a labelled text field setting the selection's key to what is
// typed; value is the key's while the selection holds none.
func TextField(key, label, value string) Node {
	n := Node{"kind": "text-field", "key": key, "label": label}
	if value != "" {
		n["value"] = value
	}
	return n
}

// Rows makes a text field multi-line: a text area showing
// rows lines, growing with what is typed - a remark box.
func (n Node) Rows(rows int) Node { return n.With("rows", rows) }

// ShowWhen draws a node - any kind - only while the selection holds `key=value`
// (or, with no `=`, while key is set): its `when`.
func (n Node) ShowWhen(cond string) Node { return n.With("when", cond) }

// Requires keeps a button disabled until the selection sets every key, hint
// its tooltip meanwhile.
func (n Node) Requires(hint string, keys ...string) Node {
	n = n.With("requires", keys)
	if hint != "" {
		n = n.With("requiresHint", hint)
	}
	return n
}

// FilterEquals keeps the rows whose field is the selection key's value (an
// unset key keeps every row); empty, when set, is the key's value keeping the
// rows whose field is empty.
func FilterEquals(key, field, empty string) map[string]any {
	c := map[string]any{"key": key, "field": field}
	if empty != "" {
		c["empty"] = empty
	}
	return c
}

// FilterContains keeps the rows one of whose fields holds the key's text.
func FilterContains(key string, fields ...string) map[string]any {
	return map[string]any{"key": key, "fields": fields, "match": "contains"}
}

// FilterUnless shows a row whose field is set only while the key is on.
func FilterUnless(key, field string) map[string]any {
	return map[string]any{"key": key, "field": field, "match": "unless"}
}

// InlineSelect is an inline drop-down showing value among options (`a|b|c`
// values, each its own label); a choice runs a with `${value}` the option.
func InlineSelect(value, options string, a Action) Node {
	var opts []map[string]any
	for _, o := range strings.Split(options, "|") {
		if o = strings.TrimSpace(o); o != "" {
			opts = append(opts, map[string]any{"value": o, "label": o})
		}
	}
	return Node{"kind": "select", "value": value, "options": opts, "action": a}
}

// Menu is a small button opening its entries (MenuEntry), an entry
// running its action on the element it is on.
func Menu(label string, entries ...map[string]any) Node {
	n := Node{"kind": "menu", "items": entries}
	if label != "" {
		n["label"] = label
	}
	return n
}

// MenuEntry is a menu entry running a: prompt (a question asking a short
// text first) and choice (`a|b|c`) make it ask its `${value}`.
func MenuEntry(a Action, prompt, choice string) map[string]any {
	e := map[string]any{"id": a.ID, "label": a.Label, "action": a}
	if prompt != "" {
		e["prompt"] = prompt
	}
	if choice != "" {
		e["choice"] = choice
	}
	return e
}

// List repeats item (a template: `$.item` is the element) for each element of
// data, as cards when cards is true, with per-element actions.
func List(data any, item Node, cards bool, actions ...Action) Node {
	n := Node{"kind": "list", "data": data, "item": item}
	if cards {
		n["variant"] = "cards"
	}
	if len(actions) > 0 {
		n["actions"] = actions
	}
	return n
}

// ListOf draws each element of data as the variant of its data type - the
// type's `ui-view` extension of that name, `card` when variant is empty -
// through the generic view: one definition per kind of card, in the data
// type, never an item template per list. itemType is a
// data-type URI or the type's schema inline.
func ListOf(data any, itemType any, variant string, actions ...Action) Node {
	n := Node{"kind": "list", "data": data, "itemType": itemType}
	if variant != "" {
		n["itemVariant"] = variant
	}
	if len(actions) > 0 {
		n["actions"] = actions
	}
	return n
}

// Group is one group of a grouped list: the value of its groupBy field and
// its label.
type Group struct {
	Key   string `json:"key"`
	Label string `json:"label,omitempty"`
	// Only makes a heading (SectionBy) hold these groups' elements alone: it
	// is drawn with no filter or under one of theirs, and under one of
	// theirs the headings naming none of them are not drawn.
	Only []string `json:"only,omitempty"`
	// Icon, Color and Hint draw a group as a tile (FilterTiles) and its
	// heading as a card (SectionCards): a tabler icon, a colour, a one-line
	// hint under its label.
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
	Hint  string `json:"hint,omitempty"`
}

// GroupBy groups a list by a field of its elements: a header
// with a count per group - a click shows only that group, a second click
// shows all - and the elements under their group's heading. groups give the
// groups' order and labels; another value follows, in the order first seen,
// labelled by itself.
func (n Node) GroupBy(field string, groups ...Group) Node {
	n = n.With("groupBy", field)
	if len(groups) > 0 {
		n = n.With("groups", groups)
	}
	return n
}

// Overdue names the elements' field set when one is overdue: the grouped
// list's header adds an overdue count that filters the same way; label
// replaces « en retard » / « overdue » when not empty.
func (n Node) Overdue(field, label string) Node {
	n = n.With("overdueField", field)
	if label != "" {
		n = n.With("overdueLabel", label)
	}
	return n
}

// SectionBy draws a grouped list's elements under the headings of a second
// field: the header counts and filters by its groupBy (the
// kind), the elements are drawn by this field (the holder) - every heading
// kept under a filter, « Nothing of this kind. » when nothing is left.
// sections give the headings' order and labels; another value follows, in
// the order first seen, labelled by itself.
func (n Node) SectionBy(field string, sections ...Group) Node {
	n = n.With("sectionBy", field)
	if len(sections) > 0 {
		n = n.With("sectionGroups", sections)
	}
	return n
}

// Bulk gives a grouped list actions run on several elements of one group at
// once: each element ticked gets its own run of the action - one answer each
// -, skipping an element where the action's when does not hold or its
// disabledWhen is set.
func (n Node) Bulk(actions ...Action) Node { return n.With("bulk", actions) }

// CountWords sets a grouped list's header in the page's own words: label
// names the counts for a screen reader (« Filter by group » by default), hint
// is the note under them while none filters; an empty one keeps the default.
func (n Node) CountWords(label, hint string) Node {
	if label != "" {
		n = n.With("filterLabel", label)
	}
	if hint != "" {
		n = n.With("filterHint", hint)
	}
	return n
}

// NoSectionCounts draws a grouped list's headings without their count: the
// header counts.
func (n Node) NoSectionCounts() Node { return n.With("sectionCounts", false) }

// FilterTiles draws a grouped list's header counts as tiles (
// ): each group's count a KPI tile - its label, count, hint
// icon and colour (Group) - a click showing only that group and a second
// all, the overdue count one more tile; all, when not empty, labels a first
// tile counting every element, a click on it showing all.
func (n Node) FilterTiles(all string) Node {
	n = n.With("filterStyle", "tiles")
	if all != "" {
		n = n.With("allLabel", all)
	}
	return n
}

// SectionCards draws a grouped list's headings as cards: each section a
// bordered card titled « <label> — <count> » with its group's icon and colour
// and its hint under the title, its elements inside.
func (n Node) SectionCards() Node { return n.With("sectionStyle", "card") }

// PlaceBy draws a dimmed line naming the elements' place inside a section
// each time an element's field differs from the one before it (« Pour moi »,
// « Liste X · membres »): places give the values' labels; another value is
// labelled by itself.
func (n Node) PlaceBy(field string, places ...Group) Node {
	n = n.With("placeField", field)
	if len(places) > 0 {
		n = n.With("places", places)
	}
	return n
}

// NodeField draws each element of a list as the node its field holds, in
// place of an item template: a producer that draws each element itself -
// each card its own controls - still gets the list's grouping, filters, ticks
// and actions. An element without one is drawn by the template, or not at
// all.
func (n Node) NodeField(field string) Node { return n.With("nodeField", field) }

// Table draws rows (data) by a schema — a data-type URI, `{action: <uri>}`
// or the AST inline, resolved by the host — with actions on each row, in a
// last column. Open sets what a click on a row runs.
func Table(schema, data any, actions ...Action) Node {
	n := Node{"kind": "table", "schema": schema, "data": data}
	if len(actions) > 0 {
		n["actions"] = actions
	}
	return n
}

// InlineSchema is the canonical schema of T inline — a page's form needs no
// published data-type — rooted at root, each field named in labels labelled
// in the person's language.
func InlineSchema[T any](root string, labels map[string]string) map[string]any {
	ast, err := reflect.TypeAST[T]()
	if err != nil {
		return map[string]any{}
	}
	raw, _ := json.Marshal(ast)
	var t map[string]any
	_ = json.Unmarshal(raw, &t)
	if fields, ok := t["fields"].(map[string]any); ok {
		for name, label := range labels {
			if f, ok := fields[name].(map[string]any); ok {
				f["label"] = label
			}
		}
	}
	return map[string]any{"root": root, "types": map[string]any{root: t}}
}

// DescribeFields sets the descriptions of an inline schema's root fields - a
// field's hint under its input, in the person's language - and answers the
// schema; a field the root does not hold is left out.
func DescribeFields(schema map[string]any, descriptions map[string]string) map[string]any {
	root, _ := schema["root"].(string)
	types, _ := schema["types"].(map[string]any)
	t, _ := types[root].(map[string]any)
	fields, _ := t["fields"].(map[string]any)
	for name, d := range descriptions {
		if f, ok := fields[name].(map[string]any); ok {
			f["description"] = d
		}
	}
	return schema
}

// Form edits a value by a schema (a data-type URI, `{action: <uri>}` or the
// AST inline) and runs submit with it: `${value.<path>}` reads the value, and
// a method with no Args takes the whole value as its input.
func Form(schema any, submit Action) Node {
	return Node{"kind": "form", "schema": schema, "mode": "create", "submit": submit}
}

// AnswerForm is a form a person answers in a panel: its fields and one button,
// the submit's own label and icon - no heading of its own (its section titles
// it) and no Cancel, Reset or Delete, which belong to editing an object, not
// to answering.
func AnswerForm(schema map[string]any, submit Action, icon string, data any) Node {
	return Form(schema, submit).With("mode", "edit").With("data", data).With("hideTitle", true).
		With("ui", map[string]any{"actions": []any{map[string]any{"type": "ui-action", "id": "submit", "scope": "item", "label": submit.Label, "icon": icon}}})
}

// ActionForm is the form of an action's input whose submit runs that action
// with what was typed — a launch form drawn in a page.
func ActionForm(label, uri string) Node {
	return Form(map[string]any{"action": uri}, RunURI(label, uri, nil))
}

// Open sets what a click on an element runs (a table row, a gantt bar, a
// graph box), `${item.*}` the element.
func (n Node) Open(a Action) Node { return n.With("open", a) }

// Link is a text that runs an action when clicked — open an object or one of
// its panels (NavigateTo), a chat, a dialog, a method.
func Link(text string, a Action) Node {
	return Node{"kind": "link", "text": text, "action": a}
}

// Divider is a horizontal line.
func Divider() Node { return Node{"kind": "divider"} }

// Fold is one panel of an Accordion: its label, a count or word beside it,
// an icon, its colour (an accordion with look rule draws the label as a tag
// of it), its data-* keys for a probe (Data, "data-group" -> "running"), and
// its content.
type Fold struct {
	ID      string            `json:"id,omitempty"`
	Label   string            `json:"label"`
	Icon    string            `json:"icon,omitempty"`
	Badge   any               `json:"badge,omitempty"`
	Color   string            `json:"color,omitempty"`
	Data    map[string]string `json:"-"`
	Content Node              `json:"content"`
}

// MarshalJSON writes the fold with its data-* keys beside its props.
func (f Fold) MarshalJSON() ([]byte, error) {
	type plain Fold
	raw, err := json.Marshal(plain(f))
	if err != nil || len(f.Data) == 0 {
		return raw, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	for k, v := range f.Data {
		if strings.HasPrefix(k, "data-") {
			m[k] = v
		}
	}
	return json.Marshal(m)
}

// UnmarshalJSON reads a fold and its data-* keys back.
func (f *Fold) UnmarshalJSON(raw []byte) error {
	type plain Fold
	var p plain
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	for k, v := range m {
		if s, ok := v.(string); ok && strings.HasPrefix(k, "data-") {
			if p.Data == nil {
				p.Data = map[string]string{}
			}
			p.Data[k] = s
		}
	}
	*f = Fold(p)
	return nil
}

// Grid is cells in columns of fixed widths: a width in pixels
// per column, 0 for a column sharing what is left; the cells fill row by row.
func Grid(widths []int, children ...Node) Node {
	return Node{"kind": "grid", "cols": len(widths), "widths": widths, "children": nodes(children)}
}

// Dividers draws a 1px line on the left of each column named: 0-based column
// indexes, the first (0) excluded.
func (n Node) Dividers(cols ...int) Node { return n.With("dividers", cols) }

// RuleLook draws an accordion's folds as rule lines: a caret
// the label (a tag in the fold's colour, else dimmed), its badge and a rule to
// the end of the line, with no frame.
func (n Node) RuleLook() Node { return n.With("look", "rule") }

// Swatches draws a stacked gauge's legend as swatches: per
// shown part a small square in the part's own colour beside its dimmed count
// and label, wrapping - in place of the colored chips.
func (n Node) Swatches() Node { return n.With("legend", "swatch") }

// Dashed draws a card's border dashed in its normal border colour
// : a card set apart, as a delivery constraint's goal card.
func (n Node) Dashed() Node { return n.With("dashed", true) }

// Stripe draws a band of tone (a Mantine colour) down a card's left edge: a
// row's kind, as the increment's Blockers tab colours each blocker.
func (n Node) Stripe(tone string) Node { return n.With("stripe", tone) }

// FillHeight makes a card take the window's height below it, its last child
// scrolling inside it: boxes side by side in a grid row each
// scroll their own content and the page does not.
func (n Node) FillHeight() Node { return n.With("height", "fill") }

// Accordion is folds a person opens and closes, several at a time; open names
// the ones open at first (by ID).
func Accordion(folds []Fold, open ...string) Node {
	if folds == nil {
		folds = []Fold{}
	}
	n := Node{"kind": "accordion", "panels": folds, "multiple": true}
	if len(open) > 0 {
		n["defaultOpen"] = open
	}
	return n
}

func nodes(ns []Node) []Node {
	if ns == nil {
		return []Node{}
	}
	return ns
}

// PageKinds are the node kinds a page may hold, by category — the display
// library's LAYOUT_KINDS, ATOMIC_KINDS, SCHEMA_BOUND_KINDS and
// INTERACTIVE_KINDS (a test holds the two lists together).
var PageKinds = map[string][]string{
	"layout":       {"vbox", "hbox", "grid", "box", "card", "section", "tabs", "accordion", "split"},
	"atomic":       {"heading", "text", "divider", "badge", "link", "image", "video", "audio", "file", "timeline", "kpi", "code-block", "markdown", "chart", "drawio", "legend", "monogram", "progress", "log"},
	"schema-bound": {"form", "view", "table", "wizard", "timeline-lanes"},
	"interactive":  {"button", "list", "tree", "lanes", "gauge", "gantt", "graph", "picker", "select", "menu", "stepper", "board", "grid-board", "chips", "search", "switch", "choice-cards", "text-field", "terms", "source-search", "item-detail", "action-bar", "progress-list", "panel", "object-card"},
}

// required are the props a node of a kind cannot go without (the DSL JSON
// schema's). A node bound by dataField takes its main prop from the data, so
// only its structural props stay required.
var required = map[string][]string{
	"grid": {"cols"}, "heading": {"text"}, "text": {"content"}, "badge": {"text"},
	"link": {"text"}, "image": {"src"}, "video": {"src"}, "audio": {"src"},
	"file": {"src", "name"}, "timeline": {"items"}, "kpi": {"label", "value"},
	"code-block": {"content"}, "markdown": {"content"}, "drawio": {"xml"}, "legend": {"items"}, "monogram": {"text"}, "chart": {"chartKind", "data"},
	"button": {"label", "action"}, "list": {"item"}, "lanes": {"lanes"},
	"picker":       {"search"},
	"select":       {"action"},
	"stepper":      {"steps"},
	"board":        {"select", "panel"},
	"grid-board":   {"rows", "columns"},
	"menu":         {"items"},
	"object-card":  {"row"},
	"chips":        {"key"},
	"search":       {"key"},
	"switch":       {"key", "label"},
	"choice-cards": {"key"},
	"text-field":   {"key", "label"},
	"terms":        {"action"},
	"panel":        {"panelId"},
	"action-bar":   {"parts"},
	"split":        {"main", "side"},
}

var structural = map[string]bool{"cols": true, "item": true, "lanes": true, "label": true, "action": true, "chartKind": true, "browse": true, "select": true, "object": true, "search": true, "main": true, "side": true, "source": true, "panel": true, "key": true}

// checkText refuses a text's lineClamp that is not a whole number of 1 or
// more, a mono or expandable that is not true or false, and expandable
// without lineClamp.
func checkText(n map[string]any, at string) error {
	if v, ok := n["lineClamp"]; ok {
		if f, isNum := v.(float64); !isNum || f < 1 || f != float64(int(f)) {
			return fmt.Errorf("render: page %s: a text's lineClamp is a whole number of 1 or more, not %v", at, v)
		}
	}
	for _, p := range []string{"mono", "expandable"} {
		if v, ok := n[p]; ok {
			if _, isBool := v.(bool); !isBool {
				return fmt.Errorf("render: page %s: a text's %s is true or false, not %v", at, p, v)
			}
		}
	}
	if n["expandable"] == true && n["lineClamp"] == nil {
		return fmt.Errorf("render: page %s: an expandable text needs its lineClamp", at)
	}
	return nil
}

// Validate refuses a Def that cannot display: a page without a tree, or a
// tree off the page grammar.
func (d Def) Validate() error {
	if d.Kind == "page" {
		if d.DSLTree == nil {
			return fmt.Errorf("render: a page needs its dslTree")
		}
		return ValidatePage(d.DSLTree)
	}
	return nil
}

// ValidatePage walks a page tree (a Node, or the same tree as decoded JSON)
// and refuses an unknown kind, a missing required prop, a gauge with neither
// value nor segments, a negative segment, an action without its URI, a node
// id used twice and a section without its id or load method — each error
// naming the node's path.
func ValidatePage(tree any) error {
	raw, err := json.Marshal(tree)
	if err != nil {
		return fmt.Errorf("render: page: %w", err)
	}
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("render: page: %w", err)
	}
	known := map[string]bool{}
	for _, ks := range PageKinds {
		for _, k := range ks {
			known[k] = true
		}
	}
	ids := map[string]string{}
	var walk func(v any, at string) error
	walk = func(v any, at string) error {
		n, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("render: page %s: a node is an object", at)
		}
		kind, _ := n["kind"].(string)
		if !known[kind] {
			return fmt.Errorf("render: page %s: unknown kind %q", at, kind)
		}
		at = at + "/" + kind
		if err := checkSection(n, at, ids); err != nil {
			return err
		}
		_, bound := n["dataField"].(string)
		for _, p := range required[kind] {
			// A list naming its elements' type draws each by that type's
			// card: it needs no item template.
			if kind == "list" && p == "item" && (n["itemType"] != nil || n["nodeField"] != nil) {
				continue
			}
			if _, ok := n[p]; !ok && (!bound || structural[p]) {
				return fmt.Errorf("render: page %s: %q is required", at, p)
			}
		}
		if kind == "link" {
			_, href := n["href"]
			_, act := n["action"]
			if href == act && !bound {
				return fmt.Errorf("render: page %s: a link needs an href or an action, one of the two", at)
			}
		}
		if kind == "gauge" {
			if v, ok := n["legend"]; ok && v != true && v != false && v != "swatch" {
				return fmt.Errorf("render: page %s: a gauge's legend is true, false or swatch, not %v", at, v)
			}
			segs, _ := n["segments"].([]any)
			if _, ok := n["value"]; !ok && len(segs) == 0 && !bound {
				return fmt.Errorf("render: page %s: a gauge needs value or segments", at)
			}
			for i, s := range segs {
				sm, _ := s.(map[string]any)
				if v, ok := sm["value"].(float64); !ok || v < 0 {
					return fmt.Errorf("render: page %s: segment %d needs a value of 0 or more", at, i)
				}
				items, _ := sm["items"].([]any)
				for j, it := range items {
					if im, _ := it.(map[string]any); im == nil || im["id"] == nil || im["id"] == "" {
						return fmt.Errorf("render: page %s: segment %d: item %d needs its id", at, i, j)
					}
				}
			}
		}
		if kind == "list" {
			if err := checkGrouped(n, at); err != nil {
				return err
			}
			// The nodes its elements carry are the page's too.
			if f, _ := n["nodeField"].(string); f != "" {
				els, _ := n["data"].([]any)
				for i, el := range els {
					em, _ := el.(map[string]any)
					if c, ok := em[f]; ok && c != nil {
						if err := walk(c, fmt.Sprintf("%s.data[%d].%s", at, i, f)); err != nil {
							return err
						}
					}
				}
			}
		}
		if kind == "text" {
			if err := checkText(n, at); err != nil {
				return err
			}
		}
		if kind == "choice-cards" {
			opts, _ := n["options"].([]any)
			for i, o := range opts {
				om, _ := o.(map[string]any)
				if v, _ := om["value"].(string); v == "" {
					return fmt.Errorf("render: page %s: choice card %d has no value", at, i)
				}
			}
		}
		if kind == "text-field" {
			if v, ok := n["rows"]; ok {
				if f, isNum := v.(float64); !isNum || f < 1 || f != float64(int(f)) {
					return fmt.Errorf("render: page %s: a text field's rows is a number of lines, 1 or more, not %v", at, v)
				}
			}
		}
		if kind == "button" {
			if v, ok := n["iconPosition"]; ok && v != "left" && v != "right" {
				return fmt.Errorf("render: page %s: a button's iconPosition is left or right, not %v", at, v)
			}
			if v, ok := n["requires"]; ok {
				keys, isList := v.([]any)
				for _, k := range keys {
					if s, _ := k.(string); s == "" {
						isList = false
					}
				}
				if !isList || len(keys) == 0 {
					return fmt.Errorf("render: page %s: a button's requires is a list of selection keys, not %v", at, v)
				}
			}
		}
		if kind == "card" {
			if v, ok := n["dashed"]; ok {
				if _, isBool := v.(bool); !isBool {
					return fmt.Errorf("render: page %s: a card's dashed is true or false, not %v", at, v)
				}
			}
			if v, ok := n["stripe"]; ok {
				if s, isText := v.(string); !isText || s == "" {
					return fmt.Errorf("render: page %s: a card's stripe is a tone, not %v", at, v)
				}
			}
			if v, ok := n["height"]; ok && v != "fill" {
				return fmt.Errorf("render: page %s: a card's height is fill, not %v", at, v)
			}
		}
		if kind == "kpi" {
			if v, ok := n["variant"]; ok && v != "inline" {
				return fmt.Errorf("render: page %s: a kpi's variant is inline, not %v", at, v)
			}
		}
		if kind == "stepper" {
			if v, ok := n["variant"]; ok && v != "steps" && v != "dots" {
				return fmt.Errorf("render: page %s: a stepper's variant is steps or dots, not %v", at, v)
			}
		}
		if kind == "board" {
			if v, ok := n["layout"]; ok && v != "list" && v != "cards" {
				return fmt.Errorf("render: page %s: a board's layout is list or cards, not %v", at, v)
			}
		}
		if kind == "grid" {
			if err := checkGrid(n, at); err != nil {
				return err
			}
		}
		if kind == "accordion" {
			if v, ok := n["look"]; ok && v != "rule" {
				return fmt.Errorf("render: page %s: an accordion's look is rule, not %v", at, v)
			}
		}
		if kind == "graph" {
			if err := checkGraph(n, at); err != nil {
				return err
			}
		}
		if err := checkActions(n, at); err != nil {
			return err
		}
		for _, key := range []string{"children", "aside"} {
			kids, _ := n[key].([]any)
			for i, c := range kids {
				if err := walk(c, fmt.Sprintf("%s.%s[%d]", at, key, i)); err != nil {
					return err
				}
			}
		}
		for _, key := range []string{"tabs", "panels"} {
			ps, _ := n[key].([]any)
			for i, p := range ps {
				pm, _ := p.(map[string]any)
				if err := walk(pm["content"], fmt.Sprintf("%s.%s[%d]", at, key, i)); err != nil {
					return err
				}
			}
		}
		for _, key := range []string{"item", "main", "side", "panel"} {
			if c, ok := n[key]; ok && c != nil {
				if err := walk(c, at+"."+key); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(root, "")
}

// checkGrid refuses a grid's widths and dividers off their shape
// : a width per column - its length the grid's cols -, none
// negative (0 is a share of what is left), and a divider naming a column
// after the first.
func checkGrid(n map[string]any, at string) error {
	cols, _ := n["cols"].(float64)
	if w, ok := n["widths"]; ok {
		ws, isList := w.([]any)
		if !isList {
			return fmt.Errorf("render: page %s: a grid's widths is a list of pixels", at)
		}
		if float64(len(ws)) != cols {
			return fmt.Errorf("render: page %s: a grid's widths has %d widths for %v columns", at, len(ws), cols)
		}
		for i, v := range ws {
			if f, isNum := v.(float64); !isNum || f < 0 {
				return fmt.Errorf("render: page %s: width %d is a number of pixels, 0 or more, not %v", at, i, v)
			}
		}
	}
	if d, ok := n["dividers"]; ok {
		ds, isList := d.([]any)
		if !isList {
			return fmt.Errorf("render: page %s: a grid's dividers is a list of 0-based column indexes, the first (0) excluded", at)
		}
		for _, v := range ds {
			if f, isNum := v.(float64); !isNum || f < 1 || f >= cols || f != float64(int(f)) {
				return fmt.Errorf("render: page %s: a divider is a column from 1 to %v, not %v", at, cols-1, v)
			}
		}
	}
	return nil
}

// checkGrouped refuses a grouped list's props off their shape
// : a groupBy, sectionBy, overdueField or overdueLabel that
// is not a text, a group or section without its key, headings by a field on
// a list counted by nothing, sections named with no sectionBy, bulk actions
// that are not a list, or bulk actions on a list grouped by nothing - a bulk
// answer is for one group.
func checkGrouped(n map[string]any, at string) error {
	for _, k := range []string{"groupBy", "sectionBy", "overdueField", "overdueLabel", "filterLabel", "filterHint", "nodeField", "allLabel", "placeField"} {
		if v, ok := n[k]; ok {
			if s, isText := v.(string); !isText || ((k == "groupBy" || k == "sectionBy" || k == "nodeField" || k == "placeField") && s == "") {
				return fmt.Errorf("render: page %s: a list's %s is a text", at, k)
			}
		}
	}
	for k, values := range map[string][]string{"filterStyle": {"chips", "tiles"}, "sectionStyle": {"heading", "card"}} {
		if v, ok := n[k]; ok {
			if s, _ := v.(string); !slices.Contains(values, s) {
				return fmt.Errorf("render: page %s: a list's %s is %s", at, k, strings.Join(values, " or "))
			}
		}
	}
	for _, k := range []string{"filterStyle", "allLabel", "sectionStyle", "placeField"} {
		if _, ok := n[k]; ok {
			if _, grouped := n["groupBy"]; !grouped {
				return fmt.Errorf("render: page %s: a list's %s draws its groups: the list needs its groupBy", at, k)
			}
		}
	}
	if _, ok := n["places"]; ok {
		if _, by := n["placeField"]; !by {
			return fmt.Errorf("render: page %s: a list's places name the values of its placeField: the list needs its placeField", at)
		}
	}
	for _, k := range []string{"groups", "sectionGroups", "places"} {
		v, ok := n[k]
		if !ok {
			continue
		}
		gs, isList := v.([]any)
		if !isList {
			return fmt.Errorf("render: page %s: a list's %s are a list", at, k)
		}
		for i, g := range gs {
			gm, _ := g.(map[string]any)
			if key, _ := gm["key"].(string); gm == nil || key == "" {
				return fmt.Errorf("render: page %s: %s %d needs its key", at, k, i)
			}
			if o, ok := gm["only"]; ok {
				os, isList := o.([]any)
				if len(os) == 0 {
					isList = false
				}
				for _, x := range os {
					if xs, _ := x.(string); xs == "" {
						isList = false
					}
				}
				if !isList || k != "sectionGroups" {
					return fmt.Errorf("render: page %s: %s %d: only names groups by their keys, on a heading (sectionGroups)", at, k, i)
				}
			}
		}
	}
	if _, ok := n["sectionBy"]; ok {
		if _, grouped := n["groupBy"]; !grouped {
			return fmt.Errorf("render: page %s: a list's sectionBy draws headings beside the counts of its groupBy: the list needs its groupBy", at)
		}
	}
	if _, ok := n["sectionGroups"]; ok {
		if _, by := n["sectionBy"]; !by {
			return fmt.Errorf("render: page %s: a list's sectionGroups name the headings of its sectionBy: the list needs its sectionBy", at)
		}
	}
	if v, ok := n["sectionCounts"]; ok {
		if _, isBool := v.(bool); !isBool {
			return fmt.Errorf("render: page %s: a list's sectionCounts is true or false", at)
		}
	}
	if v, ok := n["bulk"]; ok {
		if _, isList := v.([]any); !isList {
			return fmt.Errorf("render: page %s: a list's bulk actions are a list", at)
		}
		if _, grouped := n["groupBy"]; !grouped {
			return fmt.Errorf("render: page %s: bulk actions answer the elements of one group: the list needs its groupBy", at)
		}
	}
	return nil
}

// checkSection refuses a node id used twice in a page - an op targets a node
// by its id - and a section without its id or its load
// method, or with a placeholder or a when it does not know.
func checkSection(n map[string]any, at string, ids map[string]string) error {
	id, _ := n["id"].(string)
	if id != "" {
		if first, dup := ids[id]; dup {
			return fmt.Errorf("render: page %s: node id %q is already used at %s", at, id, first)
		}
		ids[id] = at
	}
	load, isSection := n["load"]
	if !isSection {
		// A pushed section: no load, its placeholder an
		// object - the producer of a page panel replaces it by its id.
		ph, pushed := n["placeholder"].(map[string]any)
		if !pushed {
			return nil
		}
		if id == "" {
			return fmt.Errorf("render: page %s: a pushed section needs its id", at)
		}
		switch ph["kind"] {
		case PlaceholderSkeleton, PlaceholderCards, PlaceholderRows:
			return nil
		default:
			return fmt.Errorf("render: page %s: section %q: a placeholder is skeleton, cards or rows, not %v", at, id, ph["kind"])
		}
	}
	if id == "" {
		return fmt.Errorf("render: page %s: a section needs its id", at)
	}
	lm, _ := load.(map[string]any)
	if m, _ := lm["method"].(string); m == "" {
		return fmt.Errorf("render: page %s: section %q needs its load method", at, id)
	}
	if ph, ok := n["placeholder"]; ok {
		pm, _ := ph.(map[string]any)
		switch pm["kind"] {
		case PlaceholderSkeleton, PlaceholderCards, PlaceholderRows:
		default:
			return fmt.Errorf("render: page %s: section %q: a placeholder is skeleton, cards or rows, not %v", at, id, pm["kind"])
		}
	}
	if w, ok := n["when"]; ok && w != WhenVisible && w != WhenOpen && w != WhenEager {
		return fmt.Errorf("render: page %s: section %q: when is visible, open or eager, not %v", at, id, w)
	}
	return nil
}

// checkGraph refuses a graph box that cannot be drawn: a
// frame other than solid or dotted, a group naming no box of the graph, or
// badges that are not texts.
func checkGraph(n map[string]any, at string) error {
	data, _ := n["data"].(map[string]any)
	boxes, _ := data["nodes"].([]any)
	ids := map[string]bool{}
	for _, b := range boxes {
		if bm, _ := b.(map[string]any); bm != nil {
			if id, _ := bm["id"].(string); id != "" {
				ids[id] = true
			}
		}
	}
	for i, b := range boxes {
		bm, _ := b.(map[string]any)
		if bm == nil {
			return fmt.Errorf("render: page %s: box %d is an object", at, i)
		}
		if f, ok := bm["frame"]; ok && f != "solid" && f != "dotted" {
			return fmt.Errorf("render: page %s: box %d: frame is solid or dotted, not %v", at, i, f)
		}
		if g, ok := bm["group"]; ok {
			if gs, _ := g.(string); !ids[gs] {
				return fmt.Errorf("render: page %s: box %d: its group %v is no box of the graph", at, i, g)
			}
		}
		if bs, ok := bm["badges"]; ok {
			list, isList := bs.([]any)
			for _, x := range list {
				if _, isText := x.(string); !isText {
					isList = false
				}
			}
			if !isList {
				return fmt.Errorf("render: page %s: box %d: badges are a list of texts", at, i)
			}
		}
	}
	return nil
}

// checkActions refuses an action that does not name exactly one verb (uri,
// navigate, chat, create, query, tab, select), a uri not of the form provider:slot/method, a
// navigate without its object and a chat without its task.
func checkActions(n map[string]any, at string) error {
	var acts []any
	for _, key := range []string{"action", "move", "open", "submit", "toggle"} {
		if a, ok := n[key]; ok {
			acts = append(acts, a)
		}
	}
	// A tabs node's select runs when a tab is chosen.
	if a, ok := n["select"]; ok && n["kind"] == "tabs" {
		acts = append(acts, a)
	}
	if list, ok := n["actions"].([]any); ok {
		acts = append(acts, list...)
	}
	// A grouped list's bulk actions run on each element ticked.
	if list, ok := n["bulk"].([]any); ok {
		acts = append(acts, list...)
	}
	if cols, ok := n["columns"].([]any); ok && n["kind"] == "grid-board" {
		for _, c := range cols {
			if cm, _ := c.(map[string]any); cm != nil && cm["move"] != nil {
				acts = append(acts, cm["move"])
			}
		}
	}
	if lanes, ok := n["lanes"].([]any); ok {
		for _, l := range lanes {
			if lm, _ := l.(map[string]any); lm != nil && lm["move"] != nil {
				acts = append(acts, lm["move"])
			}
		}
	}
	if n["kind"] == "menu" {
		entries, ok := n["items"].([]any)
		if _, has := n["items"]; has && !ok {
			return fmt.Errorf("render: page %s: a menu's items are a list", at)
		}
		for i, e := range entries {
			a, err := menuEntryAction(e)
			if err != nil {
				return fmt.Errorf("render: page %s: entry %d: %w", at, i, err)
			}
			acts = append(acts, a)
		}
	}
	for _, a := range acts {
		if err := CheckAction(a); err != nil {
			return fmt.Errorf("render: page %s: %w", at, err)
		}
	}
	return nil
}

// menuEntryAction is the action a menu entry runs — its `action`, or the
// entry itself when it names a verb (a ui-action `{id, label, uri, args}`) —
// refusing an entry without its id and label, with no action, with a
// `prompt` that is not a string, or a `choice` neither a string (`a|b|c`, or
// the element's field) nor an object {options, optionsField}.
func menuEntryAction(e any) (any, error) {
	em, ok := e.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("a menu entry is an object")
	}
	if id, _ := em["id"].(string); id == "" {
		return nil, fmt.Errorf("a menu entry needs its id")
	}
	if l, _ := em["label"].(string); l == "" {
		return nil, fmt.Errorf("a menu entry needs its label")
	}
	if p, ok := em["prompt"]; ok {
		if _, isStr := p.(string); !isStr {
			return nil, fmt.Errorf("a menu entry's prompt is a string")
		}
	}
	if c, ok := em["choice"]; ok {
		switch cv := c.(type) {
		case string:
			if strings.TrimSpace(cv) == "" {
				return nil, fmt.Errorf("a menu entry's choice names its options")
			}
		case map[string]any:
			if o, has := cv["options"]; has {
				if _, isList := o.([]any); !isList {
					return nil, fmt.Errorf("a menu entry's choice options are a list")
				}
			}
		default:
			return nil, fmt.Errorf("a menu entry's choice is a string or an object {options, optionsField}")
		}
	}
	if a, ok := em["action"]; ok {
		return a, nil
	}
	for _, v := range ActionVerbs {
		if x, ok := em[v]; ok && x != nil && x != "" {
			return em, nil
		}
	}
	return nil, fmt.Errorf("a menu entry needs its action")
}

// CheckAction refuses an action (an Action, or the same as decoded JSON) that
// does not name exactly one verb, or whose verb lacks what it needs.
func CheckAction(a any) error {
	am, ok := a.(map[string]any)
	if !ok {
		raw, err := json.Marshal(a)
		if err != nil {
			return fmt.Errorf("an action: %w", err)
		}
		if err := json.Unmarshal(raw, &am); err != nil || am == nil {
			return fmt.Errorf("an action is an object")
		}
	}
	var verbs []string
	for _, v := range ActionVerbs {
		if x, ok := am[v]; ok && x != nil && x != "" {
			verbs = append(verbs, v)
		}
	}
	if len(verbs) != 1 {
		return fmt.Errorf("an action needs exactly one of %s; got [%s]", strings.Join(ActionVerbs, ", "), strings.Join(verbs, ", "))
	}
	switch verbs[0] {
	case "uri":
		uri, _ := am["uri"].(string)
		if i := strings.Index(uri, ":"); i <= 0 || i == len(uri)-1 {
			return fmt.Errorf("an action needs its uri (provider:slot/method), got %q", uri)
		}
	case "navigate":
		nav, _ := am["navigate"].(map[string]any)
		if o, _ := nav["object"].(string); o == "" {
			return fmt.Errorf("a navigate action needs its object")
		}
	case "chat":
		c, _ := am["chat"].(map[string]any)
		if t, _ := c["task"].(string); t == "" {
			return fmt.Errorf("a chat action needs its task")
		}
	case "create":
		if _, ok := am["create"].(map[string]any); !ok {
			return fmt.Errorf("a create action is an object {type, parent}")
		}
	case "query":
		if _, ok := am["query"].(map[string]any); !ok {
			return fmt.Errorf("a query action is an object {args}")
		}
	case "tab":
		if _, ok := am["tab"].(string); !ok {
			return fmt.Errorf("a tab action names the panel it shows")
		}
	case "select":
		sel, _ := am["select"].(map[string]any)
		if k, _ := sel["key"].(string); k == "" {
			return fmt.Errorf("a select action names the key it sets")
		}
	case "drawer":
		d, _ := am["drawer"].(map[string]any)
		node, hasNode := d["node"]
		panel, _ := d["panel"].(string)
		file, _ := d["file"].(string)
		opens := 0
		for _, set := range []bool{hasNode && node != nil, panel != "", file != ""} {
			if set {
				opens++
			}
		}
		if opens != 1 {
			return fmt.Errorf("a drawer action opens one of a node, a panel or a file")
		}
		if hasNode && node != nil {
			if err := ValidatePage(node); err != nil {
				return fmt.Errorf("a drawer's node: %w", err)
			}
		}
	case "panel":
		if id, _ := am["panel"].(string); !strings.HasPrefix(id, "dp-") && !strings.Contains(id, "${") {
			return fmt.Errorf("a panel action opens a display panel by its id (dp-…); got %q", id)
		}
	case "screen":
		if s, _ := am["screen"].(string); !slices.Contains(HostScreens, s) {
			return fmt.Errorf("a screen action opens one of the host's screens (%s); got %q", strings.Join(HostScreens, ", "), s)
		}
	case "close":
		if c, _ := am["close"].(bool); !c {
			return fmt.Errorf("a close action is close: true")
		}
	case "modal":
		m, _ := am["modal"].(map[string]any)
		node, ok := m["node"]
		if !ok || node == nil {
			return fmt.Errorf("a modal action opens a node")
		}
		if err := ValidatePage(node); err != nil {
			return fmt.Errorf("a modal's node: %w", err)
		}
	}
	return nil
}

// ActionVerbs are the verbs of an action — the fields of which exactly one is
// set (the display library's ACTION_VERBS, held equal by a test).
var ActionVerbs = []string{"uri", "navigate", "chat", "create", "query", "tab", "select", "drawer", "modal", "panel", "screen", "close"}

// HostTemplates are the names a page's HOST fills when an action runs — the
// element the action is on (`item`, `index`), a submitted form's `value`, a
// dropped card's `lane`, the person acting, the page's `selection` (a
// section's load args, an action's templates). Whoever resolves a declared page
// before the host (the object pane) leaves a template naming one as written.
var HostTemplates = []string{"item", "index", "value", "lane", "person", "selection"}

// IsHostTemplate reports whether a `${…}` expression names a host template
// (`item`, `item.id`, …).
func IsHostTemplate(expr string) bool {
	expr = strings.TrimSpace(expr)
	for _, h := range HostTemplates {
		if expr == h || strings.HasPrefix(expr, h+".") {
			return true
		}
	}
	return false
}
