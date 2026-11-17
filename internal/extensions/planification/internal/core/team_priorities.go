// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// TEAM PRIORITIES (view 7 of the mock-up
// modules/research-delivery-overview/delivery-views-v2-mockup.html) - the
// weekly team meeting's page: a toggle per stream and one for what the
// current release delivered on top; then the increments of the streams shown
// in the order they will be done - delivered in the current releases, then a
// section per open release of every stream shown (each stream's current
// release its own), by due date with the streams' releases interleaved (the
// undated last; a same date by stream order, then id), then « No release »,
// then the open increments in no stream under « No stream » (shown whatever
// streams are picked: none is hidden) - a separator per
// group naming its release with its due date and days left, its number of
// increments and how much is done; inside a group by priority, then rank (the
// id's number: the older first, the scheduler's tie-break). A stream's
// releases still to ship (planned or frozen) by date are its current one, its
// next and the others. Each line changes its priority (board-move) and
// its release (board-ship: the next release ships it in, a later one plans
// it, « No release » takes it out) at once - no meeting validation - and the
// host re-reads the page: the list re-sorts.

const teamPrioritiesURI = "ktw-agents:increments/team-priorities-page"

// teamWords are the page's words, per language.
var teamWords = map[string]map[string]string{
	"en": {
		"title": "Team priorities", "streams": "Streams", "delivered_toggle": "Delivered in the current release",
		"delivered": "Delivered in the current releases", "current": "Current release", "next": "Next release",
		"none": "No release", "due": "due", "days_left": "days left", "day_left": "day left",
		"late": "days late", "increments": "increments", "increment": "increment", "done_pct": "done",
		"not_planned": "not planned", "legend_done": "done", "legend_running": "running",
		"legend_hand": "✋ waits on a person", "legend_warn": "⚠ blocked", "badge_delivered": "delivered",
		"opt_current": "(current)", "opt_next": "(next)", "opt_none": "— No release",
		"hand": "A step waits on a person — see ⚠.", "foot": "Each change applies at once.",
		"pick": "Pick a stream.", "priority": "Priority", "release": "Release", "no-stream": "No stream",
	},
	"fr": {
		"title": "Priorités d'équipe", "streams": "Flux", "delivered_toggle": "Livré dans la version actuelle",
		"delivered": "Livré dans les versions actuelles", "current": "Version actuelle", "next": "Version suivante",
		"none": "Sans version", "due": "échéance", "days_left": "jours restants", "day_left": "jour restant",
		"late": "jours de retard", "increments": "incréments", "increment": "incrément", "done_pct": "fait",
		"not_planned": "non planifié", "legend_done": "fait", "legend_running": "en cours",
		"legend_hand": "✋ attend une personne", "legend_warn": "⚠ bloqué", "badge_delivered": "livré",
		"opt_current": "(actuelle)", "opt_next": "(suivante)", "opt_none": "— Sans version",
		"hand": "Une étape attend une personne — voir ⚠.", "foot": "Chaque changement s'applique immédiatement.",
		"pick": "Choisissez un flux.", "priority": "Priorité", "release": "Version", "no-stream": "Sans flux",
	},
}

func teamWord(lang, key string) string {
	if l, ok := teamWords[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return teamWords["en"][key]
}

// teamReleaseKinds name a release group by its place in its stream: its
// current release, its next, then the others (TeamGroupRelease).
var teamReleaseKinds = []string{api.TeamGroupCurrent, api.TeamGroupNext}

// teamReleaseGroup is a release's group with its stream's place, which
// orders releases due the same day.
type teamReleaseGroup struct {
	group  *api.TeamGroup
	stream int
}

// TeamPriorities answers Team priorities' data.
func (i *Increments) TeamPriorities(ctx context.Context, in api.TeamPrioritiesInput) (api.TeamPrioritiesOutput, error) {
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.TeamPrioritiesOutput{}, err
	}
	streams, err := orderedStreams(ctx, cwm)
	if err != nil {
		return api.TeamPrioritiesOutput{}, err
	}
	out := api.TeamPrioritiesOutput{Streams: []api.TeamStream{}, HideDelivered: in.HideDelivered, Groups: []api.TeamGroup{}}
	single := map[string]*api.TeamGroup{}
	for _, k := range []string{api.TeamGroupDelivered, api.TeamGroupNone, api.TeamGroupNoStream} {
		single[k] = &api.TeamGroup{Kind: k, Lines: []api.TeamLine{}}
	}
	// One group per open release, keyed by the release.
	byRelease := map[string]*teamReleaseGroup{}
	var releaseGroups []*teamReleaseGroup
	for sn, so := range streams {
		st := toStream(so)
		on := !slices.Contains(in.HiddenStreams, st.ID)
		// CurrentOnly reads each stream's current release only, and only
		// the increments that ship in it.
		most := 0
		if in.CurrentOnly {
			most = 1
		}
		rels := openReleasesOf(ctx, cwm, st.ID, most)
		out.Streams = append(out.Streams, api.TeamStream{ID: st.ID, Name: st.Name, Color: st.Color, On: on, Releases: rels})
		if !on {
			continue
		}
		pos := map[string]int{}
		for n, r := range rels {
			pos[r.ID] = n
		}
		incs, err := inStream(ctx, cwm, st.ID, api.IncrementType)
		if err != nil {
			continue
		}
		for _, o := range incs {
			if in.CurrentOnly && (len(rels) == 0 || edgeTo(o, api.EdgeShipsIn) != rels[0].ID) {
				continue
			}
			inc := toIncrement(ctx, o)
			release := inc.ShipsIn + inc.PlannedFor
			var g *api.TeamGroup
			switch inc.State {
			case api.IncrementDone:
				if len(rels) == 0 || inc.ShipsIn != rels[0].ID || in.HideDelivered || in.CurrentOnly {
					continue // older history, or hidden
				}
				g = single[api.TeamGroupDelivered]
			case api.IncrementOpen:
				n, ok := pos[release]
				if in.CurrentOnly && (!ok || n != 0) {
					continue // not its stream's current release
				}
				if !ok {
					g, release = single[api.TeamGroupNone], ""
					break
				}
				rg := byRelease[release]
				if rg == nil {
					kind := api.TeamGroupRelease
					if n < len(teamReleaseKinds) {
						kind = teamReleaseKinds[n]
					}
					rg = &teamReleaseGroup{stream: sn, group: &api.TeamGroup{Kind: kind, Stream: st.ID,
						Releases: []api.TeamRelease{rels[n]}, Lines: []api.TeamLine{}}}
					byRelease[release] = rg
					releaseGroups = append(releaseGroups, rg)
				}
				g = rg.group
			default:
				continue // dropped
			}
			line := teamLineOf(ctx, inc, release, g.Kind == api.TeamGroupDelivered)
			if !line.Delivered {
				line.Options = releaseChoices(rels, release)
			}
			g.Lines = append(g.Lines, line)
		}
	}
	// The release groups by their release's own date across the streams -
	// the undated last - a same date by stream order, then id.
	sort.SliceStable(releaseGroups, func(a, b int) bool {
		ra, rb := releaseGroups[a].group.Releases[0], releaseGroups[b].group.Releases[0]
		if c := releaseDateCmp(ra.Date, rb.Date); c != 0 {
			return c < 0
		}
		if sa, sb := releaseGroups[a].stream, releaseGroups[b].stream; sa != sb {
			return sa < sb
		}
		return idBefore(ra.ID, rb.ID)
	})
	ordered := []*api.TeamGroup{single[api.TeamGroupDelivered]}
	for _, rg := range releaseGroups {
		ordered = append(ordered, rg.group)
	}
	ordered = append(ordered, single[api.TeamGroupNone], single[api.TeamGroupNoStream])
	// The open increments in no stream: the views list increments through
	// their stream, so these would be hidden everywhere.
	if lone, err := noStreamIncrements(ctx, cwm); err == nil && !in.CurrentOnly {
		g := single[api.TeamGroupNoStream]
		for _, o := range lone {
			inc := toIncrement(ctx, o)
			g.Lines = append(g.Lines, teamLineOf(ctx, inc, inc.ShipsIn+inc.PlannedFor, false))
		}
	}
	for _, g := range ordered {
		if len(g.Lines) == 0 {
			continue
		}
		sort.SliceStable(g.Lines, func(a, b int) bool {
			if pa, pb := priorityRank(g.Lines[a].Priority), priorityRank(g.Lines[b].Priority); pa != pb {
				return pa < pb
			}
			if na, nb := idNumber(g.Lines[a].ID), idNumber(g.Lines[b].ID); na != nb {
				return na < nb
			}
			return g.Lines[a].ID < g.Lines[b].ID
		})
		sum := 0
		for _, l := range g.Lines {
			sum += l.Done
		}
		g.Done = (sum + len(g.Lines)/2) / len(g.Lines)
		out.Groups = append(out.Groups, *g)
	}
	return out, nil
}

// orderedStreams are every stream in their drawing order.
func orderedStreams(ctx context.Context, cwm cwmapi.CWMService) ([]cwmapi.Object, error) {
	got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: api.StreamType})
	if err != nil {
		return nil, err
	}
	var streams []cwmapi.Object
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
	return streams, nil
}

// openReleasesOf are a stream's releases still to ship - planned or frozen -
// on their own dates, the dated first, the undated last: its current one,
// its next, then the others - the first most of them when most > 0.
func openReleasesOf(ctx context.Context, cwm cwmapi.CWMService, stream string, most int) []api.TeamRelease {
	rels, err := inStream(ctx, cwm, stream, api.ReleaseType)
	if err != nil {
		return nil
	}
	sort.SliceStable(rels, func(i, j int) bool { return releaseBefore(rels[i], rels[j]) })
	var out []api.TeamRelease
	for _, r := range rels {
		st := releaseState(r)
		if st != api.ReleasePlanned && st != api.ReleaseFrozen {
			continue
		}
		if most > 0 && len(out) == most {
			break
		}
		// Its own date: an undated release shows no due date, never the
		// stream's next cut it is placed at. Its label and days left only -
		// never its whole card, whose scope reads every object it ships.
		f := releaseFields(r)
		tr := api.TeamRelease{ID: r.ID, Label: api.ReleaseLabel(f.Version, titleOf(r)), Date: f.Date, Frozen: st == api.ReleaseFrozen}
		if n, ok := calendarDaysLeft(f.Date, time.Now()); ok {
			tr.DaysLeft = &n
		}
		out = append(out, tr)
	}
	return out
}

// releaseChoices are what a line's release drop-down offers: its stream's
// releases still to ship - a frozen one only to the line already in it, the
// verbs refusing it to any other - then none (out).
func releaseChoices(rels []api.TeamRelease, release string) []api.RowOption {
	var out []api.RowOption
	for _, r := range rels {
		if r.Frozen && r.ID != release {
			continue
		}
		out = append(out, api.RowOption{Value: r.ID, Label: r.Label})
	}
	return append(out, api.RowOption{Value: "out"})
}

// teamLineOf is one increment's line: its progress, whether a step waits on
// a person, and what blocks it (what it waits on, one line each).
func teamLineOf(ctx context.Context, inc api.Increment, release string, delivered bool) api.TeamLine {
	l := api.TeamLine{ID: inc.ID, Title: inc.Title, Description: inc.Description, Stream: inc.Stream,
		Priority: incrementRowPriority(inc), Release: release, Delivered: delivered}
	if delivered {
		l.Done = 100
		return l
	}
	sum, err := NewIncrements().Summary(ctx, api.GetInput{ID: inc.ID})
	if err != nil {
		return l
	}
	total := sum.RequestsDone + sum.Running + sum.Awaiting + sum.ToDo + sum.WaitingOn + sum.Idle + sum.Blocked
	if total > 0 {
		l.Done = sum.RequestsDone * 100 / total
		l.Running = sum.Running * 100 / total
	}
	l.NeedsYou = sum.Awaiting > 0
	l.Blocks = strings.Join(sum.Waits, "\n")
	return l
}

// TeamPrioritiesPage answers the Team priorities page.
func (i *Increments) TeamPrioritiesPage(ctx context.Context, in api.TeamPrioritiesInput) (render.Def, error) {
	d, err := i.TeamPriorities(ctx, in)
	if err != nil {
		return render.Def{}, err
	}
	lang := in.Language
	w := func(k string) string { return teamWord(lang, k) }
	// The top: a toggle per stream - a click shows or hides it - then the
	// delivered toggle and the legend.
	var hidden []string
	for _, s := range d.Streams {
		if !s.On {
			hidden = append(hidden, s.ID)
		}
	}
	top := []node{{"kind": "text", "content": strings.ToUpper(w("streams")), "size": "xs", "color": "dimmed"}}
	for _, s := range d.Streams {
		flipped := []string{}
		for _, h := range hidden {
			if h != s.ID {
				flipped = append(flipped, h)
			}
		}
		if s.On {
			flipped = append(flipped, s.ID)
		}
		// On = filled in the brand colour (clearly selected); off = outline
		// in the same colour (clearly not); a coloured dot keeps the stream
		// told apart.
		variant := "outline"
		if s.On {
			variant = "filled"
		}
		button := render.Button(s.Name, render.QueryWith(s.Name, map[string]any{"hidden_streams": flipped, "hide_delivered": d.HideDelivered})).
			With("id", "tp-stream:"+s.ID).With("variant", variant).With("size", "xs").With("color", "blue").With("data-stream", s.ID).With("data-on", s.On)
		item := button
		if s.Color != "" {
			dot := node{"kind": "monogram", "text": " ", "color": s.Color, "size": 10}
			item = node{"kind": "hbox", "gap": 6, "align": "center", "children": []node{dot, button}}
		}
		top = append(top, item)
	}
	delVariant := "outline"
	if !d.HideDelivered {
		delVariant = "filled"
	}
	top = append(top,
		render.Button(w("delivered_toggle"), render.QueryWith(w("delivered_toggle"), map[string]any{"hidden_streams": nonNil(hidden), "hide_delivered": !d.HideDelivered})).
			With("id", "tp-delivered").With("variant", delVariant).With("size", "xs").With("color", "blue").With("data-on", !d.HideDelivered),
		render.Badge(w("legend_done"), "green").With("icon", "circle-filled"),
		render.Badge(w("legend_running"), "orange").With("icon", "circle-filled"),
		render.Badge(w("legend_hand"), "yellow"),
		render.Badge(w("legend_warn"), "yellow"),
	)
	kids := []node{{"kind": "hbox", "gap": "sm", "wrap": true, "align": "center", "data-view": "team-priorities-top", "children": top}}
	if len(d.Groups) == 0 {
		kids = append(kids, render.Dimmed(w("pick")))
	}
	// Each group's lines are Delivery's increment rows (its « team » table):
	// priority, title with its id and release, progress, what it waits on,
	// Change release and the row's menu - in the group's order.
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	// With more than one stream shown, a release group's label names its
	// stream: the streams' current releases read apart.
	names := map[string]string{}
	for _, s := range d.Streams {
		if s.On {
			names[s.ID] = s.Name
		}
	}
	for _, g := range d.Groups {
		stream := ""
		if len(names) > 1 {
			stream = names[g.Stream]
		}
		kids = append(kids, teamSeparator(lang, g, stream), teamGroupTable(ctx, cwm, lang, g, "team"))
	}
	kids = append(kids, render.Dimmed(w("foot")))
	page := node{"kind": "vbox", "gap": "xs", "data-view": "team-priorities", "children": kids}
	return render.Def{Kind: "page", Title: w("title"), DSLTree: page}, nil
}

// teamGroupTable is a group's lines as Delivery's increment rows, in the
// group's order, drawn in a variant of the increment-row data type - Team
// priorities' « team », the deck Overview's « overview » -; a click on a row
// opens the increment.
func teamGroupTable(ctx context.Context, cwm cwmapi.CWMService, lang string, g api.TeamGroup, variant string) node {
	rows := make([]api.IncrementRow, 0, len(g.Lines))
	for _, l := range g.Lines {
		o, err := loadIncrement(ctx, cwm, l.ID)
		if err != nil {
			continue
		}
		rows = append(rows, rowOf(ctx, cwm, o, lang))
	}
	return node{"kind": "table", "schema": incrementRowSchema, "variant": variant, "data": rows,
		"id": "tp-rows:" + teamGroupKey(g), "data-group": g.Kind, "open": render.NavigateTo("Open", "${item.id}", "")}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// teamSeparatorTone is a group's accent colour (the mock-up's), used for the
// small dot before its label and the divider under it - a dark-theme-friendly
// break, no light-tint band.
var teamSeparatorTone = map[string]string{
	api.TeamGroupDelivered: "green", api.TeamGroupCurrent: "blue", api.TeamGroupNext: "teal", api.TeamGroupRelease: "gray", api.TeamGroupNone: "gray",
	api.TeamGroupNoStream: "orange",
}

// teamGroupKey tells a group apart on the page: its kind, and a release
// group's release (tp-sep:current:<release>).
func teamGroupKey(g api.TeamGroup) string {
	if g.Stream != "" && len(g.Releases) == 1 {
		return g.Kind + ":" + g.Releases[0].ID
	}
	return g.Kind
}

// teamSeparatorText is a group's separator line: its label - after its
// stream's name when given - then its release with its due date and days
// left, its count and how much is done.
func teamSeparatorText(lang string, g api.TeamGroup, stream string) (label, meta string) {
	w := func(k string) string { return teamWord(lang, k) }
	label = w(g.Kind)
	if stream != "" {
		label = stream + " · " + label
	}
	n := len(g.Lines)
	count := fmt.Sprintf("%d %s", n, w("increments"))
	if n == 1 {
		count = fmt.Sprintf("1 %s", w("increment"))
	}
	if g.Kind == api.TeamGroupDelivered || g.Kind == api.TeamGroupNoStream {
		return label, count
	}
	var rels []string
	for _, r := range g.Releases {
		s := r.Label
		if r.Date != "" {
			s += " · " + w("due") + " " + dayMonth(r.Date)
		}
		if r.DaysLeft != nil {
			switch d := *r.DaysLeft; {
			case d < 0:
				s += fmt.Sprintf(" · %d %s", -d, w("late"))
			case d == 1:
				s += fmt.Sprintf(" · 1 %s", w("day_left"))
			default:
				s += fmt.Sprintf(" · %d %s", d, w("days_left"))
			}
		}
		rels = append(rels, s)
	}
	where := strings.Join(rels, " — ")
	if where == "" {
		where = w("not_planned")
	}
	meta = where + " · " + count
	if g.Kind != api.TeamGroupNone {
		meta += fmt.Sprintf(" · %d%% %s", g.Done, w("done_pct"))
	}
	return label, meta
}

// dayMonth writes a YYYY-MM-DD date as the mock-up does: 31/10.
func dayMonth(date string) string {
	if len(date) == 10 && date[4] == '-' && date[7] == '-' {
		return date[8:10] + "/" + date[5:7]
	}
	return date
}

func teamSeparator(lang string, g api.TeamGroup, stream string) node {
	label, meta := teamSeparatorText(lang, g, stream)
	var ids []string
	for _, r := range g.Releases {
		ids = append(ids, r.ID)
	}
	tone := teamSeparatorTone[g.Kind]
	// A plain hbox: a small coloured dot, the group's label, its meta dimmed;
	// a thin divider under it in the tone - no light-tint background.
	row := node{"kind": "hbox", "gap": "sm", "align": "center", "wrap": true, "children": []node{
		{"kind": "monogram", "text": " ", "color": tone, "size": 10},
		{"kind": "text", "content": label, "size": "sm", "fw": 700},
		{"kind": "text", "content": meta, "size": "xs", "color": "dimmed"},
	}}
	sep := node{"kind": "vbox", "gap": 2, "id": "tp-sep:" + teamGroupKey(g), "data-group": g.Kind, "data-releases": strings.Join(ids, " "),
		"children": []node{row, {"kind": "divider", "color": tone}}}
	if g.Stream != "" {
		sep["data-stream"] = g.Stream
	}
	return sep
}

// noStreamIncrements are the open increments in no stream, by id.
func noStreamIncrements(ctx context.Context, cwm cwmapi.CWMService) ([]cwmapi.Object, error) {
	got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: api.IncrementType})
	if err != nil {
		return nil, err
	}
	var out []cwmapi.Object
	for _, it := range got.Objects {
		o, err := loadIncrement(ctx, cwm, it.ID)
		if err != nil || releaseState(o) != api.IncrementOpen || edgeTo(o, api.EdgeInStream) != "" {
			continue
		}
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
