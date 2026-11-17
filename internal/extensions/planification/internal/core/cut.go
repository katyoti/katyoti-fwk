// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A STREAM RELEASES BY ITS MODE — a cut makes the stream's next release: the
// increments naming the stream, done, in no release yet, shipped in a release
// numbered by the scheme's next version, its level computed (major when one
// breaks), frozen; nothing delivered, nothing made. A periodic stream is cut
// by its timer (`timed`, every its period); an on-change stream when one of
// its increments is delivered - at most once per min_interval, its timer
// cutting what waited. Releasing stays behind ready-to-release (a person
// confirms the change log); the release event is what publications follow.

// cutTimer is the name of a stream's timer.
const cutTimer = "stream-cut"

// period turns "14d" into a duration (api.Period).
func period(s string) (time.Duration, bool) { return api.Period(s) }

// withCutTimer gives a periodic or on-change stream its timer - every its
// period, or its minimum interval (a day when none) - and takes it from a
// manual one; every stream gets its daily release-due timer.
func withCutTimer(o cwmapi.Object, f api.StreamFields) (cwmapi.Object, error) {
	every := ""
	switch f.ReleaseMode {
	case api.ModePeriodic:
		every = f.Every
	case api.ModeOnChange:
		every = f.MinInterval
		if every == "" {
			every = "1d"
		}
	}
	var timed eventsapi.TimedFields
	if t, ok, _ := cwmapi.DecodeCapability[eventsapi.TimedFields](o, eventsapi.CapTimed); ok {
		timed = t
	}
	var timers []eventsapi.Timer
	for _, t := range timed.Timers {
		if t.Name != cutTimer && t.Name != dueTimer {
			timers = append(timers, t)
		}
	}
	if d, ok := period(every); ok {
		timers = append(timers, eventsapi.Timer{Name: cutTimer, Every: fmt.Sprintf("%dh", int(d.Hours()))})
	}
	// Every stream, whatever its mode, looks once a day for a release past
	// its date with an increment not done.
	timers = append(timers, eventsapi.Timer{Name: dueTimer, At: "06:00"})
	if len(timers) == 0 {
		if _, ok := o.Capabilities[eventsapi.CapTimed]; ok {
			delete(o.Capabilities, eventsapi.CapTimed)
		}
		return o, nil
	}
	err := o.SetCapability(eventsapi.CapTimed, eventsapi.TimedFields{Timers: timers})
	return o, err
}

// inStream lists what is in a stream, of a type.
func inStream(ctx context.Context, cwm cwmapi.CWMService, stream, typ string) ([]cwmapi.Object, error) {
	ids, err := incoming(ctx, cwm, stream, api.EdgeInStream)
	if err != nil {
		return nil, err
	}
	var out []cwmapi.Object
	for _, id := range ids {
		if o, _, err := loadAny(ctx, cwm, id); err == nil && o.Type == typ {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// numbers reads a semver-like version: its numbers and the width of each.
func numbers(v string) ([]int, []int) {
	v, _, _ = strings.Cut(v, "-")
	var ns, ws []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, nil
		}
		ns, ws = append(ns, n), append(ws, len(p))
	}
	return ns, ws
}

// nextVersion is a stream's next release version by its scheme, from the
// versions its releases took and the day; "" when the scheme has none left
// today (a date already used).
func nextVersion(scheme string, taken []string, breaking bool, now time.Time) string {
	has := func(v string) bool {
		for _, t := range taken {
			if t == v {
				return true
			}
		}
		return false
	}
	switch scheme {
	case api.SchemeCalendar:
		month := now.Format("2006.01")
		if !has(month) {
			return month
		}
		for n := 1; ; n++ {
			if v := fmt.Sprintf("%s-rc%d", month, n); !has(v) {
				return v
			}
		}
	case api.SchemeDate:
		if day := now.Format("2006-01-02"); !has(day) {
			return day
		}
		return ""
	}
	// semver: the highest taken, its minor bumped (its width kept: 1.005 →
	// 1.006), or its major when an increment breaks.
	var best, width []int
	for _, t := range taken {
		ns, ws := numbers(t)
		if len(ns) < 2 {
			continue
		}
		if best == nil || compareNumbers(ns, best) > 0 {
			best, width = ns, ws
		}
	}
	if best == nil {
		return "1.0.0"
	}
	next := append([]int{}, best...)
	if breaking {
		next[0]++
		for i := 1; i < len(next); i++ {
			next[i] = 0
		}
	} else {
		next[1]++
		for i := 2; i < len(next); i++ {
			next[i] = 0
		}
	}
	parts := make([]string, len(next))
	for i, n := range next {
		parts[i] = fmt.Sprintf("%0*d", width[i], n)
	}
	return strings.Join(parts, ".")
}

func compareNumbers(a, b []int) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] - b[i]
		}
	}
	return len(a) - len(b)
}

// Cut makes a stream's next release from its delivered increments.
func (s *Streams) Cut(ctx context.Context, in api.CutInput) (api.CutOutput, error) {
	out := api.CutOutput{Increments: []string{}}
	// The daily timer moves what is past its date; it never cuts.
	if in.Timer == dueTimer {
		moved, err := s.CarryOver(ctx, in)
		out.Increments, out.Note = moved.Moved, moved.Note
		return out, err
	}
	by := in.By
	if by.ID == "" {
		by = PlanningActor
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return out, err
	}
	so, err := loadStream(ctx, cwm, in.ID)
	if err != nil {
		return out, err
	}
	sf := streamFields(so)
	breaking := false
	for _, inc := range ridingAlong(ctx, cwm, so.ID) {
		out.Increments = append(out.Increments, inc.ID)
		breaking = breaking || releaseFields(inc).Breaking
	}
	if len(out.Increments) == 0 {
		out.Note = "no increment delivered since the last release: nothing made"
		return out, nil
	}
	if why, err := noRoomInStream(ctx, cwm, so.ID); err != nil {
		return out, err
	} else if why != "" {
		// Not an error: the cut makes it once one of the two is released.
		out.Increments = []string{}
		out.Note = "nothing made: " + why
		return out, nil
	}
	rels, err := inStream(ctx, cwm, so.ID, api.ReleaseType)
	if err != nil {
		return out, err
	}
	var taken []string
	for _, r := range rels {
		taken = append(taken, releaseVersion(r))
	}
	version := nextVersion(sf.VersionScheme, taken, breaking, time.Now())
	if version == "" {
		out.Note = "a release of this stream already took today's date: the next cut makes it"
		return out, nil
	}
	level := api.LevelMinor
	if breaking {
		level = api.LevelMajor
	}
	module, _ := cwmapi.ModuleOfObjectID(so.ID)
	rel := NewReleases()
	made, err := rel.Plan(ctx, api.PlanInput{Module: module, Version: version, Level: level, Stream: so.ID})
	if err != nil {
		return out, fmt.Errorf("streams.Cut %s: %w", so.ID, err)
	}
	for _, id := range out.Increments {
		if _, err := shipIn(ctx, api.ShipInInput{ID: id, In: made.Release.ID}, false); err != nil {
			return out, fmt.Errorf("streams.Cut %s: ship %s: %w", so.ID, id, err)
		}
	}
	if frozen, err := rel.Freeze(ctx, api.MoveInput{ID: made.Release.ID, By: by, Note: "cut by its stream's " + sf.ReleaseMode + " mode"}); err != nil {
		out.Note = "planned, not frozen: " + err.Error()
	} else {
		made = frozen
	}
	sf.LastCut = time.Now().UTC().Format(time.RFC3339)
	if so, err = withStreamFields(so, sf); err == nil {
		_, err = cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: so})
	}
	if err != nil {
		return out, err
	}
	r := made.Release
	out.Release = &r
	return out, nil
}

// nextPlannedRelease is a stream's next release (api.NextPlanned) read from
// its release objects.
func nextPlannedRelease(ctx context.Context, cwm cwmapi.CWMService, stream string) (string, bool) {
	rels, err := inStream(ctx, cwm, stream, api.ReleaseType)
	if err != nil {
		return "", false
	}
	list := make([]api.Release, 0, len(rels))
	for _, r := range rels {
		list = append(list, toRelease(r))
	}
	next, ok := api.NextPlanned(list)
	return next.ID, ok
}

// ridingAlong is a stream's increments done and in no live release: they ride
// along in its next release, not assigned, until a freeze or a cut takes them.
func ridingAlong(ctx context.Context, cwm cwmapi.CWMService, stream string) []cwmapi.Object {
	incs, err := inStream(ctx, cwm, stream, api.IncrementType)
	if err != nil {
		return nil
	}
	var out []cwmapi.Object
	for _, inc := range incs {
		if releaseState(inc) != api.IncrementDone {
			continue
		}
		if r := edgeTo(inc, api.EdgeShipsIn); r != "" {
			if ro, _, err := loadAny(ctx, cwm, r); err == nil && releaseState(ro) != api.ReleaseDropped {
				continue
			}
		}
		out = append(out, inc)
	}
	return out
}

// cutOnChange cuts the stream a delivered increment names, when it releases
// on change and its minimum interval has passed; otherwise its timer will.
func cutOnChange(ctx context.Context, cwm cwmapi.CWMService, inc cwmapi.Object) {
	sm := edgeTo(inc, api.EdgeInStream)
	if sm == "" {
		return
	}
	so, err := loadStream(ctx, cwm, sm)
	if err != nil {
		return
	}
	sf := streamFields(so)
	if sf.ReleaseMode != api.ModeOnChange {
		return
	}
	if last, err := time.Parse(time.RFC3339, sf.LastCut); err == nil {
		gap, ok := period(sf.MinInterval)
		if !ok {
			gap = 24 * time.Hour
		}
		if time.Since(last) < gap {
			return // within its minimum interval: its timer cuts what waits
		}
	}
	if _, err := NewStreams().Cut(ctx, api.CutInput{ID: sm}); err != nil {
		slog.Warn("planification: a delivered increment's stream did not cut its release", "increment", inc.ID, "stream", sm, "error", err)
	}
}
