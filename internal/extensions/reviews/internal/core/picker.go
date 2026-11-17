// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// startPicker is the Reviews panel's New review section: the review types
// that apply to the object, grouped by dimension — a button per type, one per
// dimension running all its types — and one per preset. Each starts a run
// (reviews/start); a run whose estimate is over the threshold asks to be
// confirmed first. Empty when no type applies.
func startPicker(lang, provider, object string, types api.TypesOutput) render.Node {
	if len(types.Dimensions) == 0 {
		return nil
	}
	uri := provider + ":reviews/start"
	cost := map[string]float64{}
	for _, d := range types.Dimensions {
		for _, t := range d.Types {
			cost[t.Name] = t.MaxCostUSD
		}
	}
	start := func(label string, args map[string]any, names []string) render.Action {
		var est float64
		for _, n := range names {
			est += cost[n]
		}
		args["object"] = object
		a := render.RunURI(label, uri, args)
		if est > api.ConfirmOverUSD {
			args["confirm"] = true
			a.Confirm = fmt.Sprintf(tr(lang, "confirmCost"), est)
		}
		return a
	}
	var rows []render.Node
	for _, d := range types.Dimensions {
		var names []string
		line := []render.Node{render.Text(d.Dimension)}
		for _, t := range d.Types {
			names = append(names, t.Name)
			line = append(line, render.Button(t.Title, start(t.Title, map[string]any{"types": []string{t.Name}}, []string{t.Name})))
		}
		if len(d.Types) > 1 {
			line = append(line, render.Button(fmt.Sprintf(tr(lang, "allOf"), d.Dimension),
				start(d.Dimension, map[string]any{"dimension": d.Dimension}, names)))
		}
		rows = append(rows, render.HBox(line...))
	}
	if len(types.Presets) > 0 {
		line := []render.Node{render.Text(tr(lang, "presets"))}
		for _, p := range types.Presets {
			line = append(line, render.Button(p.Name, start(p.Name, map[string]any{"preset": p.Name}, p.Types)))
		}
		rows = append(rows, render.HBox(line...))
	}
	return render.Section(tr(lang, "startReview"), "play", rows...)
}
