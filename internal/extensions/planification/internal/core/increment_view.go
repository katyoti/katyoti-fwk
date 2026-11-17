// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE INCREMENT'S DETAILED VIEW - its pane says where it
// is assigned and lets the person change it, then each delivery goal down to
// what it became and each request serving that with where it is in its work:
// its state, its current step, who holds it, what it awaits; with its goals
// editor and a dependency editor.

// The increment verbs the detailed view runs.
const (
	incrementSetGoalsURI = "ktw-agents:increments/set-goals"
	requestDependURI     = "ktw-agents:requests/depend"
)

// goalsForm writes an increment's delivery goals.
type goalsForm struct {
	Goals []string `json:"goals" jsonschema:"description=Its delivery goals - one per line - in the user's words"`
}

// dependForm orders an increment after another.
type dependForm struct {
	On string `json:"on" jsonschema:"description=The increment it waits on (its id),required"`
}

// assignment says where an increment is: ships in, planned for, or not
// assigned - each release by its title, opening it.
func assignment(ctx context.Context, cwm cwmapi.CWMService, inc api.Increment) node {
	switch {
	case inc.ShipsIn != "":
		name := titleOfID(ctx, cwm, inc.ShipsIn)
		return render.Link("Ships in "+name+" ("+inc.ShipsIn+")", render.NavigateTo("Ships in "+name, inc.ShipsIn, ""))
	case inc.PlannedFor != "":
		name := titleOfID(ctx, cwm, inc.PlannedFor)
		return render.Link("Planned for "+name+" ("+inc.PlannedFor+") - ships in it once it is next", render.NavigateTo("Planned for "+name, inc.PlannedFor, ""))
	}
	say := "Not assigned: it waits in its stream's backlog"
	if inc.Stream == "" {
		say = "Not assigned, and aimed at no stream"
	}
	if inc.Returned != "" {
		say += " - " + inc.Returned
	}
	return node{"kind": "text", "content": say + ".", "size": "sm", "color": "orange", "data-assignment": "none"}
}

// whereInWork is where a request is in its work: its current step and who
// holds it, or what holds it as the one reader says (holdWords,
// ) - read from its to-dos.
func whereInWork(ctx context.Context, request string) string {
	todos, err := todosOf(ctx, request)
	if err != nil {
		return "its steps could not be read"
	}
	var open *api.WorkTodo
	for i := range todos {
		t := &todos[i]
		step, _ := t.Work["step"].(string)
		switch t.State {
		case schedapi.StateClaimed:
			who := ""
			if t.Claim != nil {
				who = " - held by " + t.Claim.By.ID
			}
			return "step " + step + who
		case schedapi.StateReady:
			if open == nil || open.State != schedapi.StateReady {
				open = t
			}
		case schedapi.StateWaiting:
			if open == nil {
				open = t
			}
		}
	}
	if open == nil {
		return "no step open"
	}
	step, _ := open.Work["step"].(string)
	if cwm, err := cwmOf(ctx); err == nil {
		if o, err := loadObject(ctx, cwm, request); err == nil {
			if b, ok := holdOf(ctx, o); ok {
				return "step " + orDefault(b.Step, step) + " - " + holdWords(b)
			}
		}
	}
	if open.State == schedapi.StateReady {
		return "step " + step + " - ready"
	}
	return "step " + step + " - waits"
}
