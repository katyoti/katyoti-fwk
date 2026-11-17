// SPDX-License-Identifier: Apache-2.0

package render

// The one object card (A): an object drawn as a
// card is the display library's `object-card` node filled from its CWM row.
// A producer passes rows - ObjectCardRow - and never assembles a card of
// nodes of its own (rule; internal/test/arch's card guard).

// CardBadge is one mark of an object card: a CWM badge as the object's row
// carries it (its JSON is cwm's Badge's).
type CardBadge struct {
	ID      string `json:"id,omitempty"`
	Label   string `json:"label"`
	Icon    string `json:"icon,omitempty"`
	Color   string `json:"color,omitempty"`
	Tooltip string `json:"tooltip,omitempty"`
}

// ObjectCardRow is what an object card is filled from: the object's title,
// id and type, its lifecycle state, the badges its type and capabilities
// declare, its one next action and the entries of its « ⋯ » menu. Every
// action runs on the row (`${item.id}` is the object's id).
type ObjectCardRow struct {
	ID     string      `json:"id"`
	Title  string      `json:"title"`
	Type   string      `json:"type,omitempty"`
	Icon   string      `json:"icon,omitempty"`
	State  *CardBadge  `json:"state,omitempty"`
	Badges []CardBadge `json:"badges,omitempty"`
	Next   *Action     `json:"next,omitempty"`
	Menu   []Action    `json:"menu,omitempty"`
}

// ObjectCard is one object's card; open, when set, runs on a click of the
// card (opening the object).
func ObjectCard(row ObjectCardRow, open *Action) Node {
	n := Node{"kind": "object-card", "row": row}
	if open != nil {
		n["open"] = *open
	}
	return n
}

// ObjectCards draws each row as its object card, in a grid of columns (3
// when columns is 0), empty the text shown when there is none.
func ObjectCards(rows []ObjectCardRow, columns int, empty string, open *Action) Node {
	if columns <= 0 {
		columns = 3
	}
	if rows == nil {
		rows = []ObjectCardRow{}
	}
	item := Node{"kind": "object-card", "dataField": "$.item"}
	if open != nil {
		item["open"] = *open
	}
	n := Node{"kind": "list", "layout": "grid", "columns": columns, "data": rows, "item": item}
	if empty != "" {
		n["empty"] = empty
	}
	return n
}
