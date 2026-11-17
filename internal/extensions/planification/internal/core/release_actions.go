// SPDX-License-Identifier: Apache-2.0

package core

import (
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// The release verbs its pane runs, as the person.
const (
	releaseFreezeURI      = "ktw-agents:releases/freeze"
	releaseDeliverURI     = "ktw-agents:releases/deliver"
	releaseLetterURI      = "ktw-agents:releases/letter"
	releaseSignOffURI     = "ktw-agents:releases/sign-off"
	releasePromoteURI     = "ktw-agents:releases/promote"
	releaseShipPlannedURI = "ktw-agents:releases/ship-planned"
	releaseUnfreezeURI    = "ktw-agents:releases/unfreeze"
	releaseRescheduleURI  = "ktw-agents:releases/reschedule"
	releaseDropURI        = "ktw-agents:releases/drop"
	incrementPlanForURI   = "ktw-agents:increments/plan-for"
	incrementUnplanURI    = "ktw-agents:increments/unplan"
)

// deliverForm delivers a candidate: its build, optional.
type deliverForm struct {
	Build string `json:"build,omitempty" jsonschema:"description=The build it is delivered as (a build id) - optional"`
}

// rescheduleForm moves a release's planned date or its level.
type rescheduleForm struct {
	Date  string `json:"date,omitempty" jsonschema:"description=Its new planned date (YYYY-MM-DD) - unchanged when empty"`
	Level string `json:"level,omitempty" jsonschema:"description=Its new level - unchanged when empty,enum=|major|minor|patch"`
}

// candidateForm names the candidate an act is on.
type candidateForm struct {
	Candidate string `json:"candidate" jsonschema:"description=The candidate,required"`
}

// releaseActions is what a person does to a release from its pane, each
// offered only in the stage where it applies: a planned
// release is frozen; a frozen one delivers a candidate; on the candidate chosen
// on the page (its last by default) its letter is written, then a person signs
// it off, then it is promoted once every gate passes on it. Each runs the
// release verb as the person; the verb refuses with its reason what a gate
// does not allow. Nothing for a released or dropped release.
func releaseActions(rel api.Release, state string, gates api.GatesOutput) node {
	var kids []node
	switch state {
	case api.ReleasePlanned:
		kids = append(kids, render.Button("Freeze it", render.RunURI("Freeze it", releaseFreezeURI, map[string]any{"id": rel.ID, "by": "${person}"})))
		kids = append(kids, steerRelease(rel)...)
	case api.ReleaseFrozen:
		kids = append(kids, render.AnswerForm(render.InlineSchema[deliverForm]("DeliverCandidate", map[string]string{"build": "Build"}),
			render.RunURI("Deliver a candidate", releaseDeliverURI, map[string]any{"id": rel.ID, "build": "${value.build}", "by": "${person}"}),
			"package", deliverForm{}))
		var names []any
		var last *api.Candidate
		for i := range rel.Candidates {
			names = append(names, rel.Candidates[i].Name)
			last = &rel.Candidates[i]
		}
		if last == nil {
			kids = append(kids, render.Button("Unfreeze it", render.RunURI("Unfreeze it", releaseUnfreezeURI, map[string]any{"id": rel.ID, "by": "${person}"})))
			kids = append(kids, steerRelease(rel)...)
			break
		}
		pick := func(label, uri, icon string) node {
			schema := render.InlineSchema[candidateForm]("Candidate"+icon, map[string]string{"candidate": "Candidate"})
			if t, ok := schema["types"].(map[string]any)["Candidate"+icon].(map[string]any); ok {
				if f, ok := t["fields"].(map[string]any)["candidate"].(map[string]any); ok {
					f["enum"] = names
				}
			}
			return render.AnswerForm(schema, render.RunURI(label, uri, map[string]any{"id": rel.ID, "candidate": "${value.candidate}", "by": "${person}"}),
				icon, candidateForm{Candidate: last.Name})
		}
		switch {
		case last.Letter == "":
			kids = append(kids, pick("Write its letter", releaseLetterURI, "mail"))
		case last.SignedOff == nil:
			kids = append(kids, pick("Sign it off", releaseSignOffURI, "signature"))
		}
		if gates.Ready || onlyChangeLogOpen(gates) {
			kids = append(kids, pick("Promote it", releasePromoteURI, "rocket"))
		}
		kids = append(kids, steerRelease(rel)...)
	default:
		return nil
	}
	return node{"kind": "vbox", "gap": "xs", "children": kids}
}

// steerRelease is what changes a release still to ship, as the person
// : its date or level, or dropping it - its increments
// then wait in the backlog.
func steerRelease(rel api.Release) []node {
	return []node{
		render.AnswerForm(render.InlineSchema[rescheduleForm]("RescheduleRelease", map[string]string{"date": "New date", "level": "Level"}),
			render.RunURI("Reschedule it", releaseRescheduleURI, map[string]any{"id": rel.ID, "date": "${value.date}", "level": "${value.level}", "by": "${person}"}),
			"calendar", rescheduleForm{Date: rel.Date}),
		render.Button("Drop it", render.Action{Label: "Drop it", URI: releaseDropURI, Args: map[string]any{"id": rel.ID, "by": "${person}"},
			Confirm: "Drop this release? It will not ship: its increments, and those planned for it, wait in the backlog.", Dangerous: true}),
	}
}
