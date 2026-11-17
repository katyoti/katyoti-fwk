// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE WAITS — what a module awaits from another, a file per wait in its
// planning space (`.ktw/waiting/`), written under the module's lock like its
// to-dos. A wait ends by being removed: what lasts is on the request.

// Waits implements api.WaitService over the store port: the store the
// application chose (WaitsIn, OpenSchedulerStore).
type Waits struct{ store Store }

var _ api.WaitService = (*Waits)(nil)

// clock is the time a deadline is read against.
var clock = time.Now

// SetClock sets the time deadlines are read against — a test's day later —
// and answers what puts it back.
func SetClock(f func() time.Time) (restore func()) {
	old := clock
	clock = f
	return func() { clock = old }
}

// moduleOfWait is the module a wait id names.
func moduleOfWait(id string) (string, error) {
	m, _, ok := strings.Cut(id, ".WT-")
	if !ok || m == "" {
		return "", fmt.Errorf("%w: %q is not a wait id (<module>.WT-…)", cwmapi.ErrInvalidInput, id)
	}
	return m, nil
}

// Add records what a module awaits — once per thing awaited.
func (w *Waits) Add(ctx context.Context, in api.AddWaitInput) (api.Wait, error) {
	if err := checkActor(in.By); err != nil {
		return api.Wait{}, err
	}
	if in.Module == "" || in.On == "" || in.From == "" || strings.TrimSpace(in.What) == "" {
		return api.Wait{}, invalid("waits.Add: a wait has a module, what it waits on, the module asked and what is awaited")
	}
	if in.Days < 0 {
		return api.Wait{}, invalid("waits.Add: days is positive, got %d", in.Days)
	}
	space, err := planningSpace(ctx, in.Module)
	if err != nil {
		return api.Wait{}, err
	}
	st, err := settings(space)
	if err != nil {
		return api.Wait{}, err
	}
	days := in.Days
	if days == 0 {
		days = st.WaitDays
	}
	return w.store.AddWait(ctx, in.Module, in.On, func(id string) api.Wait {
		at := time.Now().UTC()
		return api.Wait{ID: id, Module: in.Module, On: in.On, From: in.From, What: strings.TrimSpace(in.What),
			Since: at.Format(time.RFC3339Nano), Deadline: at.AddDate(0, 0, days).Format(time.RFC3339Nano), By: in.By}
	})
}

// List answers the waits, oldest first, each flagged overdue past its
// deadline, each naming its module by its reference - qualified by its
// project when several are served, so two projects' waits stay apart
func (w *Waits) List(ctx context.Context, in api.ListWaitsInput) (api.WaitsOutput, error) {
	out := api.WaitsOutput{Waits: []api.Wait{}}
	if err := oneModule(ctx, in.Module); err != nil {
		return out, err
	}
	mods, err := modules(ctx)
	if err != nil {
		return out, err
	}
	now := clock().UTC()
	for _, m := range mods {
		if in.Module != "" && !sameModule(m, in.Module) {
			continue
		}
		all, err := w.store.ListWaits(ctx, m)
		if err != nil {
			return out, err
		}
		for _, x := range all {
			x.Module = m
			if d, err := time.Parse(time.RFC3339Nano, x.Deadline); err == nil && now.After(d) {
				x.Overdue = true
			}
			if (in.On != "" && x.On != in.On) || (in.Overdue && !x.Overdue) {
				continue
			}
			out.Waits = append(out.Waits, x)
		}
	}
	sort.Slice(out.Waits, func(i, j int) bool { return out.Waits[i].Since < out.Waits[j].Since })
	return out, nil
}

// Clear ends a wait, or every wait on what was answered.
func (w *Waits) Clear(ctx context.Context, in api.ClearWaitInput) (api.ClearWaitOutput, error) {
	var out api.ClearWaitOutput
	var ids []string
	switch {
	case in.ID != "":
		ids = []string{in.ID}
	case in.On != "":
		got, err := w.List(ctx, api.ListWaitsInput{On: in.On})
		if err != nil {
			return out, err
		}
		for _, x := range got.Waits {
			ids = append(ids, x.ID)
		}
	default:
		return out, invalid("waits.Clear: name the wait (id) or what was answered (on)")
	}
	for _, id := range ids {
		err := w.store.RemoveWait(ctx, id)
		if errors.Is(err, cwmapi.ErrNotFound) {
			if in.ID != "" {
				return out, fmt.Errorf("%w: wait %s", cwmapi.ErrNotFound, id)
			}
			continue
		}
		if err != nil {
			return out, err
		}
		out.Cleared++
	}
	return out, nil
}

// remind flags, once, each wait past its deadline: the module chases what it
// awaits or gives up (Clear).
func (w *Waits) remind(ctx context.Context, rep *api.TickReport) {
	got, err := w.List(ctx, api.ListWaitsInput{Overdue: true})
	if err != nil {
		rep.Errors = append(rep.Errors, err.Error())
		return
	}
	for _, x := range got.Waits {
		if x.Reminded != "" {
			continue
		}
		x.Reminded, x.Overdue = now(), false
		if err := w.store.PutWait(ctx, x); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("remind %s: %v", x.ID, err))
			continue
		}
		slog.Warn("scheduler: a module waits past its deadline", "module", x.Module, "on", x.On, "from", x.From, "deadline", x.Deadline)
		rep.Reminded = append(rep.Reminded, fmt.Sprintf("%s: %s awaits %s from %s since %s", x.ID, x.Module, x.On, x.From, x.Since))
	}
}
