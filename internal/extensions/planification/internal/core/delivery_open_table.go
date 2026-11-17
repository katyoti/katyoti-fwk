// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// DELIVERY'S « OPEN INCREMENTS » (view 1 « Open
// increments » of the mock-up
// modules/research-delivery-overview/delivery-views-v2-mockup.html, sorted by
// priority - its « one table » drawn as the planification card by the
// person's word) - one card per open increment in no
// release: its id, title, a « held » tag, its stream, its one-line
// description; at the card's foot the three things a person manages - its
// priority, Hold… or Resume, the release it ships in - each a button or a
// menu, never an inline drop-down. The search box
// (id, title), the stream and the held filters on top;
// sorted by priority, then rank (its id's number); 25 cards, « Afficher plus »
// adds 25. The list is a section: its own page (OpenPage) read for the
// filters and the count the page's selection holds.

// The Open increments list's source, page size and selection keys.
const (
	openPageURI  = "ktw-agents:increments/open-page"
	openPageSize = 25

	openStreamKey = "open_stream"
	openHeldKey   = "open_held"
	openShownKey  = "open_shown"
	openSearchKey = "open_search"

	openAllStreams = "all"
	openNotHeld    = "not-held"
	openHeld       = "held"
)

// openWords are the table's words, per language.
var openWords = map[string]map[string]string{
	"en": {
		"search": "Search id, title…", "all": "All streams", "no_stream": "No stream", "not_held": "Not held", "held": "Held", "sorted": "sorted by priority, then rank",
		"held_tag": "held", "resume": "Resume", "hold": "Hold…", "hold_title": "Put it on hold", "hold_reason": "Why it waits",
		"priority": "Priority", "ships_in": "Ships in", "stream": "Stream", "none": "— none", "no_release": "— No release", "next": " (next)",
		"more": "… %d more · %d per page", "show_more": "Show more", "empty": "No open increment here.",
	},
	"fr": {
		"search": "Chercher id, titre…", "all": "Tous les flux", "no_stream": "Sans flux", "not_held": "Non suspendus", "held": "Suspendus", "sorted": "triés par priorité, puis par rang",
		"held_tag": "suspendu", "resume": "Reprendre", "hold": "Suspendre…", "hold_title": "Le suspendre", "hold_reason": "Pourquoi il attend",
		"priority": "Priorité", "ships_in": "Livré dans", "stream": "Flux", "none": "— aucune", "no_release": "— Aucune version", "next": " (prochaine)",
		"more": "… %d de plus · %d par page", "show_more": "Afficher plus", "empty": "Aucun incrément ouvert ici.",
	},
}

func openWord(lang, key string) string {
	if l, ok := openWords[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return openWords["en"][key]
}

// openSelection is the selection the table opens on: no search, every
// stream, the increments not held, the first page.
func openSelection() map[string]any {
	return map[string]any{openSearchKey: "", openStreamKey: openAllStreams, openHeldKey: openNotHeld, openShownKey: openPageSize}
}

// openIncrementsTable is the « Open increments » tab of view 1A: the search
// box (id and title), the stream and held filters with their counts (the
// totals, not narrowed by the search), the order it is sorted by, and the
// table - a region reading them from the page's selection.
func openIncrementsTable(lang string, lanes []api.StreamLane, backlog []api.IncrementBacklogItem) node {
	byStream, held := map[string]int{}, 0
	for _, it := range backlog {
		s := it.Increment.Stream
		if s == "" {
			s = noStreamRow
		}
		byStream[s]++
		if it.Increment.Held != nil {
			held++
		}
	}
	all, notHeld := len(backlog), len(backlog)-held
	streams := []render.ChipOption{{Value: openAllStreams, Label: openWord(lang, "all"), Count: &all}}
	for _, l := range lanes {
		n := byStream[l.ID]
		streams = append(streams, render.ChipOption{Value: l.ID, Label: l.Name, Count: &n, Color: l.Color})
	}
	if n := byStream[noStreamRow]; n > 0 {
		streams = append(streams, render.ChipOption{Value: noStreamRow, Label: openWord(lang, "no_stream"), Count: &n})
	}
	holds := []render.ChipOption{
		{Value: openNotHeld, Label: openWord(lang, "not_held"), Count: &notHeld},
		{Value: openHeld, Label: openWord(lang, "held"), Count: &held},
	}
	// The search is read from the whole selection: a
	// section waits while a key its args name is empty, and the search is
	// empty until the person types - and again once they clear it.
	table := loadedSection("open-increments", openPageURI,
		node{"selection": "${selection}", "stream": "${selection." + openStreamKey + "}", "held": "${selection." + openHeldKey + "}",
			"shown": "${selection." + openShownKey + "}", "language": lang},
		openWord(lang, "empty"), render.PlaceholderRows, 6)
	return node{"kind": "vbox", "gap": "sm", "data-view": "open-increments-tab", "children": []node{
		{"kind": "hbox", "gap": "sm", "wrap": true, "align": "center", "justify": "space-between", "data-view": "open-increments-toolbar", "children": []node{
			{"kind": "hbox", "gap": "sm", "wrap": true, "align": "center", "children": []node{
				render.Search(openSearchKey, openWord(lang, "search")),
				render.Chips(openStreamKey, "", streams...),
				render.Chips(openHeldKey, "", holds...),
			}},
			{"kind": "text", "content": openWord(lang, "sorted"), "size": "xs", "mono": true, "color": "dimmed"},
		}},
		table,
	}}
}

// OpenPage answers the Open increments table: the open
// increments in no release of the stream and the hold asked, by priority then
// rank, the first `shown` of them, « Afficher plus » for the next page.
func (i *Increments) OpenPage(ctx context.Context, in api.OpenIncrementsInput) (render.Def, error) {
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	backlog, err := i.Backlog(ctx, api.IncrementBacklogInput{Bare: true})
	if err != nil {
		return render.Def{}, err
	}
	search := in.Search
	if q, ok := in.Selection[openSearchKey].(string); ok && search == "" {
		search = q
	}
	lines := openLines(backlog.Items, in.Stream, in.Held, search)
	shown := in.Shown
	if shown <= 0 {
		shown = openPageSize
	}
	lang := in.Language
	w := func(k string) string { return openWord(lang, k) }
	var cards []card
	for n, it := range lines {
		if n == shown {
			break
		}
		cards = append(cards, openCard(ctx, cwm, lang, it))
	}
	// None: the list says so once, in the page's language (no default of its
	// own beside a line of the page's).
	kids := []node{cardList(cards, 2, nil, node{"data-view": "open-increments", "empty": w("empty")})}
	if rest := len(lines) - shown; rest > 0 {
		kids = append(kids, node{"kind": "hbox", "gap": "sm", "justify": "center", "align": "center", "data-view": "open-increments-more", "children": []node{
			{"kind": "text", "content": fmt.Sprintf(w("more"), rest, openPageSize), "size": "xs", "mono": true, "color": "dimmed"},
			render.Button(w("show_more"), render.SelectOn(w("show_more"), openShownKey, shown+openPageSize)).With("variant", "subtle").With("size", "xs").With("data-show-more", "open-increments"),
		}})
	}
	page := node{"kind": "vbox", "gap": "xs", "data-view": "open-increments-table", "children": kids}
	return render.Def{Kind: "page", Title: "Open increments", DSLTree: page}, nil
}

// openLines are the backlog's increments the filters keep - a stream's (none:
// in no stream; all or empty: every one), held or not held (the default),
// those whose id or title hold the search (ignoring case; empty: every one) -
// by priority, then rank. The paging comes after, so « … n more » follows.
func openLines(items []api.IncrementBacklogItem, stream, held, search string) []api.IncrementBacklogItem {
	q := strings.ToLower(strings.TrimSpace(search))
	var out []api.IncrementBacklogItem
	for _, it := range items {
		inc := it.Increment
		switch stream {
		case "", openAllStreams:
		case noStreamRow:
			if inc.Stream != "" {
				continue
			}
		default:
			if inc.Stream != stream {
				continue
			}
		}
		if (held == openHeld) != (inc.Held != nil) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(inc.ID), q) && !strings.Contains(strings.ToLower(inc.Title), q) {
			continue
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(a, b int) bool {
		x, y := out[a].Increment, out[b].Increment
		if pa, pb := priorityRank(incrementRowPriority(x)), priorityRank(incrementRowPriority(y)); pa != pb {
			return pa < pb
		}
		if na, nb := idNumber(x.ID), idNumber(y.ID); na != nb {
			return na < nb
		}
		return x.ID < y.ID
	})
	return out
}

// openCard is one increment as the planification card: its id (opening it),
// title, held tag, its stream and its one-line description; at its foot its
// priority, Hold… or Resume, the release it ships in, and Run while it can
// run.
func openCard(ctx context.Context, cwm cwmapi.CWMService, lang string, it api.IncrementBacklogItem) card {
	inc := it.Increment
	w := func(k string) string { return openWord(lang, k) }
	var badges []node
	tone := "blue"
	if inc.Held != nil {
		badges = append(badges, node(render.Badge(w("held_tag"), "yellow").With("size", "xs").With("data-held", inc.ID)))
		tone = "yellow"
	}
	stream := it.StreamName
	if inc.Stream == "" {
		stream = w("no_stream")
	}
	badges = append(badges, node{"kind": "badge", "text": stream, "size": "xs", "variant": "light", "color": "gray", "data-stream": inc.Stream})
	priority := incrementRowPriority(inc)
	manage := []node{
		render.Menu(priority, render.MenuEntry(render.Action{ID: "priority", Label: w("priority"), URI: assessURI, Args: map[string]any{"id": inc.ID, "priority": "${value}", "by": "${person}"}}, "", strings.Join(boardPriorities, "|"))).
			With("variant", "default").With("data-control", "priority"),
	}
	if inc.Held != nil {
		manage = append(manage, render.Button(w("resume"), render.RunURI(w("resume"), incrementResumeURI, map[string]any{"id": inc.ID, "by": "${person}"})).
			With("variant", "light").With("color", "yellow").With("size", "xs").With("data-control", "resume"))
	} else {
		manage = append(manage, render.Node(drawerButton(w("hold"), w("hold_title"), render.AnswerForm(holdFormSchema(w("hold_reason")),
			render.RunURI(w("hold_title"), boardMoveURI, withLanguage(map[string]any{"id": inc.ID, "column": heldColumn, "reason": "${value.reason}", "by": "${person}"}, lang)), "player-pause", incrementHoldForm{}))).
			With("size", "xs").With("data-control", "hold"))
	}
	manage = append(manage, openShipsMenu(ctx, cwm, lang, inc))
	if whyNotRunnable(inc) == "" {
		// Not yet run, with its goals, not held: the person's go
		//
		pw := paneData{lang: lang}.actWord
		manage = append(manage, runButton(inc.ID, pw("start"), pw("start_title")))
	}
	return card{ID: inc.ID, Title: inc.Title, Badges: badges, Meta: strings.TrimSpace(inc.Description), Actions: manage, Tone: tone,
		Attrs: node{"data-increment": inc.ID}}
}

// openShipsMenu is the release an increment ships in, a menu offering its
// stream's planned releases - the next ships it in, a later one plans it - and
// « No release »; for an increment in no stream, the streams to put it in.
func openShipsMenu(ctx context.Context, cwm cwmapi.CWMService, lang string, inc api.Increment) node {
	w := func(k string) string { return openWord(lang, k) }
	action := render.Action{ID: "ships-in", Label: w("ships_in"), URI: boardShipURI, Args: withLanguage(map[string]any{"id": inc.ID, "in": "${value}", "by": "${person}"}, lang)}
	if inc.Stream == "" {
		var opts []map[string]any
		if lanes, err := NewStreams().Lanes(ctx, api.LanesInput{}); err == nil {
			for _, l := range lanes.Lanes {
				opts = append(opts, map[string]any{"value": l.ID, "label": l.Name})
			}
		}
		action.ID, action.Label = "stream", w("stream")
		entry := render.MenuEntry(action, "", "")
		entry["choice"] = map[string]any{"options": opts}
		return render.Menu(w("no_stream"), entry).With("variant", "default").With("data-control", "stream")
	}
	current := inc.ShipsIn + inc.PlannedFor
	label := w("none")
	if current != "" {
		label = releaseVersionOf(ctx, cwm, current)
	}
	var opts []map[string]any
	for n, r := range plannedReleases(ctx, cwm, inc.Stream) {
		l := r.label
		if n == 0 {
			l += w("next")
		}
		opts = append(opts, map[string]any{"value": r.id, "label": l})
	}
	opts = append(opts, map[string]any{"value": "out", "label": w("no_release")})
	entry := render.MenuEntry(action, "", "")
	entry["choice"] = map[string]any{"options": opts}
	return render.Menu(label, entry).With("variant", "default").With("data-control", "ships-in")
}

// releaseVersionOf is a release's version (1.007), else its id.
func releaseVersionOf(ctx context.Context, cwm cwmapi.CWMService, id string) string {
	o, _, err := loadAny(ctx, cwm, id)
	if err != nil {
		return id
	}
	if v := releaseFields(o).Version; v != "" {
		return v
	}
	return id
}

// assessURI changes a request's - an increment's - priority.
const assessURI = "ktw-agents:requests/assess"
