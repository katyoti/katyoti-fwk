// SPDX-License-Identifier: Apache-2.0

// Package waiting is « À faire », the one place: everything
// waiting on a person, read from the human tasks, the inbox's pending entries
// and the contributed sources of the askers that do not yet ask through a
// human task - each question once, the person's own first, then each holding
// list they are a member of, overdue first, then by kind.
package waiting

import (
	"context"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/internal/humantask"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// SourceInbox is the inbox's own source: its human tasks and its entries.
const SourceInbox = api.WaitingSourceInbox

// KindOverdue is the filter keeping only the overdue questions.
const KindOverdue = "overdue"

// The kinds an inbox entry is asked as, by what its answer is.
const kindNotice = "notice"

// clock is the time dates are read against (a test sets it).
var clock = time.Now

// Service is « À faire ».
type Service struct{}

// New answers the service.
func New() *Service { return &Service{} }

var _ api.WaitingService = (*Service)(nil)

// sources are the contributed sources (WaitingSource), read at each call.
var sources = func(ctx context.Context) []api.WaitingSource {
	return application.UseExtensions[api.WaitingSource](ctx)
}

// Count answers how many questions wait on the person: the rail's badge.
func (s *Service) Count(ctx context.Context, in api.WaitingInput) (api.WaitingCountOutput, error) {
	in.Kind = ""
	out, err := s.List(ctx, in)
	if err != nil {
		return api.WaitingCountOutput{}, err
	}
	return api.WaitingCountOutput{Count: out.Count}, nil
}

// Inbox answers what the rail's Inbox opens: « À faire ».
func (s *Service) Inbox(context.Context, api.InboxInput) (api.InboxScreen, error) {
	// « À faire » is planification's previous page reused, every waiting item
	// in it: the rail opens it, with its count.
	return api.InboxScreen{Page: api.PersonQueuePageURI, Count: api.PersonQueueCountURI, Replaces: []string{api.FormerPersonQueuePanel}}, nil
}

// List answers everything waiting on the person.
func (s *Service) List(ctx context.Context, in api.WaitingInput) (api.WaitingOutput, error) {
	me, name := person(ctx, in.Person)
	kinds := kindsOf(ctx)
	now := clock()

	var items []api.WaitingItem
	var recent []api.WaitingItem
	srcs := sources(ctx)
	for _, src := range srcs {
		if src.Items == nil || api.SourceLeftOut(ctx, src.Name) {
			continue
		}
		got, err := src.Items(ctx)
		if err != nil {
			slog.Warn("inbox: a source of « À faire » cannot be read; its questions are left out", "source", src.Name, "err", err)
			continue
		}
		for _, it := range got {
			it.Source = src.Name
			if it.Kind == kindNotice {
				recent = append(recent, it)
				continue
			}
			items = append(items, it)
		}
	}
	carried := map[string]bool{}
	for _, it := range items {
		if it.Entry != "" {
			carried[it.Entry] = true
		}
	}
	for _, it := range recent {
		if it.Entry != "" {
			carried[it.Entry] = true
		}
	}

	asked, notices, err := entryItems(ctx, srcs, carried)
	if err != nil {
		return api.WaitingOutput{}, err
	}
	items = append(items, asked...)
	recent = append(recent, notices...)

	tasks, lists, err := humanTaskItems(ctx, me, now)
	if err != nil {
		return api.WaitingOutput{}, err
	}
	items = append(items, tasks...)

	// The kinds' labels in the person's language (: the
	// screen that draws this list shows them as they come).
	lang := LanguageOf(ctx, in.Language)
	for i := range items {
		// Always in the person's language: a human task's to-do carries the
		// label its kind had when it was asked.
		own := items[i].Label
		if own == "" {
			own = kinds.label(items[i].Kind)
		}
		items[i].Label = KindLabel(lang, items[i].Kind, own)
		if items[i].Module == "" && items[i].About != "" {
			items[i].Module, _ = cwmapi.ModuleOfObjectID(items[i].About)
		}
		// Asked with answers: every question carries its answer nodes - the
		// inbox's own and those its source contributes -, for a screen that is
		// not the inbox's (RQ-1017).
		if in.Answers {
			items[i].Answer = controlsOf(ctx, items[i], lang)
		}
	}
	// A notice carries its controls too: the inbox's own its « Écarter ».
	if in.Answers {
		for i := range recent {
			recent[i].Answer = controlsOf(ctx, recent[i], lang)
		}
	}
	sortItems(items, kinds)
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].Since > recent[j].Since })

	out := api.WaitingOutput{Person: me, Name: name, Groups: []api.WaitingGroup{}, Recent: recent}
	counts := map[string]int{}
	for _, it := range items {
		out.Count++
		counts[it.Kind]++
		if it.Overdue {
			out.Overdue++
		}
	}
	for _, k := range kinds.order(counts) {
		out.Kinds = append(out.Kinds, api.KindCount{Kind: k, Label: KindLabel(lang, k, kinds.label(k)), Count: counts[k]})
	}

	mine := api.WaitingGroup{Mine: true, Items: []api.WaitingItem{}}
	byList := map[string]*api.WaitingGroup{}
	var order []string
	for _, it := range items {
		if !keep(it, in.Kind) {
			continue
		}
		if it.List == "" {
			mine.Items = append(mine.Items, it)
			continue
		}
		g, ok := byList[it.List]
		if !ok {
			l := lists[it.List]
			g = &api.WaitingGroup{List: it.List, Title: l.title, Members: l.members, Items: []api.WaitingItem{}}
			byList[it.List] = g
			order = append(order, it.List)
		}
		g.Items = append(g.Items, it)
	}
	// Every list the person is a member of shows, even with nothing waiting:
	// what waits on the team is seen at a glance.
	for id, l := range lists {
		if _, ok := byList[id]; !ok && l.member {
			byList[id] = &api.WaitingGroup{List: id, Title: l.title, Members: l.members, Items: []api.WaitingItem{}}
			order = append(order, id)
		}
	}
	sort.Strings(order)
	if in.Kind == "" || in.Kind == api.KindConversation {
		out.Conversations = conversationsOf(ctx)
	}
	out.Groups = append(out.Groups, mine)
	for _, id := range order {
		out.Groups = append(out.Groups, *byList[id])
	}
	return out, nil
}

// keep says an item shows under the filter: every one when empty, the
// overdue ones, or one kind's.
func keep(it api.WaitingItem, kind string) bool {
	switch kind {
	case "":
		return true
	case KindOverdue:
		return it.Overdue
	case api.KindConversation:
		return false
	default:
		return it.Kind == kind
	}
}

// sortItems orders the questions: overdue first, then by kind (its order
// among the kinds), then priority, then due date, then age.
func sortItems(items []api.WaitingItem, kinds kindBook) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Overdue != b.Overdue {
			return a.Overdue
		}
		if ra, rb := kinds.rank(a.Kind), kinds.rank(b.Kind); ra != rb {
			return ra < rb
		}
		if pa, pb := priorityRank(a.Priority), priorityRank(b.Priority); pa != pb {
			return pa < pb
		}
		if a.Due != b.Due {
			return a.Due != "" && (b.Due == "" || a.Due < b.Due)
		}
		if a.Since != b.Since {
			return a.Since < b.Since
		}
		return a.Key < b.Key
	})
}

// priorityRank is P0 first; unset as P2.
func priorityRank(p string) int {
	if len(p) == 2 && p[0] == 'P' && p[1] >= '0' && p[1] <= '3' {
		return int(p[1] - '0')
	}
	return 2
}

// person is whose list: the one named, else the profile's handle, with their
// name.
func person(ctx context.Context, handle string) (string, string) {
	p, err := profileOf(ctx)
	if handle == "" {
		if err != nil {
			return "user", ""
		}
		return p.ID, p.DisplayName()
	}
	if err == nil && p.ID == handle {
		return handle, p.DisplayName()
	}
	return handle, ""
}

// profileOf reads the profile (a test sets it).
var profileOf = func(ctx context.Context) (profileapi.Profile, error) {
	svc, err := application.UseService[profileapi.ProfileService](ctx)
	if err != nil {
		return profileapi.Profile{}, err
	}
	return svc.Get(ctx)
}

// LanguageOf is the language the page is in: the one asked, else the profile's.
func LanguageOf(ctx context.Context, lang string) string {
	if lang != "" {
		return lang
	}
	if p, err := profileOf(ctx); err == nil {
		return p.Language
	}
	return ""
}

// askKinds reads the contributed kinds of questions (a test sets it).
var askKinds = humantask.Kinds

// kindBook is the contributed kinds of questions: their order and label.
type kindBook struct {
	ranks  map[string]int
	labels map[string]string
}

func kindsOf(ctx context.Context) kindBook {
	b := kindBook{ranks: map[string]int{}, labels: map[string]string{}}
	ks, err := askKinds(ctx)
	if err != nil {
		slog.Debug("inbox: the kinds of questions cannot be read; « À faire » orders by name", "err", err)
	}
	for i, k := range ks {
		b.ranks[k.Name] = i
		b.labels[k.Name] = k.Label
	}
	return b
}

func (b kindBook) rank(k string) int {
	if r, ok := b.ranks[k]; ok {
		return r
	}
	return len(b.ranks)
}

func (b kindBook) label(k string) string {
	if l := b.labels[k]; l != "" {
		return l
	}
	return k
}

// order is the kinds counted, in the kinds' order (unknown ones by name, last).
func (b kindBook) order(counts map[string]int) []string {
	out := make([]string, 0, len(counts))
	for k := range counts {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if ri, rj := b.rank(out[i]), b.rank(out[j]); ri != rj {
			return ri < rj
		}
		return out[i] < out[j]
	})
	return out
}

// entryItems are the inbox's pending entries no source carries or answers
// for: an approval request asked as its kind (a verdict to approve, a text to
// give, a choice), a notice - or any other entry filed for triage - for
// information only.
func entryItems(ctx context.Context, srcs []api.WaitingSource, carried map[string]bool) ([]api.WaitingItem, []api.WaitingItem, error) {
	inbox, err := application.UseService[api.InboxService](ctx)
	if err != nil {
		return nil, nil, nil
	}
	tri, err := inbox.Triage(ctx, api.TriageInput{})
	if err != nil {
		return nil, nil, err
	}
	var asked, notices []api.WaitingItem
	for _, e := range tri.Entries {
		if carried[e.ID] || answeredElsewhere(srcs, e) {
			continue
		}
		it := api.WaitingItem{Key: SourceInbox + ":" + e.ID, Source: SourceInbox, Title: e.Title, About: e.About,
			Since: e.RequestedAt, Entry: e.ID, Ref: e.Expects, AskedBy: e.RequestedBy}
		if e.About != "" && strings.Contains(e.About, ".") {
			it.Module, _ = cwmapi.ModuleOfObjectID(e.About)
		}
		switch {
		case e.Type != api.RequestType, e.Expects == api.ExpectsNotice:
			if e.Type == api.RequestType && e.Status != "" && e.Status != api.StatusPending {
				continue
			}
			it.Kind = kindNotice
			notices = append(notices, it)
			continue
		case e.Status != "" && e.Status != api.StatusPending:
			continue
		case e.Expects == api.ExpectsText:
			it.Kind = api.KindForm
		case e.Expects == api.ExpectsChoice:
			it.Kind = api.KindChoose
		default:
			it.Kind = api.KindApprove
		}
		if e.Urgency == "high" {
			it.Priority = "P1"
		}
		asked = append(asked, it)
	}
	return asked, notices, nil
}

// answeredElsewhere says a source answers for the entry: shown through its
// items only.
func answeredElsewhere(srcs []api.WaitingSource, e api.Entry) bool {
	for _, s := range srcs {
		if s.Answers != nil && s.Answers(e) {
			return true
		}
	}
	return false
}

// list is a holding list as « À faire » shows it.
type list struct {
	title   string
	members []api.Member
	filed   bool
	member  bool
}

// humanTaskItems are the open human tasks waiting on the person: given to
// them, taken by them, or waiting on a list that is theirs - one filed with
// them a member (shown under it), or one no list is filed for (open: their
// own). A question given to or taken by another, or on a list they are not a
// member of, is not theirs.
func humanTaskItems(ctx context.Context, me string, now time.Time) ([]api.WaitingItem, map[string]list, error) {
	lists, err := memberLists(ctx, me)
	if err != nil {
		return nil, nil, err
	}
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return nil, lists, nil
	}
	all, err := todos.List(ctx, schedapi.ListInput{})
	if err != nil {
		return nil, lists, err
	}
	var out []api.WaitingItem
	for _, t := range all.Todos {
		if t.Owner != api.Owner || t.Result != nil || (t.State != schedapi.StateReady && t.State != schedapi.StateClaimed) {
			continue
		}
		it, ok := humanTaskItem(t, now)
		if !ok {
			continue
		}
		isMe := func(a *cwmapi.Actor) bool { return a != nil && a.ID == me && profileapi.IsPerson(*a) }
		l, known := lists[t.Group]
		if !known {
			l = listOf(ctx, t.Group, me)
			lists[t.Group] = l
		}
		switch {
		case isMe(t.Assignee), t.Claim != nil && isMe(&t.Claim.By):
			// Given to them, or taken by them: theirs.
		case t.Assignee != nil:
			continue // given to another
		case !l.filed:
			// An open list (none filed): theirs, as everyone's.
		case l.member:
			it.List = t.Group
		default:
			continue
		}
		if t.Claim != nil {
			by := t.Claim.By
			it.ClaimedBy = &by
		}
		out = append(out, it)
	}
	for id, l := range lists {
		if !l.filed {
			delete(lists, id)
		}
	}
	return out, lists, nil
}

// humanTaskItem is a human task's to-do as a question.
func humanTaskItem(t schedapi.Todo, now time.Time) (api.WaitingItem, bool) {
	ask, ok := t.Work[api.WorkAsk].(map[string]any)
	if !ok {
		return api.WaitingItem{}, false
	}
	spec, err := humantask.SpecOf(ask)
	if err != nil {
		return api.WaitingItem{}, false
	}
	it := api.WaitingItem{
		Key: SourceInbox + ":" + t.ID, Source: SourceInbox, Kind: spec.Kind, Label: spec.Label, Title: spec.Title,
		Asks: spec.Question, Options: spec.Options, FormDef: spec.FormDef, About: spec.About, Request: t.Request, Module: t.Module, Satisfies: spec.Satisfies,
		AnswerableBy: spec.AnswerableBy, Proof: spec.Proof, Priority: t.Priority, Since: t.Since, Todo: t.ID, AskedBy: spec.AskedBy,
	}
	if it.Since == "" {
		it.Since = t.Made // since it was asked
	}
	if l, _ := t.Work[api.WorkLabel].(string); l != "" {
		it.Label = l
	}
	if due, _ := t.Work[api.WorkDue].(string); due != "" {
		it.Due = due
	}
	it.Overdue = api.Overdue(t, now)
	return it, true
}

// memberLists are the holding lists filed in sys-inbox the person is a member
// of.
func memberLists(ctx context.Context, me string) (map[string]list, error) {
	out := map[string]list{}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return out, nil
	}
	got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: api.HoldingListType})
	if err != nil {
		slog.Warn("inbox: the holding lists cannot be read; « À faire » shows the person's own questions", "err", err)
		return out, nil
	}
	for _, o := range got.Objects {
		l := listOf(ctx, o.ID, me)
		if l.member {
			out[o.ID] = l
		}
	}
	return out, nil
}

// listOf reads one holding list: its name, its members, whether it is filed
// and whether the person is one of its members.
func listOf(ctx context.Context, id, me string) list {
	ms, listed, err := humantask.ListMembers(ctx, id)
	if err != nil || !listed {
		return list{}
	}
	l := list{title: id, filed: true}
	if cwm, err := application.UseService[cwmapi.CWMService](ctx); err == nil {
		if module, err := cwmapi.ModuleOfObjectID(id); err == nil {
			if got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id}); err == nil {
				l.title = got.Object.DisplayTitle()
			}
		}
	}
	for _, m := range ms {
		l.members = append(l.members, api.Member{ID: m.ID, Kind: m.Kind})
		if m.ID == me && profileapi.IsPerson(m) {
			l.member = true
		}
	}
	return l
}

// controlsOf asks an item's source for its controls; the inbox's own items
// get the inbox's.
func controlsOf(ctx context.Context, it api.WaitingItem, lang string) []render.Node {
	if it.Source == SourceInbox {
		return inboxControls(it, lang)
	}
	i := slices.IndexFunc(sources(ctx), func(s api.WaitingSource) bool { return s.Name == it.Source })
	if i < 0 || sources(ctx)[i].Controls == nil {
		return nil
	}
	return sources(ctx)[i].Controls(ctx, it, lang)
}
