// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// THE INCREMENT CARD — one increment as every view shows it (the backlog, the
// portfolio's planning boards, a stream's delivery): its priority, its title
// opening it, its id, its state, its stream and release, and - when its
// summary is read - a bar of what it holds done, a stacked bar of its
// requests and the first thing it waits on. One builder, so no view draws an
// increment its own way.

// IncrementCardInput is what a view knows of an increment for its card.
type IncrementCardInput struct {
	Increment Increment
	// Summary is where it stands (Summary); nil when the view does not show it.
	Summary *IncrementSummaryOutput
	// Unread says why its summary could not be read: shown instead of bars
	// that would look true.
	Unread string
	// StreamName and ReleaseName name its stream and the release it ships in.
	StreamName, ReleaseName string
	// Action is the card's one action (« Ship in … »), or nil.
	Action render.Node
	// Body is what a view shows under it (the tree of what it holds).
	Body []render.Node
	// ShowID shows its id beside its title (the backlog); otherwise the id is
	// on hover only (the portfolio's boards).
	ShowID bool
	// French words the card in French.
	French bool
}

// IncrementCard is the increment's card.
func IncrementCard(in IncrementCardInput) render.Node {
	say := func(fr, en string) string {
		if in.French {
			return fr
		}
		return en
	}
	inc := in.Increment
	priority := inc.Priority
	if priority == "" {
		priority = DefaultPriority
	}
	standing := IncrementStanding(IncrementStandingInput{Increment: inc})
	if in.Summary != nil && in.Summary.Standing != "" {
		standing = in.Summary.Standing
	}
	state, color := StandingLabel(standing, in.French)
	head := render.Node{"kind": "hbox", "gap": 6, "wrap": true, "children": []render.Node{
		{"kind": "badge", "text": priority, "variant": "filled", "color": map[string]string{"P0": "red", "P1": "orange", "P2": "blue", "P3": "gray"}[priority], "tooltip": inc.ID},
		{"kind": "button", "label": inc.Title, "variant": "subtle", "size": "xs",
			"action": render.Node{"navigate": render.Node{"object": inc.ID}, "label": inc.Title}},
		{"kind": "badge", "text": state, "variant": "light", "color": color},
	}}
	if in.ShowID {
		head["children"] = append(head["children"].([]render.Node)[:2:2],
			render.Node{"kind": "badge", "text": inc.ID, "variant": "outline", "color": "gray", "size": "xs", "keepCase": true, "mono": true},
			head["children"].([]render.Node)[2])
	}
	kids := []render.Node{head}
	var meta []string
	if in.StreamName != "" {
		meta = append(meta, in.StreamName)
	}
	if in.ReleaseName != "" {
		meta = append(meta, say("livré dans ", "ships in ")+in.ReleaseName)
	}
	if len(meta) > 0 {
		kids = append(kids, render.Node{"kind": "text", "content": strings.Join(meta, " · "), "size": "xs", "color": "dimmed"})
	}
	if inc.Returned != "" {
		kids = append(kids, render.Node{"kind": "text", "content": inc.Returned, "size": "xs", "color": "orange"})
	}
	switch {
	case in.Unread != "":
		kids = append(kids, render.Node{"kind": "text", "content": say("Son avancement n'a pas pu être lu : ", "Its progress could not be read: ") + in.Unread, "size": "xs", "color": "red"})
	case in.Summary != nil:
		sum := in.Summary
		kids = append(kids,
			render.Node{"kind": "gauge", "label": say("Ce qu'il contient", "What it holds"), "value": sum.Done, "max": max(1, sum.Total),
				"valueText": fmt.Sprintf("%d/%d", sum.Done, sum.Total), "size": "sm"},
			render.Node{"kind": "gauge", "legend": true, "size": "sm",
				"valueText": fmt.Sprintf(say("%d/%d demandes faites", "%d/%d requests done"), sum.RequestsDone, sum.Requests),
				"segments": []render.Node{
					{"value": sum.RequestsDone, "color": "green", "label": say("faites", "done")},
					{"value": sum.Running, "color": "blue", "label": say("en cours", "in progress")},
					{"value": sum.ToDo, "color": "cyan", "label": say("à faire", "to do")},
					{"value": sum.Awaiting, "color": "grape", "label": say("attendent une personne", "awaiting a person")},
					{"value": sum.WaitingOn, "color": "violet", "label": say("en attente d'une autre", "waiting")},
					{"value": sum.Blocked, "color": "red", "label": say("bloquées", "blocked")},
					{"value": sum.Idle, "color": "pink", "label": say("plus rien à faire", "nothing left")},
				}})
		if len(sum.Waits) > 0 {
			kids = append(kids, render.Node{"kind": "text", "content": sum.Waits[0], "size": "xs", "color": "orange"})
		}
	}
	kids = append(kids, in.Body...)
	if in.Action != nil {
		kids = append(kids, in.Action)
	}
	return render.Node{"kind": "card", "withBorder": true, "p": "xs", "radius": "sm", "gap": 4, "data-increment": inc.ID, "children": kids}
}
