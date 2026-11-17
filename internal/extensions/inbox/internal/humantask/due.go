// SPDX-License-Identifier: Apache-2.0

package humantask

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// BY WHEN: a question can say by when it must be answered. At
// its follow-up date it reminds - once, recorded on its to-do and announced;
// at its due date it escalates - handed back to its holding list (no longer
// given to one member), its priority raised one step, recorded and announced
// - and it shows overdue. Its run keeps the dates: it wakes at the next one,
// or at its hourly recheck, whichever comes first.

// clock is the time the dates are read against (a test sets it).
var clock = time.Now

// whenOf is a date a human task names - RFC3339, or a duration from when it
// is asked ("48h") - as RFC3339, or "" when it names none or none that reads.
func whenOf(v string, asked time.Time) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return asked.Add(d).UTC().Format(time.RFC3339)
	}
	slog.Warn("inbox: a human task's date does not read (RFC3339 or a duration); it is ignored", "date", v)
	return ""
}

// workTime is a time a to-do's work holds, and whether it holds one.
func workTime(t schedapi.Todo, key string) (time.Time, bool) {
	s, _ := t.Work[key].(string)
	if s == "" {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339, s)
	return at, err == nil
}

// nextDate is the next date the run must wake for: the follow-up not yet
// reminded, else the due date not yet escalated; zero when none is left.
func nextDate(t schedapi.Todo) time.Time {
	if _, done := workTime(t, api.WorkReminded); !done {
		if at, ok := workTime(t, api.WorkFollowUp); ok {
			return at
		}
	}
	if _, done := workTime(t, api.WorkEscalated); !done {
		if at, ok := workTime(t, api.WorkDue); ok {
			return at
		}
	}
	return time.Time{}
}

// remindOrEscalate reminds a question past its follow-up date and escalates
// one past its due date, each once. A to-do a worker holds is left for the
// next wake: its holder answers or lets the lease pass.
func remindOrEscalate(ctx context.Context, todos schedapi.TodoService, t schedapi.Todo, now time.Time) schedapi.Todo {
	if t.State != schedapi.StateReady && t.State != schedapi.StateClaimed {
		return t
	}
	changed := t
	changed.Work = map[string]any{}
	for k, v := range t.Work {
		changed.Work[k] = v
	}
	var said []string
	if at, ok := workTime(t, api.WorkFollowUp); ok && !now.Before(at) {
		if _, done := workTime(t, api.WorkReminded); !done {
			changed.Work[api.WorkReminded] = now.UTC().Format(time.RFC3339)
			said = append(said, "reminded")
		}
	}
	if at, ok := workTime(t, api.WorkDue); ok && !now.Before(at) {
		if _, done := workTime(t, api.WorkEscalated); !done {
			changed.Work[api.WorkEscalated] = now.UTC().Format(time.RFC3339)
			changed.Assignee = nil
			changed.Priority = raised(t.Priority)
			said = append(said, "escalated")
		}
	}
	if len(said) == 0 {
		return t
	}
	if changed.State == schedapi.StateClaimed {
		return t // held: a Put is refused; the next wake tries again
	}
	out, err := todos.Put(ctx, schedapi.PutInput{Todo: changed})
	if err != nil {
		slog.Warn("inbox: a human task's reminder or escalation could not be recorded; the next wake tries again", "todo", t.ID, "err", err)
		return t
	}
	if bus, err := application.UseService[eventbus.EventBus](ctx); err == nil && bus != nil {
		for _, what := range said {
			_ = bus.Publish(engine.HumanTaskDueTopic(t.ID, what), t.ID)
		}
	}
	return out.Todo
}

// raised is a priority one step more urgent (P0 stays P0).
func raised(p string) string {
	switch p {
	case "P3":
		return "P2"
	case "P2", "":
		return "P1"
	default:
		return "P0"
	}
}
