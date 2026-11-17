// SPDX-License-Identifier: Apache-2.0

package core

import (
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE TIMELINE of a demand: its activity, oldest first — each step the planner
// started (its run and that run's session, the transcript; or the inbox
// request a person was asked in) and each move of its plan (planned, ended,
// waits, blocked, split, done, dropped). Each step keeps its own session: the
// timeline links them, it does not merge them.

// moveTitles say what a plan's transition means, for a person.
var moveTitles = map[string]string{
	"plan": "Planned", "settle": "Step ended", "wait": "Waits", "block": "Blocked",
	"split": "Split into parts", "finish": "Done", "drop": "Dropped",
}

// timelineOf merges a plan's steps and its moves, oldest first — and a
// waiver of its module update. A start move is the step itself: the step's
// entry stands for it.
func timelineOf(f api.PlanFields, w *api.Waiver) []api.TimelineEntry {
	var out []api.TimelineEntry
	for _, s := range f.Steps {
		e := api.TimelineEntry{At: s.StartedAt, Kind: api.TimelineStep, Run: s.Run, Session: s.Session, Ask: s.Ask}
		if s.Doer == api.DoerPerson {
			e.Title = "Asked a person"
		} else {
			e.Title = "Started " + s.Task
		}
		if s.Step != "" {
			e.Note = s.Step
		}
		if s.Phase != "" {
			if e.Note != "" {
				e.Note += " · "
			}
			e.Note += s.Phase
		}
		out = append(out, e)
	}
	for _, m := range f.Trail {
		if m.Transition == "start" {
			continue
		}
		title := moveTitles[m.Transition]
		if title == "" {
			title = m.Transition
		}
		out = append(out, api.TimelineEntry{At: m.At, Kind: api.TimelineMove, Title: title, Note: m.Note,
			State: stateLabels[m.To], By: m.By.ID})
	}
	if w != nil {
		// A person waived its module update: a move of its end, shown in time.
		out = append(out, api.TimelineEntry{At: w.At, Kind: api.TimelineMove, Title: "Module update waived", Note: w.Note, By: w.By.ID})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}
