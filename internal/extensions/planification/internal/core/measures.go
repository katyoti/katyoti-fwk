// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// PLANIFICATION'S RESILIENCE MEASURED - `planning check
// --measure` adds to the check the study's « Measure » over its scope and a
// window: computed from stored timestamps only (the incidents' list, the
// to-dos done, the structured waits the check read), never reported by an
// agent. What no store records yet is named in Unmeasured with why.

// Measures no store records yet, and why.
const (
	unmeasuredHubStarts = "no store records the hub's starts yet: each hub's starts a day is a feature of its own under the measures feature, waiting on the hub's record of its starts"
	unmeasuredSpend     = "no step records its spend yet: each step's spend in tokens and dollars is a feature of its own under the measures feature"
)

// measures computes the measures of the scope the check read, at r.now.
func (r *holdReader) measures(ctx context.Context, in api.PlanningCheckInput, out api.PlanningCheckOutput) (api.PlanningMeasures, error) {
	var incidents []api.IncidentRecord
	if path := incidentsPath(ctx, r.cwm); path != "" {
		doc, err := readIncidents(path)
		if err != nil {
			return api.PlanningMeasures{}, err
		}
		incidents = doc.Incidents
	}
	s, err := sched(ctx)
	if err != nil {
		return api.PlanningMeasures{}, err
	}
	done, err := s.List(ctx, schedapi.ListInput{Done: true})
	if err != nil {
		return api.PlanningMeasures{}, err
	}
	inScope := func(work string, labels []string) bool {
		return r.scope == "" || r.works[work] || slices.Contains(labels, r.scope)
	}
	m := measure(r.now, in.Days, inScope, incidents, done.Todos, out.Blockers)
	if id := in.Increment + in.Release; id != "" {
		m.CapUSD = r.capOf(ctx, id)
	}
	return m, nil
}

// capOf is the spend's cap of an increment or a release: its own budget,
// else (an increment) its release's.
func (r *holdReader) capOf(ctx context.Context, id string) float64 {
	o, err := loadRelease(ctx, r.cwm, id)
	if err != nil {
		return 0
	}
	if b := releaseFields(o).BudgetUSD; b > 0 || o.Type != api.IncrementType {
		return b
	}
	if rel := releaseOfIncrement(ctx, r.cwm, id); rel != "" {
		if ro, err := loadRelease(ctx, r.cwm, rel); err == nil {
			return releaseFields(ro).BudgetUSD
		}
	}
	return 0
}

// measure is the measures at now over days back (DefaultMeasureDays when
// unset): the incidents raised and closed on the scope's work, the person's
// steps done, and what the check found waiting now. Pure.
func measure(now time.Time, days int, inScope func(work string, labels []string) bool,
	incidents []api.IncidentRecord, done []schedapi.Todo, blockers []api.Blocker) api.PlanningMeasures {
	if days <= 0 {
		days = api.DefaultMeasureDays
	}
	now = now.UTC()
	from := now.Add(-time.Duration(days) * 24 * time.Hour)
	m := api.PlanningMeasures{From: from.Format(time.RFC3339), To: now.Format(time.RFC3339), Days: days,
		IncidentsPerDay: map[string]int{},
		Unmeasured:      map[string]string{"hub_starts": unmeasuredHubStarts, "spend": unmeasuredSpend}}
	inWindow := func(t time.Time) bool { return !t.Before(from) && !t.After(now) }

	var system []time.Duration
	for _, inc := range incidents {
		if !inScope(inc.Work, nil) {
			continue
		}
		since, ok := parseAt(inc.Since)
		if !ok {
			continue
		}
		if inc.Closed == "" {
			m.StallsOpen++
		}
		if inWindow(since) {
			m.Stalls++
			m.IncidentsPerDay[since.Format(time.DateOnly)]++
		}
		if closed, ok := parseAt(inc.Closed); ok && inWindow(closed) && !closed.Before(since) {
			system = append(system, closed.Sub(since))
		}
	}
	m.UnblockSystem = durations(system)

	ended := map[string]time.Time{}
	for _, t := range done {
		if t.Result != nil {
			if at, ok := parseAt(t.Result.At); ok {
				ended[t.ID] = at
			}
		}
	}
	var person []time.Duration
	for _, t := range done {
		at, ok := ended[t.ID]
		if t.For != schedapi.ForPerson || !ok || !inWindow(at) || !inScope(t.Request, t.Labels) {
			continue
		}
		// It could be acted on once it was made and what it came after ended.
		ready, ok := parseAt(t.Made)
		if !ok {
			continue
		}
		for _, a := range t.After {
			if e, ok := ended[a]; ok && e.After(ready) {
				ready = e
			}
		}
		if !at.Before(ready) {
			person = append(person, at.Sub(ready))
		}
	}
	m.UnblockPerson = durations(person)

	var waiting, unclassified []time.Duration
	for _, b := range blockers {
		since, ok := parseAt(b.Since)
		age := time.Duration(0)
		if ok && now.After(since) {
			age = now.Sub(since)
		}
		switch {
		case b.Kind == api.BlockerIncident && strings.HasPrefix(b.Reason, unclassifiedWait):
			unclassified = append(unclassified, age)
			if age > 24*time.Hour {
				m.UnclassifiedOver1++
			}
		case b.ActorKind == api.ActorPerson && (b.Kind == api.BlockerAnswer || b.Kind == api.BlockerAct):
			waiting = append(waiting, age)
		}
	}
	m.PersonWaiting = durations(waiting)
	u := durations(unclassified)
	m.Unclassified, m.UnclassifiedMaxS = u.Count, u.MaxS
	if len(m.IncidentsPerDay) == 0 {
		m.IncidentsPerDay = nil
	}
	return m
}

// durations sums up a set of waits: how many, the median and the longest.
func durations(ds []time.Duration) api.Durations {
	if len(ds) == 0 {
		return api.Durations{}
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	med := ds[len(ds)/2]
	if len(ds)%2 == 0 {
		med = (ds[len(ds)/2-1] + ds[len(ds)/2]) / 2
	}
	return api.Durations{Count: len(ds), MedianS: int64(med / time.Second), MaxS: int64(ds[len(ds)-1] / time.Second)}
}

// parseAt reads a stored timestamp (RFC 3339, with or without fractions).
func parseAt(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}
