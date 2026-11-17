// SPDX-License-Identifier: Apache-2.0

package core

import (
	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// THE PLANIFICATION CARD (the person's word of
// 2026-11-03: « keep this card everywhere »). Every planification list draws
// its items as the deck's « Needs you » card: a bordered card with a tone edge
// (the list's toneField), a bold title, the id badge, a state badge and one
// dimmed line. One builder, cardNode, draws it - bound to a list's elements
// (itemCard) or from a view's own values (card) -, cardList lays the cards in
// a grid. A view adds what it carries generically: more badges, a body under
// the lines (an increment's open requests), the controls a person manages at
// its foot.

// card is one item drawn as the planification card.
type card struct {
	ID, Title string
	// State is the state badge's word and StateColor its colour; no badge
	// when empty.
	State, StateColor string
	// Badges come after the state badge (a step, an increment, a stream).
	Badges []node
	// Meta is the one dimmed line, clamped to MetaLines lines (one when 0),
	// MetaAttrs its element ids.
	Meta      string
	MetaLines int
	MetaAttrs node
	// OpenLabel names the id's action in the person's language, when the id
	// opens the item (« Open <id> » when empty).
	OpenLabel string
	// Body is drawn under the card's lines; Actions at its foot.
	Body, Actions []node
	// Tone is the colour of the card's edge.
	Tone string
	// Attrs are the card's element ids (data-*), on its node.
	Attrs node
}

// cardNode is the card's face: its title, then the id, the state badge, the
// other badges and the dimmed line on one wrapping line, then its body and
// its actions.
func cardNode(title, id, state, meta node, badges, body, actions []node) node {
	line := []node{id}
	if state != nil {
		line = append(line, state)
	}
	line = append(line, badges...)
	if meta != nil {
		line = append(line, meta)
	}
	kids := []node{title, {"kind": "hbox", "gap": 6, "wrap": true, "align": "center", "children": line}}
	kids = append(kids, body...)
	if len(actions) > 0 {
		kids = append(kids, node{"kind": "hbox", "gap": "xs", "wrap": true, "align": "center", "justify": "flex-end", "data-view": "card-actions", "children": actions})
	}
	return node{"kind": "vbox", "gap": 4, "children": kids}
}

// itemCard is the card bound to a list's element: its title, its id, its
// state as a coloured badge, and a dimmed line.
func itemCard() node {
	return cardNode(
		node{"kind": "text", "content": "", "dataField": "$.item.title", "size": "sm", "fw": 600},
		idBadge(),
		node{"kind": "badge", "text": "", "dataField": "$.item.state", "size": "xs", "variant": "light", "colors": labelColors()},
		node{"kind": "text", "content": "", "dataField": "$.item.meta", "size": "xs", "color": "dimmed"},
		nil, nil, nil)
}

// node is the card drawn from its values. Its id opens the item when the card
// holds controls or rows of its own (opens false: a click on the card would
// run under theirs); else it is a badge, the whole card opening it.
func (c card) node(opens bool) node {
	title := node{"kind": "text", "content": c.Title, "size": "sm", "fw": 600, "data-title": c.ID}
	id := node{"kind": "badge", "text": c.ID, "size": "xs", "variant": "outline", "color": "gray", "mono": true}
	if !opens {
		id = idChip(c.ID)
		if c.OpenLabel != "" {
			id["action"] = render.NavigateTo(c.OpenLabel, c.ID, "")
		}
	}
	var state, meta node
	if c.State != "" {
		state = node{"kind": "badge", "text": c.State, "size": "xs", "variant": "light", "color": c.StateColor}
	}
	if c.Meta != "" {
		lines := c.MetaLines
		if lines == 0 {
			lines = 1
		}
		meta = node{"kind": "text", "content": c.Meta, "size": "xs", "color": "dimmed", "lineClamp": lines}
		for k, v := range c.MetaAttrs {
			meta[k] = v
		}
	}
	n := cardNode(title, id, state, meta, c.Badges, c.Body, c.Actions)
	for k, v := range c.Attrs {
		n[k] = v
	}
	return n
}

// cardList lays cards in a grid of columns, each with its tone edge; open,
// when given, is what a click on a card runs (${item.id} its id) - a card
// holding controls or rows of its own is opened by its id instead.
func cardList(cards []card, columns int, open *render.Action, attrs node) node {
	data := make([]node, 0, len(cards))
	for _, c := range cards {
		tone := c.Tone
		if tone == "" {
			tone = "gray"
		}
		data = append(data, node{"id": c.ID, "tone": tone, "card": c.node(open != nil)})
	}
	n := node{"kind": "list", "layout": "grid", "columns": columns, "data": data, "nodeField": "card", "toneField": "tone"}
	if open != nil {
		n["open"] = *open
	}
	for k, v := range attrs {
		n[k] = v
	}
	return n
}
