// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE DELIVERY VIEW - every stream as a lane on the
// timeline: its release mode and next cut with Change mode, Cut now and New
// increment, its releases as cards, its backlog with the held increments among
// it; every item shows its id and opens on a click. Called with a stream's id
// it is that stream's lane alone, its next release selected: the stream's own
// pane, the same builder. The screen is
// delivery_pages.go's; the rows are the delivery data types'.

// DeliveryPage answers the Delivery view: the screen of the approved mock-up
// over every stream's lane, or one stream's - its own pane, its next release
// selected (delivery_pages.go).
func (s *Streams) DeliveryPage(ctx context.Context, in api.DeliveryPageInput) (render.Def, error) {
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	one := in.ID
	if one == "" && isStreamID(in.Target) {
		one = in.Target
	}
	lanes, err := s.Lanes(ctx, api.LanesInput{ID: one, Language: in.Language})
	if err != nil {
		return render.Def{}, err
	}
	title := deliveryWord(in.Language, "title")
	if one != "" && len(lanes.Lanes) == 1 {
		title = lanes.Lanes[0].Name
	}
	screen := deliveryScreen(ctx, cwm, lanes.Lanes, title, in.Language)
	if one != "" {
		return screen, nil // a stream's own pane: its lane alone
	}
	// The Delivery view's tabs: Planned delivery
	// Running, Open increments and Delivered
	//
	backlog, err := NewIncrements().Backlog(ctx, api.IncrementBacklogInput{Bare: true})
	if err != nil {
		return render.Def{}, err
	}
	// The Running tab is a section loaded when it opens:
	// the page counts the increments being built and builds none of them.
	// Open increments is one table.
	open := openIncrementsTable(in.Language, lanes.Lanes, backlog.Items)
	if d, ok := screen.Data.(node); ok {
		if sel, ok := d["selection"].(node); ok {
			for k, v := range openSelection() {
				sel[k] = v
			}
		}
	}
	tabs := []node{
		{"id": "planned", "label": deliveryWord(in.Language, "tab_planned"), "badge": plannedCount(ctx), "content": screen.DSLTree},
		{"id": "running", "label": runningWord(in.Language, "tab"), "badge": deliveryRunningCount(ctx), "content": runningContent(in.Language)},
		{"id": "open", "label": deliveryWord(in.Language, "tab_open"), "badge": len(backlog.Items), "content": open},
		{"id": "delivered", "label": deliveredWord(in.Language, "tab"), "badge": deliveredCount(ctx), "badgeColor": "green", "content": deliveredTab(in.Language)},
	}
	screen.DSLTree = node{"kind": "tabs", "data-view": "delivery-tabs", "tabs": tabs}
	return screen, nil
}

// isStreamID is whether a target names a stream (SM-n).
func isStreamID(id string) bool {
	_, local, ok := cutID(id)
	return ok && len(local) > 3 && local[:3] == "SM-"
}

// cutID splits a module-qualified id.
func cutID(id string) (module, local string, ok bool) {
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] == '.' {
			return id[:i], id[i+1:], true
		}
	}
	return "", "", false
}

// runningPageURI loads the Running tab.
const runningPageURI = "ktw-agents:streams/running-page"

// runningContent is the Running tab's content: a section loading it when it
// opens.
func runningContent(lang string) node {
	args := node{}
	if lang != "" {
		args["language"] = lang
	}
	return loadedSection("delivery:running", runningPageURI, args, "", render.PlaceholderCards, 3)
}

// RunningPage answers the Running tab, loaded when it opens: every increment
// being built with its open requests (F-098).
func (s *Streams) RunningPage(ctx context.Context, in api.DeliveryPageInput) (render.Def, error) {
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	return render.Def{Kind: "page", Title: runningWord(in.Language, "tab"), DSLTree: runningTab(in.Language, deliveryRunning(ctx, cwm))}, nil
}
