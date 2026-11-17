// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// Contents is what an object holds, as the page the Contents panel shows:
// every object of its subtree — a project's is the whole index, since its
// modules hang under it through the index, not through a parent — grouped by
// type, each with its state and the object it sits under. Chips filter by
// type and state, a form by text; each re-reads this verb with the filter.
// The id is bare or qualified; when the universe serves several projects the
// contents are those of the project holding the object (api.Holding). Read
// from CWM's index only.
func (s *Service) Contents(ctx context.Context, in api.ContentsInput) (render.Def, error) {
	if in.ID == "" {
		return render.Def{}, fmt.Errorf("%w: contents: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return render.Def{}, fmt.Errorf("object-pane: resolve CWMService: %w", err)
	}
	rows, err := api.Holding(ctx, cwm, in.ID, strings.TrimSpace(in.Text))
	if err != nil {
		return render.Def{}, err
	}
	return render.Def{Kind: "page", DSLTree: contentsTree(in, rows), Data: map[string]any{"filter": map[string]any{"text": in.Text}}}, nil
}

// contentsTree draws the contents: the type chips (counted over everything
// the text keeps), the state chips (over the chosen type), the text form, then
// the rows the filter keeps, by type.
func contentsTree(in api.ContentsInput, rows []api.ObjectRow) render.Node {
	lang := in.Language
	q := map[string]any{"type": in.Type, "state": in.State, "text": in.Text}
	ofType := rows
	if in.Type != "" {
		ofType = filterRows(rows, func(r api.ObjectRow) bool { return r.Type == in.Type })
	}
	shown := ofType
	if in.State != "" {
		shown = filterRows(ofType, func(r api.ObjectRow) bool { return r.State == in.State })
	}

	typeChips := []api.Chip{{Label: tr(lang, "all"), Count: len(rows), Active: in.Type == "", Query: api.Query(api.Query(q, "type", ""), "state", "")}}
	for _, g := range api.GroupByType(rows) {
		typeChips = append(typeChips, api.Chip{Label: g.Label, Count: len(g.Rows), Color: "blue", Active: in.Type == g.Type,
			Query: api.Query(api.Query(q, "type", g.Type), "state", "")})
	}
	blocks := []render.Node{api.Chips(tr(lang, "types"), typeChips)}
	if chips := api.StateChips(tr(lang, "states"), tr(lang, "all"), ofType, in.State, q); chips != nil {
		blocks = append(blocks, chips)
	}
	blocks = append(blocks, textForm(lang, map[string]any{"type": in.Type, "state": in.State}))

	title := fmt.Sprintf(tr(lang, "shownOf"), len(shown), len(rows))
	body := render.Section(title, "list-tree").With("size", "md").With("color", "blue").With("badge", len(shown))
	if len(shown) == 0 {
		empty := tr(lang, "nothingHere")
		if len(rows) > 0 {
			empty = tr(lang, "nothingMatches")
		}
		body = body.Empty(empty, "")
	} else {
		body["children"] = []render.Node{api.ByType(shown, nil)}
	}
	return render.VBox(append(blocks, body)...).With("gap", "md")
}

func filterRows(rows []api.ObjectRow, keep func(api.ObjectRow) bool) []api.ObjectRow {
	var out []api.ObjectRow
	for _, r := range rows {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out
}

// textFilter is the one field of the text form.
type textFilter struct {
	Text string `json:"text,omitempty" jsonschema:"description=Words in the id or name or description or tags"`
}

// textForm is a one-field form bound to the page's `filter`: submitting it
// re-reads the source with the text and keep (the rest of the filter).
func textForm(lang string, keep map[string]any) render.Node {
	args := map[string]any{"text": "${value.text}"}
	for k, v := range keep {
		if v != "" {
			args[k] = v
		}
	}
	schema := render.InlineSchema[textFilter]("TextFilter", map[string]string{"text": tr(lang, "search")})
	return render.Form(schema, render.QueryWith(tr(lang, "search"), args)).With("dataField", "$.filter").With("mode", "edit")
}
