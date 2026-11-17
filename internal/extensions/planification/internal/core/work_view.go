// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE WORK VIEW — a module's to-dos by state: what workers have pulled, what is
// ready (in the order the scheduler hands it out), what waits and on what, what
// failed or is blocked; the archived ones counted; each request with where it
// stands (its rollup).

// Work renders the Work view of a module, or of every module.
func (v *Views) Work(ctx context.Context, in api.WorkInput) (api.DocumentsOutput, error) {
	all, err := todosOf(ctx, "")
	if err != nil {
		return api.DocumentsOutput{}, err
	}
	s, err := sched(ctx)
	if err != nil {
		return api.DocumentsOutput{}, err
	}
	lang := in.Lang
	var todos []api.WorkTodo
	for _, t := range all {
		if in.Module == "" || t.Module == in.Module {
			todos = append(todos, t)
		}
	}
	q, err := s.Queue(ctx, schedapi.Filter{Module: in.Module})
	if err != nil {
		return api.DocumentsOutput{}, err
	}
	rank := map[string]int{}
	for i, e := range q.Queue {
		rank[e.Todo.ID] = i
	}
	by := map[string][]api.WorkTodo{}
	for _, t := range todos {
		switch {
		case stopped(t):
			by["failed"] = append(by["failed"], t)
		case t.State == schedapi.StateClaimed || t.State == schedapi.StateReported:
			by["running"] = append(by["running"], t)
		default:
			by[t.State] = append(by[t.State], t)
		}
	}
	sort.SliceStable(by["ready"], func(i, j int) bool { return rank[by["ready"][i].ID] < rank[by["ready"][j].ID] })
	done, err := s.List(ctx, schedapi.ListInput{Module: in.Module, Done: true})
	if err != nil {
		return api.DocumentsOutput{}, err
	}

	scope := in.Module
	if scope == "" {
		scope = label(lang, "td_all")
	}
	title := label(lang, "work") + " — " + scope
	var b strings.Builder
	counts := map[string]int{"running": len(by["running"]), "ready": len(by["ready"]), "waiting": len(by["waiting"]), "failed": len(by["failed"]), "done": len(done.Todos)}
	head, rule, row := "|", "|", "|"
	for _, k := range []string{"running", "ready", "waiting", "failed", "done"} {
		head += " " + label(lang, "td_"+k) + " |"
		rule += "---|"
		row += fmt.Sprintf(" %d |", counts[k])
	}
	b.WriteString(head + "\n" + rule + "\n" + row + "\n\n")
	var requests []string
	for _, t := range todos {
		if !slices.Contains(requests, t.Request) {
			requests = append(requests, t.Request)
		}
	}
	if len(requests) > 0 {
		fmt.Fprintf(&b, "## %s\n\n| %s | %s | %s |\n|---|---|---|\n", label(lang, "td_requests"), label(lang, "td_request"), label(lang, "td_stands"), label(lang, "td_ended"))
		for _, r := range requests {
			ru, err := NewWork().Rollup(ctx, api.RollupInput{Request: r})
			if err != nil {
				return api.DocumentsOutput{}, err
			}
			stands := ru.State
			if ru.Note != "" {
				stands += " — " + ru.Note
			}
			fmt.Fprintf(&b, "| `%s` | %s | %d/%d |\n", r, stands, ru.Done, ru.Total)
		}
		b.WriteString("\n")
	}
	for _, k := range []string{"running", "ready", "waiting", "failed"} {
		heading := label(lang, "td_"+k)
		if k == "ready" {
			heading = label(lang, "td_ready_order")
		}
		fmt.Fprintf(&b, "## %s\n\n", heading)
		if len(by[k]) == 0 {
			fmt.Fprintf(&b, "%s\n\n", label(lang, "none"))
			continue
		}
		cols := []string{label(lang, "td_todo"), label(lang, "td_step"), label(lang, "td_request"), label(lang, "td_who"), label(lang, "td_priority")}
		if k == "waiting" || k == "failed" {
			cols = append(cols, label(lang, "waits"))
		}
		fmt.Fprintf(&b, "| %s |\n|%s\n", strings.Join(cols, " | "), strings.Repeat("---|", len(cols)))
		for _, t := range by[k] {
			cells := []string{"`" + t.ID + "`", t.Packet.Step, "`" + t.Request + "`", who(t), dash(t.Priority)}
			if k == "waiting" || k == "failed" {
				cells = append(cells, dash(t.Waits))
			}
			fmt.Fprintf(&b, "| %s |\n", strings.Join(cells, " | "))
		}
		b.WriteString("\n")
	}
	key := "work"
	if in.Module != "" {
		key += "/" + in.Module
	}
	return api.DocumentsOutput{Documents: []render.Document{page(key, "", title, b.String())}}, nil
}

// who is who does a to-do: its task (or persona), or a person and what is asked.
func who(t api.WorkTodo) string {
	if t.Packet.Doer == api.DoerPerson {
		return "person (" + t.Packet.Interaction + ")"
	}
	return dash(t.Packet.Task)
}
