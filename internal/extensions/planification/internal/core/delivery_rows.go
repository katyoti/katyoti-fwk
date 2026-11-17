// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE DELIVERY ROWS - the sources answering rows of
// the delivery data types (api/delivery_rows.go): a stream's lane, a
// release's level, an increment's detail. They compute nothing of their own:
// where each thing stands is the standing function's, a stream's next cut
// NextCut's, a release's gates gatesOf's, what an increment holds and its
// requests the increment summary's.

// Lanes answers every stream's lane, or one's.
func (s *Streams) Lanes(ctx context.Context, in api.LanesInput) (api.LanesOutput, error) {
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.LanesOutput{}, err
	}
	var streams []cwmapi.Object
	if in.ID != "" {
		so, err := loadStream(ctx, cwm, in.ID)
		if err != nil {
			return api.LanesOutput{}, err
		}
		streams = []cwmapi.Object{so}
	} else {
		got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: api.StreamType})
		if err != nil {
			return api.LanesOutput{}, err
		}
		for _, o := range got.Objects {
			if so, err := loadStream(ctx, cwm, o.ID); err == nil {
				streams = append(streams, so)
			}
		}
		sort.SliceStable(streams, func(i, j int) bool {
			a, b := streamFields(streams[i]), streamFields(streams[j])
			if a.Order != b.Order {
				return a.Order < b.Order
			}
			return streams[i].ID < streams[j].ID
		})
	}
	backlog, err := NewIncrements().Backlog(ctx, api.IncrementBacklogInput{Bare: true}) // its lanes show no summary
	if err != nil {
		return api.LanesOutput{}, err
	}
	out := api.LanesOutput{Lanes: []api.StreamLane{}}
	for _, so := range streams {
		out.Lanes = append(out.Lanes, laneOf(ctx, cwm, so, backlog, in.Language))
	}
	return out, nil
}

// laneOf is one stream's lane: its last released release then those still to
// ship, by date, and its open increments in no release; its words in lang
func laneOf(ctx context.Context, cwm cwmapi.CWMService, so cwmapi.Object, backlog api.IncrementBacklogOutput, lang string) api.StreamLane {
	st := toStream(so)
	mode := st.ReleaseMode
	if mode == "" {
		mode = api.ModeManual
	}
	lane := api.StreamLane{ID: st.ID, Name: st.Name, Color: st.Color, Scheme: st.VersionScheme, Mode: mode, ModeText: st.Cut.Text, NextCut: st.Cut.At,
		SchemeMode: schemeMode(lang, st, mode), NextText: nextText(lang, st.Cut, mode), Releases: []api.ReleaseCard{}, Backlog: []api.BacklogItem{}}
	next, _ := nextPlannedRelease(ctx, cwm, st.ID)
	rels, _ := inStream(ctx, cwm, st.ID, api.ReleaseType)
	sort.SliceStable(rels, func(i, j int) bool { return releaseBefore(rels[i], rels[j]) })
	var last *cwmapi.Object
	var open []api.ReleaseCard
	for i := range rels {
		switch releaseState(rels[i]) {
		case api.ReleasePlanned, api.ReleaseFrozen:
			open = append(open, cardOf(ctx, cwm, rels[i], next, lang))
		case api.ReleaseReleased:
			last = &rels[i]
		}
	}
	if last != nil {
		lane.Releases = append(lane.Releases, cardOf(ctx, cwm, *last, next, lang))
	}
	lane.Releases = append(lane.Releases, open...)
	for _, it := range backlog.Items {
		if it.Increment.Stream != st.ID {
			continue
		}
		inc := it.Increment
		lane.Backlog = append(lane.Backlog, api.BacklogItem{ID: inc.ID, Title: inc.Title, Priority: incrementRowPriority(inc), Held: inc.Held != nil})
	}
	return lane
}

// cardOf is one release's card; next is its stream's next release.
func cardOf(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object, next, lang string) api.ReleaseCard {
	return cardWith(ctx, cwm, rel, next, nil, lang)
}

// cardWith is cardOf with its gates already read (nil: read here).
func cardWith(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object, next string, gates *api.GatesOutput, lang string) api.ReleaseCard {
	r := toRelease(rel)
	c := api.ReleaseCard{ID: rel.ID, Label: api.ReleaseLabel(r.Version, titleOf(rel)), Version: r.Version, Date: r.Date,
		Stage: api.ReleaseStage(r), Next: rel.ID == next, Stream: edgeTo(rel, api.EdgeInStream)}
	if c.Date == "" {
		c.Date = undatedRelease(ctx, cwm, rel, c.Stream)
	}
	// Days left count to its planned date - never to where an undated one
	// is placed.
	if r.State != api.ReleaseReleased && r.State != api.ReleaseDropped {
		if n, ok := calendarDaysLeft(r.Date, time.Now()); ok {
			c.DaysLeft = &n
		}
	}
	if n := len(r.Candidates); n > 0 {
		c.Candidate, c.Build = r.Candidates[n-1].Name, r.Candidates[n-1].Build
	}
	if sc, err := scopeOf(ctx, cwm, rel); err == nil {
		c.Done, c.Total = sc.Done, sc.Total
	}
	if planned, err := plannedFor(ctx, cwm, rel.ID); err == nil {
		c.Planned = len(planned)
	}
	c.Summary = cardSummary(lang, c)
	if c.Planned > 0 {
		c.PlannedText = deliveryCountf(lang, c.Planned, "card_planned")
	}
	if c.Candidate != "" {
		c.CandidateText = c.Candidate
		if c.Build != "" {
			c.CandidateText += " · build " + c.Build
		}
	}
	if r.State != api.ReleaseReleased && r.State != api.ReleaseDropped {
		if gates == nil {
			if g, err := gatesOf(ctx, cwm, rel, ""); err == nil {
				gates = &g
			}
		}
		if g := gates; g != nil {
			for _, l := range g.Gates {
				d := api.GateDot{Name: l.Name, Why: l.Why}
				// The candidate's own gates - and the change log, written once
				// it is promoted - are not judged before it exists.
				if g.Candidate != "" || (l.Name != api.GateEndToEnd && l.Name != api.GateSignOff && l.Name != api.GateChangeLog) {
					passed := l.Passed
					d.Passed = &passed
				} else {
					d.Why = "needs a candidate"
				}
				c.Gates = append(c.Gates, d)
			}
		}
	}
	return c
}

// undatedRelease places a release with no planned date on the timeline: a
// cut makes its release undated, so the stream's latest undated frozen or
// released one sits at its last cut; one planned, at its next cut. Empty
// otherwise - display draws it at today, dashed.
func undatedRelease(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object, stream string) string {
	if stream == "" {
		return ""
	}
	so, err := loadStream(ctx, cwm, stream)
	if err != nil {
		return ""
	}
	st := toStream(so)
	switch releaseState(rel) {
	case api.ReleaseFrozen, api.ReleaseReleased:
		if len(st.LastCut) < 10 {
			return ""
		}
		rels, _ := inStream(ctx, cwm, stream, api.ReleaseType)
		latest := ""
		for _, o := range rels {
			if st := releaseState(o); (st == api.ReleaseFrozen || st == api.ReleaseReleased) && releaseFields(o).Date == "" && laterID(o.ID, latest) {
				latest = o.ID
			}
		}
		if latest == rel.ID {
			return st.LastCut[:10]
		}
	case api.ReleasePlanned:
		return st.Cut.At
	}
	return ""
}

// laterID is whether a module-qualified id (RL-12) was made after another
// (RL-9): the higher number; any id is later than none.
func laterID(a, b string) bool {
	if b == "" {
		return true
	}
	na, nb := idNumber(a), idNumber(b)
	if na != nb {
		return na > nb
	}
	return a > b
}

// releaseBefore orders a stream's releases on their own dates - never on the
// cut an undated one is placed at: the dated first, by date, then by id
// number; the undated last, by id number.
func releaseBefore(a, b cwmapi.Object) bool {
	if c := releaseDateCmp(releaseFields(a).Date, releaseFields(b).Date); c != 0 {
		return c < 0
	}
	return idBefore(a.ID, b.ID)
}

// releaseDateCmp compares two releases' own dates (YYYY-MM-DD): the dated
// first, by date, the undated last; 0 for the same date or both undated.
func releaseDateCmp(da, db string) int {
	switch {
	case (da == "") != (db == ""):
		if da != "" {
			return -1
		}
		return 1
	case da < db:
		return -1
	case da > db:
		return 1
	}
	return 0
}

// idBefore orders two ids by their number, then as text.
func idBefore(a, b string) bool {
	if na, nb := idNumber(a), idNumber(b); na != nb {
		return na < nb
	}
	return a < b
}

// idNumber is an id's number (12 of); 0 when it has none.
func idNumber(id string) int {
	i := strings.LastIndex(id, "-")
	if i < 0 {
		return 0
	}
	n, _ := strconv.Atoi(id[i+1:])
	return n
}

// calendarDaysLeft is the number of calendar days from now's date to date
// (YYYY-MM-DD): 1 the day before, 0 on the day, negative once late.
func calendarDaysLeft(date string, now time.Time) (int, bool) {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0, false
	}
	today, _ := time.Parse("2006-01-02", now.Format("2006-01-02"))
	return int(d.Sub(today).Hours() / 24), true
}

// schemeMode is a stream's scheme and mode as its chip: « semver · periodic · 14d ».
func schemeMode(lang string, st api.Stream, mode string) string {
	parts := []string{}
	if st.VersionScheme != "" {
		parts = append(parts, st.VersionScheme)
	}
	if w := deliveryWord(lang, "stream_"+mode); w != "" {
		parts = append(parts, w)
	} else {
		parts = append(parts, mode)
	}
	switch mode {
	case api.ModePeriodic:
		if st.Every != "" {
			parts = append(parts, st.Every)
		}
	case api.ModeOnChange:
		every := st.MinInterval
		if every == "" {
			every = "1d"
		}
		parts = append(parts, fmt.Sprintf(deliveryWord(lang, "at_most_every"), every))
	}
	return strings.Join(parts, " · ")
}

// nextText is a stream's next cut in a few words.
func nextText(lang string, cut api.StreamCut, mode string) string {
	switch {
	case cut.At != "":
		return fmt.Sprintf(deliveryWord(lang, "next_cut"), cut.At)
	case mode == api.ModeOnChange:
		return deliveryWord(lang, "next_on_change")
	case mode == api.ModePeriodic:
		return deliveryWord(lang, "next_no_period")
	}
	return deliveryWord(lang, "next_by_hand")
}

// cardSummary is a release card's line: its date, what it ships done of the
// total, what is planned for it.
func cardSummary(lang string, c api.ReleaseCard) string {
	var parts []string
	if c.Date != "" {
		parts = append(parts, c.Date)
	}
	parts = append(parts, fmt.Sprintf(deliveryWord(lang, "card_done"), c.Done, c.Total))
	return strings.Join(parts, " · ")
}

// incrementRowPriority is an increment's priority, the default when it has none.
func incrementRowPriority(inc api.Increment) string {
	if inc.Priority == "" {
		return api.DefaultPriority
	}
	return inc.Priority
}

// Rows answers a release's level.
func (r *Releases) Rows(ctx context.Context, in api.GetInput) (api.ReleaseRows, error) {
	return r.rowsIn(ctx, in.ID, "")
}

// rowsIn is a release's level, its words in lang (deliveryWord).
func (r *Releases) rowsIn(ctx context.Context, id, lang string) (api.ReleaseRows, error) {
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReleaseRows{}, err
	}
	rel, err := loadRelease(ctx, cwm, id)
	if err != nil {
		return api.ReleaseRows{}, err
	}
	next, _ := nextPlannedRelease(ctx, cwm, edgeTo(rel, api.EdgeInStream))
	var gates *api.GatesOutput
	if g, err := gatesOf(ctx, cwm, rel, ""); err == nil {
		gates = &g
	}
	card := cardWith(ctx, cwm, rel, next, gates, lang)
	card.Gates = localGates(lang, card.Gates)
	out := api.ReleaseRows{Release: card, Gates: card.Gates, Stepper: stepperOf(card.Stage), Ships: []api.IncrementRow{}, Planned: []api.IncrementRow{}}
	stream := titleOfID(ctx, cwm, card.Stream)
	out.Eyebrow, out.Title = rel.ID, card.Label
	if card.Stream != "" {
		out.Eyebrow, out.Title = stream+" · "+rel.ID, stream+" "+card.Label
	}
	if card.Next {
		out.Eyebrow += deliveryWord(lang, "next_release")
	}
	if r := toRelease(rel); r.Date != "" && r.State != api.ReleaseReleased && r.State != api.ReleaseDropped {
		out.Due = fmt.Sprintf(deliveryWord(lang, "due"), r.Date)
		if card.DaysLeft != nil {
			switch d := *card.DaysLeft; {
			case d < 0:
				out.Due += deliveryCountf(lang, -d, "days_late")
			default:
				out.Due += deliveryCountf(lang, d, "days_left")
			}
		}
	}
	if gates != nil {
		out.NextAction = nextAction(releaseState(rel), *gates)
	}
	row := func(inc cwmapi.Object) api.IncrementRow {
		return rowOf(ctx, cwm, inc, lang)
	}
	for _, inc := range carriedIncrements(ctx, cwm, rel) {
		out.Ships = append(out.Ships, row(inc))
	}
	if planned, err := plannedFor(ctx, cwm, rel.ID); err == nil {
		for _, inc := range planned {
			out.Planned = append(out.Planned, row(inc))
		}
	}
	byPriority := func(rows []api.IncrementRow) {
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].Priority != rows[j].Priority {
				return rows[i].Priority < rows[j].Priority
			}
			return rows[i].ID < rows[j].ID
		})
	}
	byPriority(out.Ships)
	byPriority(out.Planned)
	out.ShipPlanned = card.Next && len(out.Planned) > 0
	out.NextStep, out.NextLabel = nextStep(lang, toRelease(rel), out.Ships)
	switch r := toRelease(rel); r.State {
	case api.ReleasePlanned:
		out.CanReschedule, out.CanDrop = true, true
	case api.ReleaseFrozen:
		out.CanReschedule, out.CanDrop, out.CanUnfreeze = true, true, len(r.Candidates) == 0
	}
	return out, nil
}

// nextStep is what a person does next on a release, by its stage, and its
// words: freeze it, deliver a candidate, write the candidate's letter, sign it
// off, promote it. None once released or dropped.
func nextStep(lang string, r api.Release, ships []api.IncrementRow) (string, string) {
	switch r.State {
	case api.ReleasePlanned:
		if open := openRows(ships); open > 0 {
			if deliveryLocal(lang) {
				return "freeze", deliveryCountf(lang, open, "finish")
			}
			return "freeze", fmt.Sprintf("Finish %d increment%s, then Freeze", open, plural(open))
		}
		return "freeze", deliveryWord(lang, "freeze")
	case api.ReleaseFrozen:
		if len(r.Candidates) == 0 {
			return "deliver", deliveryWord(lang, "deliver")
		}
		last := r.Candidates[len(r.Candidates)-1]
		switch {
		case last.Letter == "":
			return "letter", fmt.Sprintf(deliveryWord(lang, "letter"), last.Name)
		case last.SignedOff == nil:
			return "signoff", fmt.Sprintf(deliveryWord(lang, "signoff"), last.Name)
		}
		return "promote", fmt.Sprintf(deliveryWord(lang, "promote"), last.Name)
	}
	return "", ""
}

// stepperOf is a release's stage stepper: planned, frozen, candidate,
// released - those before its stage done, its stage current; a dropped one
// is dropped alone.
func stepperOf(stage string) []api.StepChip {
	if stage == api.ReleaseDropped {
		return []api.StepChip{{Name: api.ReleaseDropped, State: "current"}}
	}
	stages := []string{api.ReleasePlanned, api.ReleaseFrozen, api.StageCandidate, api.ReleaseReleased}
	at := -1
	for i, s := range stages {
		if s == stage {
			at = i
		}
	}
	out := make([]api.StepChip, 0, len(stages))
	for i, s := range stages {
		state := "todo"
		switch {
		case at < 0:
		case i < at:
			state = "done"
		case i == at:
			state = "current"
		}
		out = append(out, api.StepChip{Name: s, State: state})
	}
	return out
}

// rowOf is one increment's row: where it stands and its requests from its
// summary - read once - beside its hold and its assignment; its words in lang
// (deliveryWord, waitWord).
func rowOf(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, lang string) api.IncrementRow {
	inc := toIncrement(ctx, o)
	row := api.IncrementRow{ID: inc.ID, Title: inc.Title, Priority: incrementRowPriority(inc), ShipsIn: inc.ShipsIn, PlannedFor: inc.PlannedFor,
		Standing: api.IncrementStanding(api.IncrementStandingInput{Increment: inc}), Progress: []int{0, 0, 0, 0, 0},
		Description: inc.Description}
	if rel := inc.ShipsIn + inc.PlannedFor; rel != "" {
		if r, _, err := loadAny(ctx, cwm, rel); err == nil {
			row.Release = releaseFields(r).Version
		}
		if row.Release == "" {
			row.Release = rel
		}
	}
	row.Place = inc.ID
	if row.Release != "" {
		row.Place += " · " + row.Release
	}
	if inc.Held != nil {
		row.Held, row.HeldText = inc.Held.Reason, api.StandingOnHold
	}
	if inc.PlannedFor != "" {
		row.PlannedChip = "planned"
	}
	for _, o := range releaseOptions(ctx, cwm, inc) {
		if o.ID == "" || o.ID == inc.ShipsIn || o.ID == inc.PlannedFor {
			continue
		}
		mode := deliveryWord(lang, "mode_"+o.Mode)
		if mode == "" {
			mode = strings.ReplaceAll(o.Mode, "-", " ")
		}
		row.Options = append(row.Options, api.RowOption{Value: o.ID, Label: o.Label + " — " + mode})
	}
	if sum, err := NewIncrements().Summary(ctx, api.GetInput{ID: inc.ID}); err == nil {
		if sum.Standing != "" {
			row.Standing = sum.Standing
		}
		// Its fourth slot holds every open request no step of which runs nor
		// asks a person - to do, waiting on another, nothing left - so the
		// row's total stays the summary's.
		row.Progress = []int{sum.RequestsDone, sum.Running, sum.Awaiting, sum.ToDo + sum.WaitingOn + sum.Idle, sum.Blocked}
		row.NeedsYou = sum.Awaiting
		if sum.Awaiting > 0 {
			row.NeedsYouText = deliveryWord(lang, "needs_you")
		}
		if len(sum.Waits) > 0 {
			row.Waits = sum.Waits[0]
			if deliveryLocal(lang) {
				if w := incrementWaitsIn(ctx, cwm, o, lang); len(w) > 0 {
					row.Waits = w[0]
				}
			}
		}
	}
	return row
}

// Detail answers an increment's detail.
func (i *Increments) Detail(ctx context.Context, in api.IncrementDetailInput) (api.IncrementDetail, error) {
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementDetail{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementDetail{}, err
	}
	inc := toIncrement(ctx, o)
	d := api.IncrementDetail{Row: rowOf(ctx, cwm, o, ""), Eyebrow: fmt.Sprintf(deliveryWord(in.Language, "eyebrow"), inc.ID),
		Assigned: assignedText(ctx, cwm, inc, in.Language), Proposal: proposalText(o, in.Language, applyHeld(ctx, inc))}
	// Change release: the releases it is not in, and its backlog while it is
	// assigned - the row's choices, with the backlog.
	for _, opt := range releaseOptions(ctx, cwm, inc) {
		switch {
		case opt.ID != "" && (opt.ID == inc.ShipsIn || opt.ID == inc.PlannedFor):
		case opt.ID == "" && inc.ShipsIn == "" && inc.PlannedFor == "":
		default:
			d.Options = append(d.Options, opt)
		}
	}
	d.Goals = goalLines(ctx, cwm, o, inc.Delivery)
	return d, nil
}

// assignedText says where an increment is assigned, in one line, in lang.
func assignedText(ctx context.Context, cwm cwmapi.CWMService, inc api.Increment, lang string) string {
	switch {
	case inc.ShipsIn != "":
		return fmt.Sprintf(deliveryWord(lang, "ships_in"), releaseLabelOf(ctx, cwm, inc.ShipsIn))
	case inc.PlannedFor != "":
		say := fmt.Sprintf(deliveryWord(lang, "planned_for"), releaseLabelOf(ctx, cwm, inc.PlannedFor))
		if next, ok := nextPlannedRelease(ctx, cwm, inc.Stream); ok && next != inc.PlannedFor {
			say += fmt.Sprintf(deliveryWord(lang, "ships_once"), releaseLabelOf(ctx, cwm, next))
		}
		if inc.Moved != "" {
			say += " (" + inc.Moved + ")" // : why it moved
		}
		return say
	case inc.Stream == "":
		return deliveryWord(lang, "unassigned_no_stream")
	}
	say := deliveryWord(lang, "unassigned")
	if inc.Returned != "" {
		say += " - " + inc.Returned
	} else if inc.Moved != "" {
		say += " - " + inc.Moved
	}
	return say
}

// releaseLabelOf names a release by its id as every view names it.
func releaseLabelOf(ctx context.Context, cwm cwmapi.CWMService, id string) string {
	o, _, err := loadAny(ctx, cwm, id)
	if err != nil {
		return id
	}
	return api.ReleaseLabel(releaseFields(o).Version, titleOf(o))
}

// releaseOptions is Change release: every planned release of its stream - the
// next one ships it in, a later one plans it - and its backlog. None for an
// increment ended or on hold, or aimed at no stream.
func releaseOptions(ctx context.Context, cwm cwmapi.CWMService, inc api.Increment) []api.ReleaseOption {
	if inc.State != api.IncrementOpen || inc.Stream == "" || inc.Held != nil {
		return nil
	}
	next, _ := nextPlannedRelease(ctx, cwm, inc.Stream)
	rels, err := inStream(ctx, cwm, inc.Stream, api.ReleaseType)
	if err != nil {
		return nil
	}
	sort.SliceStable(rels, func(i, j int) bool { return releaseBefore(rels[i], rels[j]) })
	var out []api.ReleaseOption
	for _, r := range rels {
		if releaseState(r) != api.ReleasePlanned {
			continue
		}
		mode := "planned-for"
		if r.ID == next {
			mode = "ships-in"
		}
		out = append(out, api.ReleaseOption{ID: r.ID, Label: api.ReleaseLabel(releaseFields(r).Version, titleOf(r)), Mode: mode})
	}
	return append(out, api.ReleaseOption{Label: "Backlog (not assigned)", Mode: "backlog"})
}

// proposalText is where an increment's proposal stands, in lang: not run, its
// goals analysed, its operations awaiting the person's approval, approved and
// awaiting their apply (applyHeld: the apply step waits, the increment's work
// not running), applied.
func proposalText(o cwmapi.Object, lang string, applyHeld bool) string {
	if !hasRequest(o) {
		return deliveryWord(lang, "proposal_not_run")
	}
	f := fieldsOf(o)
	pending := pendingOps(f.Delta, f.Applied)
	switch {
	case pending > 0 && proposalApproved(o):
		say := fmt.Sprintf(deliveryWord(lang, "proposal_approved"), pending)
		if applyHeld {
			say += deliveryWord(lang, "proposal_apply_held")
		}
		return say
	case pending > 0:
		return fmt.Sprintf(deliveryWord(lang, "proposal_proposed"), pending)
	case len(f.Applied) > 0 && planEnded(planState(o)):
		return deliveryWord(lang, "proposal_applied")
	case len(f.Applied) > 0:
		return deliveryWord(lang, "proposal_applied_runs")
	}
	return deliveryWord(lang, "proposal_analysed")
}

// proposalApproved is whether the person approved the proposal as it stands:
// its last approve step ended approved after its last reconcile - a proposal
// sent back, or reconciled again for a new round, waits on a new approval.
func proposalApproved(o cwmapi.Object) bool {
	f, _ := planOf(o)
	for i := len(f.Steps) - 1; i >= 0; i-- {
		s := f.Steps[i]
		if s.Outcome == "" {
			continue
		}
		switch {
		case strings.HasSuffix(s.Step, "/"+api.StepReconcile):
			return false
		case strings.HasSuffix(s.Step, "/approve"):
			return s.Outcome == schedapi.StatusSuccess && strings.HasPrefix(s.Result, api.VerdictApproved+":")
		}
	}
	return false
}

// applyHeld is whether an increment's apply step waits - the increment on
// hold, or its apply to-do not yet released (its work does not run).
func applyHeld(ctx context.Context, inc api.Increment) bool {
	if inc.Held != nil {
		return true
	}
	todos, err := todosOf(ctx, inc.ID)
	if err != nil {
		return false
	}
	for _, t := range todos {
		if (t.Packet.Step == "apply" || strings.HasSuffix(t.Packet.Step, "/apply")) && t.State == schedapi.StateWaiting {
			return true
		}
	}
	return false
}

// goalLines is each delivery goal and what its proposal made of it, down to
// the requests serving each - the same walk as the increment's pane.
func goalLines(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, goals []string) []api.GoalLine {
	became := map[string][]string{}
	if hasRequest(o) {
		f := fieldsOf(o)
		for _, x := range f.Applied {
			if x.Op >= 0 && x.Op < len(f.Delta) {
				became[f.Delta[x.Op].For] = append(became[f.Delta[x.Op].For], x.ID)
			}
		}
	}
	var out []api.GoalLine
	line := func(goal string, ids []string) api.GoalLine {
		g := api.GoalLine{Goal: goal, State: "todo"}
		done, counted := true, 0
		for _, id := range ids {
			h := heldLine(ctx, cwm, id)
			// Done as a release counts it: done, or a decision in force;
			// what was dropped, superseded or retired holds nothing back.
			switch {
			case uncounted(h.State):
			case slices.Contains(targetDoneStates, h.State):
				counted++
			default:
				counted++
				done = false
			}
			g.Became = append(g.Became, h)
		}
		done = done && counted > 0
		switch {
		case done:
			g.State = api.StateDone
		case len(ids) > 0:
			g.State = "running"
		}
		return g
	}
	for i, goal := range goals {
		g := line(goal, became[goal])
		g.Chip = o.ID + "·G" + strconv.Itoa(i+1)
		out = append(out, g)
	}
	if other := became[""]; len(other) > 0 {
		g := line("For the increment as a whole", other)
		g.Chip = o.ID
		out = append(out, g)
	}
	return out
}

// heldLine is a goal or a feature and the requests serving it.
func heldLine(ctx context.Context, cwm cwmapi.CWMService, id string) api.HeldLine {
	t, err := loadObject(ctx, cwm, id)
	if err != nil {
		return api.HeldLine{ID: id, Title: id, State: "unread"}
	}
	h := api.HeldLine{ID: id, Title: titleOf(t), State: targetState(ctx, cwm, t)}
	serving, _ := incoming(ctx, cwm, id, api.EdgeServes)
	if hasRequest(t) {
		serving = append([]string{id}, serving...)
	}
	seen := map[string]bool{}
	for _, r := range serving {
		ro, err := loadObject(ctx, cwm, r)
		if err != nil || !hasRequest(ro) || seen[r] {
			continue
		}
		seen[r] = true
		h.Requests = append(h.Requests, api.RequestLine{ID: r, Title: titleOf(ro), State: stateOf(ro), Steps: stepsOf(ctx, ro), Where: whereInWork(ctx, r)})
	}
	return h
}

// stepsOf is a request's steps: those its plan recorded done (or failed,
// blocked), then its open to-dos by position - the one held running, one back
// in the queue failed or blocked, a person's step that can be taken on-you
// (it waits on a person), the others to do.
func stepsOf(ctx context.Context, o cwmapi.Object) []api.StepChip {
	// Its current round only (a request filed again starts again at its
	// intake), each step once, done when its last result succeeded.
	var out []api.StepChip
	at := map[string]int{}
	if f, ok := planOf(o); ok {
		for _, s := range f.Steps {
			name := s.Step
			if _, after, ok := strings.Cut(name, "/"); ok {
				name = after
			}
			if name == api.StepIntake && len(out) > 0 {
				out, at = nil, map[string]int{}
			}
			state := "todo"
			switch s.Outcome {
			case schedapi.StatusSuccess:
				state = "done"
			case schedapi.StatusFail:
				state = "failed"
			case schedapi.StatusBlocked:
				state = "blocked"
			}
			if i, ok := at[name]; ok {
				out[i].State = state
				continue
			}
			at[name] = len(out)
			out = append(out, api.StepChip{Name: name, State: state})
		}
	}
	todos, err := todosOf(ctx, o.ID)
	if err != nil {
		return out
	}
	sort.SliceStable(todos, func(i, j int) bool { return todos[i].Position < todos[j].Position })
	for _, t := range todos {
		step, _ := t.Work["step"].(string)
		state := todoStepState(t)
		if i, ok := at[step]; ok {
			out[i].State = state // redone, or failed and back in the queue
			continue
		}
		at[step] = len(out)
		out = append(out, api.StepChip{Name: step, State: state})
	}
	return out
}

// todoStepState is an open to-do's step as its dot shows it: held running,
// back in the queue failed or blocked, a person's step that can be taken
// on-you, else to do.
func todoStepState(t api.WorkTodo) string {
	stop, _ := stopOf(t)
	switch {
	case t.State == schedapi.StateClaimed:
		return "running"
	case stop == schedapi.StatusFail:
		return "failed"
	case stop == schedapi.StatusBlocked:
		return "blocked"
	case t.State == schedapi.StateReady && t.For == schedapi.ForPerson:
		return "on-you"
	}
	return "todo"
}
