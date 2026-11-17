// SPDX-License-Identifier: Apache-2.0

package waiting

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	inboxcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/contrib"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// fakeTodos lists the scheduler's to-dos.
type fakeTodos struct {
	schedapi.TodoService
	todos []schedapi.Todo
}

func (f fakeTodos) List(context.Context, schedapi.ListInput) (schedapi.TodosOutput, error) {
	return schedapi.TodosOutput{Todos: f.todos}, nil
}

// fakeInbox answers the inbox's triage.
type fakeInbox struct {
	api.InboxService
	entries []api.Entry
}

func (f fakeInbox) Triage(context.Context, api.TriageInput) (api.TriageOutput, error) {
	return api.TriageOutput{Inbox: "sys-inbox.inbox", Entries: f.entries}, nil
}

var now = time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)

// humanTask is a human task's to-do as the human-task machine puts it.
func humanTask(id, kind, title, group string, assignee *cwmapi.Actor, due string) schedapi.Todo {
	ask := map[string]any{"kind": kind, "title": title, "about": "notes.RQ-001", "satisfies": []any{"notes.RQ-001/" + kind}}
	work := map[string]any{"doer": "person", "task": api.HumanTaskStrategy, api.WorkAsk: ask}
	if due != "" {
		work[api.WorkDue] = due
	}
	return schedapi.Todo{ID: id, Module: "notes", Target: "notes.RQ-001", Title: title, For: schedapi.ForPerson, Owner: api.Owner,
		State: schedapi.StateReady, Priority: "P2", Since: "2026-10-23T10:00:00Z", Group: group, Assignee: assignee, Work: work}
}

// world is « À faire »'s sources for the person jdoe: two holding lists -
// hl-cosmos, of which they are a member, and hl-other, of which they are
// not -, human tasks of every whose, the inbox's entries of every kind, and
// a contributed source (planification's place).
func world(t *testing.T) context.Context {
	t.Helper()
	ctx, cwm := cwmapp.New(t, inboxcontrib.Pack(""), cwmapp.SystemModules(t))
	mod := cwmapi.Object{ID: "sys-inbox", Type: cwmapi.SystemModuleType}
	mod.Name = "sys-inbox"
	cosmos := cwmapi.Object{ID: "sys-inbox.hl-cosmos", Type: api.HoldingListType, Parent: "sys-inbox"}
	cosmos.Name = "hl-cosmos"
	cosmos.Fields = map[string]any{"members": []any{
		map[string]any{"id": "jdoe", "kind": "person"}, map[string]any{"id": "marie", "kind": "person"},
		map[string]any{"id": "relecteur", "kind": "agent"},
	}}
	other := cwmapi.Object{ID: "sys-inbox.hl-other", Type: api.HoldingListType, Parent: "sys-inbox"}
	other.Name = "hl-other"
	other.Fields = map[string]any{"members": []any{map[string]any{"id": "paul", "kind": "person"}}}
	for _, o := range []cwmapi.Object{mod, cosmos, other} {
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "sys-inbox", Object: o}); err != nil {
			t.Fatalf("seed %s: %v", o.ID, err)
		}
	}
	me := &cwmapi.Actor{ID: "jdoe", Kind: "person"}
	marie := &cwmapi.Actor{ID: "marie", Kind: "person"}
	claimed := humanTask("notes.TD-claimed", "approve", "Review the as-is chapter", "sys-inbox.hl-cosmos", nil, "")
	claimed.State, claimed.Claim = schedapi.StateClaimed, &schedapi.Claim{By: *marie, Until: "2026-10-24T10:00:00Z"}
	answered := humanTask("notes.TD-answered", "approve", "Already answered", "", me, "")
	answered.State, answered.Result = schedapi.StateReported, &schedapi.Result{Status: "success", By: *me}
	agentOpen := humanTask("notes.TD-agent", "approve", "Review for any member", "sys-inbox.hl-cosmos", nil, "")
	agentOpen.For = schedapi.ForAgent
	todos := fakeTodos{todos: []schedapi.Todo{
		humanTask("notes.TD-test", "test-by-hand", "Test the export by hand", "sys-inbox.project", me, "2026-10-24T18:00:00Z"),
		humanTask("notes.TD-choose", "choose", "Which Confluence space?", "sys-inbox.hl-cosmos", nil, ""),
		humanTask("notes.TD-late", "approve", "Approve the release notes", "sys-inbox.hl-cosmos", nil, "2026-10-23T09:00:00Z"),
		humanTask("notes.TD-notmine", "approve", "Another list's question", "sys-inbox.hl-other", nil, ""),
		humanTask("notes.TD-marie", "approve", "Given to Marie", "sys-inbox.hl-cosmos", marie, ""),
		humanTask("notes.TD-open", "provide", "Which currency?", "planning", nil, ""),
		claimed, answered, agentOpen,
		{ID: "notes.TD-step", Module: "notes", Owner: "planification", For: schedapi.ForPerson, State: schedapi.StateReady},
	}}
	inbox := fakeInbox{entries: []api.Entry{
		{ID: "desk.AR-gate", Type: api.RequestType, Title: "Confirm: change log — Release 1.006", Status: api.StatusPending,
			RequestedBy: "planification:gate:ready-to-release/change-log@2026-10-23", Expects: api.ExpectsVerdict, About: "roadmap.RL-033", RequestedAt: "2026-10-23T08:00:00Z"},
		{ID: "desk.AR-ask", Type: api.RequestType, Title: "Which format?", Status: api.StatusPending, RequestedBy: "run-7", Expects: api.ExpectsText, RequestedAt: "2026-10-23T07:00:00Z"},
		{ID: "desk.AR-notice", Type: api.RequestType, Title: "Your demand became notes.RQ-002", Status: api.StatusPending,
			RequestedBy: "planification:notes.RQ-002", Expects: api.ExpectsNotice, About: "notes.RQ-002", RequestedAt: "2026-10-23T06:00:00Z"},
		{ID: "desk.AR-step", Type: api.RequestType, Title: "validate: review — notes.RQ-001", Status: api.StatusPending,
			RequestedBy: "planification:notes.RQ-001", Expects: api.ExpectsVerdict, About: "notes.RQ-001"},
		{ID: "desk.AR-orphan", Type: api.RequestType, Title: "validate: review — notes.RQ-009", Status: api.StatusPending,
			RequestedBy: "planification:notes.RQ-009", Expects: api.ExpectsVerdict, About: "notes.RQ-009"},
		{ID: "desk.AR-done", Type: api.RequestType, Title: "Answered", Status: api.StatusApproved, Expects: api.ExpectsVerdict},
	}}
	app := application.AppFromContext(ctx)
	application.PutService[schedapi.TodoService](app, "default", todos)
	application.PutService[api.InboxService](app, "default", inbox)

	src := api.WaitingSource{
		Name: "planification",
		Items: func(context.Context) ([]api.WaitingItem, error) {
			return []api.WaitingItem{
				{Key: "planification:validate:notes.TD-step", Kind: api.KindApprove, Title: "validate: review — notes.RQ-001",
					About: "notes.RQ-001", Todo: "notes.TD-step", Entry: "desk.AR-step", Since: "2026-10-22T10:00:00Z"},
				{Key: "planification:decide:notes.OD-001", Kind: api.KindDecide, Title: "Which store?", About: "notes.OD-001", Since: "2026-10-21T10:00:00Z"},
				{Key: "planification:validate:notes.IN-001", Kind: api.KindAccept, Title: "Accept the increment's wants", About: "notes.IN-001"},
			}, nil
		},
		Answers: func(e api.Entry) bool {
			return strings.HasPrefix(e.RequestedBy, "planification:") && !strings.HasPrefix(e.RequestedBy, "planification:gate:") && e.Expects != api.ExpectsNotice
		},
		Controls: func(_ context.Context, it api.WaitingItem, _ string) []render.Node {
			return []render.Node{{"kind": "text", "content": "control of " + it.Key}}
		},
	}
	sources = func(context.Context) []api.WaitingSource { return []api.WaitingSource{src} }
	profileOf = func(context.Context) (profileapi.Profile, error) {
		return profileapi.Profile{ID: "jdoe", FirstName: "Jane", Source: profileapi.SourceLocal}, nil
	}
	askKinds = func(context.Context) ([]api.AskKind, error) {
		return []api.AskKind{{Name: "approve", Label: "Validation", Order: 1}, {Name: "decide", Label: "Decision", Order: 2},
			{Name: "test-by-hand", Label: "Test by hand", Order: 3}, {Name: "form", Label: "Answer to give", Order: 4},
			{Name: "provide", Label: "Data to provide", Order: 5}, {Name: "choose", Label: "Choice", Order: 6},
			{Name: "accept", Label: "Acceptance", Order: 9}}, nil
	}
	clock = func() time.Time { return now }
	t.Cleanup(func() {
		sources = func(ctx context.Context) []api.WaitingSource {
			return application.UseExtensions[api.WaitingSource](ctx)
		}
		clock = time.Now
	})
	return ctx
}

func keys(items []api.WaitingItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Key)
	}
	return out
}

// TestEverythingWaitingOnAPersonIsInOnePlace: « À faire »
// lists every asker's question once - the human tasks given to the person,
// taken by them or on an open list as theirs, then those of each holding
// list they are a member of; the inbox's entries, a gate's confirmation and
// an ask-user question included; the contributed source's items, the entry a
// step filed carried by its item only and one no item carries left out -
// overdue first, then by kind; the notices apart, under Recent, never
// counted; and nothing that is another's or answered.
func TestEverythingWaitingOnAPersonIsInOnePlace(t *testing.T) {
	ctx := world(t)
	out, err := New().List(ctx, api.WaitingInput{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if out.Person != "jdoe" || out.Name != "Jane" {
		t.Errorf("whose = %q %q; want the profile's", out.Person, out.Name)
	}
	if len(out.Groups) != 2 || !out.Groups[0].Mine || out.Groups[1].List != "sys-inbox.hl-cosmos" {
		t.Fatalf("groups = %+v; want Mine then hl-cosmos", out.Groups)
	}
	mine := keys(out.Groups[0].Items)
	wantMine := []string{
		"planification:validate:notes.TD-step", // approve, by age
		"inbox:desk.AR-gate",                   // a gate's confirmation (R1)
		"planification:decide:notes.OD-001",
		"inbox:notes.TD-test", // test by hand, given to them
		"inbox:desk.AR-ask",
		"inbox:notes.TD-open", // an open list: theirs
		"planification:validate:notes.IN-001",
	}
	if strings.Join(mine, " ") != strings.Join(wantMine, " ") {
		t.Errorf("Mine = %v\nwant    %v", mine, wantMine)
	}
	list := keys(out.Groups[1].Items)
	wantList := []string{"inbox:notes.TD-late", "inbox:notes.TD-agent", "inbox:notes.TD-claimed", "inbox:notes.TD-choose"}
	if strings.Join(list, " ") != strings.Join(wantList, " ") {
		t.Errorf("hl-cosmos = %v\nwant       %v (overdue first, then by kind)", list, wantList)
	}
	if g := out.Groups[1]; g.Title != "hl-cosmos" || len(g.Members) != 3 {
		t.Errorf("the list = %q members %+v; want its name and its three members", g.Title, g.Members)
	}
	late := out.Groups[1].Items[0]
	if !late.Overdue || late.Label != "Validation" || late.Due == "" {
		t.Errorf("the overdue one = %+v; want overdue with its due date and label", late)
	}
	if c := out.Groups[1].Items[2]; c.ClaimedBy == nil || c.ClaimedBy.ID != "marie" {
		t.Errorf("the claimed one = %+v; want taken by marie", c)
	}
	if gate := out.Groups[0].Items[1]; gate.Kind != api.KindApprove || gate.About != "roadmap.RL-033" || gate.Entry != "desk.AR-gate" {
		t.Errorf("the gate's confirmation = %+v; want an approval about what waits on it", gate)
	}
	if out.Count != 11 || out.Overdue != 1 {
		t.Errorf("count = %d overdue %d; want 11 and 1", out.Count, out.Overdue)
	}
	if len(out.Recent) != 1 || out.Recent[0].Entry != "desk.AR-notice" {
		t.Errorf("recent = %+v; want the notice alone", out.Recent)
	}
	counts := map[string]int{}
	var order []string
	for _, k := range out.Kinds {
		counts[k.Kind] = k.Count
		order = append(order, k.Kind)
	}
	if strings.Join(order, " ") != "approve decide test-by-hand form provide choose accept" ||
		counts["approve"] != 5 || counts["choose"] != 1 || counts["accept"] != 1 {
		t.Errorf("kinds = %+v; want each kind counted, in the kinds' order", out.Kinds)
	}
	seen := map[string]int{}
	for _, g := range out.Groups {
		for _, it := range g.Items {
			seen[it.Title]++
		}
	}
	for title, n := range seen {
		if n != 1 {
			t.Errorf("%q shows %d times; want once", title, n)
		}
	}
	for _, gone := range []string{"Another list's question", "Given to Marie", "Already answered", "validate: review — notes.RQ-009", "Answered"} {
		if seen[gone] != 0 {
			t.Errorf("%q is listed; it is not theirs, or answered, or no item carries it", gone)
		}
	}

	if got, err := New().Count(ctx, api.WaitingInput{Kind: api.KindChoose}); err != nil || got.Count != 11 {
		t.Errorf("count = %+v, %v; want 11, whatever the filter", got, err)
	}
	only, err := New().List(ctx, api.WaitingInput{Kind: KindOverdue})
	if err != nil || out.Count != only.Count || len(only.Groups[0].Items) != 0 || keys(only.Groups[1].Items)[0] != "inbox:notes.TD-late" || len(only.Groups[1].Items) != 1 {
		t.Errorf("overdue only = %+v, %v; want the late one alone, the counts whole", only.Groups, err)
	}
}

// TestTheConversationsRunningAreListedNotCounted: « À faire »
// lists the chats running in the person's sessions - the chat panels not
// ended, those waiting on their reply first, then the last moved -, each
// opening its chat; no other panel, none ended, and none counted; the
// conversation filter shows them alone, another kind's hides them.
func TestTheConversationsRunningAreListedNotCounted(t *testing.T) {
	ctx := world(t)
	at := func(s string) time.Time { v, _ := time.Parse(time.RFC3339, s); return v }
	defer func(f func(context.Context) ([]panelSummary, error)) { panels = f }(panels)
	panels = func(context.Context) ([]panelSummary, error) {
		return []panelSummary{
			{Panel: "dp-running", Type: api.ChatPanelType, State: "open", Title: "agent architecte", Updated: at("2026-10-24T08:30:00Z")},
			{Panel: "dp-waiting", Type: api.ChatPanelType, State: "asking", Waiting: true, Title: "session cosmos", Updated: at("2026-10-24T07:00:00Z")},
			{Panel: "dp-ended", Type: api.ChatPanelType, State: "complete", Title: "done"},
			{Panel: "dp-work", Type: "ktw-agents:/panel-type/progress", State: "open", Title: "an import"},
		}, nil
	}
	// A chat waiting on the person says what it asks (its last ask), as the
	// mock-up quotes its last word; only a waiting one is read.
	defer func(f func(context.Context, string) (string, error)) { asked = f }(asked)
	var read []string
	asked = func(_ context.Context, id string) (string, error) {
		read = append(read, id)
		return "Before publishing I need one answer\nWhich Confluence space?", nil
	}
	out, err := New().List(ctx, api.WaitingInput{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if c := out.Conversations; len(c) == 2 && (c[0].Last != "Before publishing I need one answer" || c[1].Last != "") {
		t.Errorf("last words = %q, %q; want the waiting chat's ask (its first line), none for the running one", c[0].Last, c[1].Last)
	}
	if len(read) == 0 || read[0] != "dp-waiting" {
		t.Errorf("snapshots read = %v; want the waiting chat's", read)
	}
	for _, id := range read {
		if id != "dp-waiting" {
			t.Errorf("read the snapshot of %s; want only a chat waiting on the person read", id)
		}
	}
	if len(out.Conversations) != 2 || out.Conversations[0].Panel != "dp-waiting" || !out.Conversations[0].Waiting || out.Conversations[1].Panel != "dp-running" {
		t.Fatalf("conversations = %+v; want the waiting chat first, then the running one", out.Conversations)
	}
	if out.Count != 11 {
		t.Errorf("count = %d; want 11 - a conversation is not counted", out.Count)
	}
	only, err := New().List(ctx, api.WaitingInput{Kind: api.KindConversation})
	if err != nil || len(only.Conversations) != 2 || len(only.Groups[0].Items)+len(only.Groups[1].Items) != 0 || only.Count != 11 {
		t.Errorf("conversations only = %+v, %v; want them alone, the count whole", only, err)
	}
	if other, _ := New().List(ctx, api.WaitingInput{Kind: api.KindApprove}); len(other.Conversations) != 0 {
		t.Errorf("a kind's filter still shows %d conversations", len(other.Conversations))
	}
}

// TestAWaitingQuestionCarriesItsOptionsAndForm: « À faire »
// lists a choose question with its options and a form with its definition,
// for its card to draw them.
func TestAWaitingQuestionCarriesItsOptionsAndForm(t *testing.T) {
	ask := map[string]any{"kind": "choose", "title": "Which store?", "options": []any{"files", "sqlite"}, "form_def": "fields: []\n"}
	td := schedapi.Todo{ID: "m.TD-1", Owner: api.Owner, Work: map[string]any{"task": api.HumanTaskStrategy, api.WorkAsk: ask}}
	it, ok := humanTaskItem(td, time.Now())
	if !ok {
		t.Fatal("the human task reads as no question")
	}
	if len(it.Options) != 2 || it.Options[1] != "sqlite" || it.FormDef != "fields: []\n" {
		t.Errorf("item options %v, form %q; want the ask's", it.Options, it.FormDef)
	}
}

// pathForm is a form question's FormDefinition: a required path, a format to
// choose.
const pathForm = "screen:\n  title: Where to\nfields:\n  - key: path\n    type: string\n    label: Path\n    required: true\n" +
	"  - key: format\n    type: enum\n    options:\n      - {value: docx, label: Word}\n      - {value: pdf, label: PDF}\n"

// cardOf is a question's answer area as the host receives it - the nodes the
// inbox answers it with, which « À faire »'s card draws
// -, and its text.
func cardOf(t *testing.T, it api.WaitingItem, lang string) (map[string]any, string) {
	t.Helper()
	raw, _ := json.Marshal(render.Node{"kind": "vbox", "children": controlsOf(context.Background(), it, lang)})
	var n map[string]any
	_ = json.Unmarshal(raw, &n)
	return n, string(raw)
}

// byID is the node of a tree with that id, its actions' modals included.
func byID(v any, id string) map[string]any {
	switch n := v.(type) {
	case []any:
		for _, c := range n {
			if got := byID(c, id); got != nil {
				return got
			}
		}
	case map[string]any:
		if n["id"] == id {
			if _, isNode := n["kind"]; isNode {
				return n
			}
		}
		for _, c := range n {
			if got := byID(c, id); got != nil {
				return got
			}
		}
	}
	return nil
}

// fieldsOf are the root fields of a form node's inline schema.
func fieldsOf(form map[string]any) map[string]any {
	s, _ := form["schema"].(map[string]any)
	types, _ := s["types"].(map[string]any)
	root, _ := types[s["root"].(string)].(map[string]any)
	f, _ := root["fields"].(map[string]any)
	return f
}

func asJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

// TestTheHumanTaskCardAnswersByItsKind: a human task's card is
// the mock-up's - the card carries its key as its id, its kind first - and
// its answer area is its kind's: a verdict two buttons, « Approve » (with
// « (passkey) » when that proof is asked) reporting approved and « Reject »
// asking why in a modal before reporting rejected, « Take it » beside them
// on a holding list's question no one took; a test by hand « All good: I
// validate » or « Something failed »; a choice its options in a drop-down
// and « Choose » (a decision also says why); a form the fields its
// FormDefinition declares, its answer the form's data (one it cannot draw
// shown and answered in words); a permission its kind's answer data-type,
// its rule one of the rules proposed, the decision the answer and the whole
// its data; data to provide a text.
func TestTheHumanTaskCardAnswersByItsKind(t *testing.T) {
	base := api.WaitingItem{Key: "inbox:m.TD-1", Source: SourceInbox, Title: "The question", Todo: "m.TD-1"}

	// A verdict on a holding list, a passkey asked.
	it := base
	it.Kind, it.List, it.Proof = api.KindApprove, "sys-inbox.hl-cosmos", "passkey"
	n, raw := cardOf(t, it, "en")
	yes, no, take := byID(n, "approve:m.TD-1"), byID(n, "reject:m.TD-1"), byID(n, "take:m.TD-1")
	if yes == nil || no == nil || take == nil {
		t.Fatalf("a verdict on a holding list lacks Approve, Reject or Take it:\n%s", raw)
	}
	if yes["label"] != "Approve (passkey)" || !strings.Contains(asJSON(yes["action"]), `"verdict":"approved"`) {
		t.Errorf("Approve: %s", asJSON(yes))
	}
	why := byID(no, "why:m.TD-1")
	if why == nil || why["kind"] != "form" || !strings.Contains(asJSON(why["submit"]), `"verdict":"rejected"`) ||
		!strings.Contains(asJSON(why["submit"]), `"summary":"${value.note}"`) {
		t.Errorf("Reject does not ask why before reporting rejected: %s", asJSON(no))
	}
	if !strings.Contains(asJSON(take["action"]), todosPullURI) {
		t.Errorf("Take it does not pull its to-do: %s", asJSON(take))
	}
	it.List, it.Proof = "", ""
	if n, _ = cardOf(t, it, "fr"); byID(n, "take:m.TD-1") != nil || byID(n, "approve:m.TD-1")["label"] != "Approuver" {
		t.Errorf("one's own verdict offers Take it, or Approve is not in French: %s", asJSON(n))
	}

	// A test by hand.
	it = base
	it.Kind = api.KindTestByHand
	n, raw = cardOf(t, it, "en")
	if byID(n, "all_good:m.TD-1")["label"] != "All good: I validate" || byID(n, "failed:m.TD-1")["label"] != "Something failed" {
		t.Errorf("a test by hand is not « All good: I validate » / « Something failed »:\n%s", raw)
	}

	// A choice: its options, « Choose », no note; a decision says why.
	it = base
	it.Kind, it.Options = api.KindChoose, []string{"ARCHI", "ARCHI-TEST"}
	n, _ = cardOf(t, it, "en")
	form := byID(n, "answer:m.TD-1")
	f := fieldsOf(form)
	if asJSON(f["choice"].(map[string]any)["enum"]) != `["ARCHI","ARCHI-TEST"]` || f["note"] != nil ||
		asJSON(form["data"]) != `{"choice":"ARCHI"}` || !strings.Contains(asJSON(form["submit"]), `"label":"Choose"`) ||
		!strings.Contains(asJSON(form["submit"]), `"answer":"${value.choice}"`) {
		t.Errorf("a choice is not its options and « Choose »: %s", asJSON(form))
	}
	it.Kind = api.KindDecide
	n, _ = cardOf(t, it, "en")
	if form = byID(n, "answer:m.TD-1"); fieldsOf(form)["note"] == nil || !strings.Contains(asJSON(form["submit"]), `"details":"${value.note}"`) {
		t.Errorf("a decision does not say why: %s", asJSON(form))
	}

	// A form: its fields; its answer the form's data.
	it = base
	it.Kind, it.FormDef = api.KindForm, pathForm
	n, _ = cardOf(t, it, "en")
	form = byID(n, "answer:m.TD-1")
	f = fieldsOf(form)
	path, _ := f["path"].(map[string]any)
	format, _ := f["format"].(map[string]any)
	if path == nil || path["label"] != "Path" || path["optional"] != nil || format == nil || asJSON(format["enum"]) != `["docx","pdf"]` ||
		format["optional"] != true || !strings.Contains(asJSON(form["submit"]), `"more":{"data":"${value}"}`) {
		t.Errorf("a form question does not draw its form: %s", asJSON(form))
	}
	it.FormDef = "fields:\n  - name: path\n"
	n, raw = cardOf(t, it, "en")
	if !strings.Contains(raw, "The form asked:") || !strings.Contains(raw, "name: path") ||
		!strings.Contains(asJSON(byID(n, "answer:m.TD-1")["submit"]), `"answer":"${value.note}"`) {
		t.Errorf("a form it cannot draw is not shown and answered in words:\n%s", raw)
	}

	// A permission: its kind's answer data-type, the proposed rules.
	it = base
	it.Kind, it.Options = api.KindPermission, []string{"git push *", "git *"}
	n, _ = cardOf(t, it, "en")
	form = byID(n, "answer:m.TD-1")
	f = fieldsOf(form)
	published, err := os.ReadFile("../../contrib/embed/cwm/data-types/permission-answer.dsl.yaml")
	if err != nil {
		t.Fatalf("the permission kind's answer data-type: %v", err)
	}
	decision, _ := f["decision"].(map[string]any)
	enum, _ := decision["enum"].([]any)
	if len(f) != 2 || len(enum) != 4 || f["rule"] == nil {
		t.Fatalf("a permission is not drawn from its kind's answer data-type: %s", asJSON(form))
	}
	for _, d := range enum {
		if !strings.Contains(string(published), "- "+d.(string)+"\n") {
			t.Errorf("the card offers %v, which the published data-type does not", d)
		}
	}
	if asJSON(f["rule"].(map[string]any)["enum"]) != `["git push *","git *"]` || asJSON(form["data"]) != `{"decision":"allow-once","rule":"git push *"}` {
		t.Errorf("a permission's rule is not one of the rules proposed: %s", asJSON(form))
	}
	for _, want := range []string{`"answer":"${value.decision}"`, `"more":"${value}"`} {
		if !strings.Contains(asJSON(form["submit"]), want) {
			t.Errorf("a permission's answer lacks %s: %s", want, asJSON(form["submit"]))
		}
	}

	// Data to provide: a text.
	it = base
	it.Kind = api.KindProvide
	n, _ = cardOf(t, it, "en")
	if form = byID(n, "answer:m.TD-1"); form == nil || !strings.Contains(asJSON(form["submit"]), `"answer":"${value.note}"`) {
		t.Errorf("data to provide is not answered in words: %s", asJSON(n))
	}
}

// TestEveryCardsAnswerFormArrivesResolved (F-027): every card
// of « À faire » - a verdict's why, a choice, a permission, a form, an answer
// in words - answers through a form
// whose ui block the host lays over its schema; the cards live in the list's
// elements (`nodeField`), so the host's resolution must reach them, or each
// card shows « Cannot render form node ».
func TestEveryCardsAnswerFormArrivesResolved(t *testing.T) {
	ctx := world(t)
	out0, err := New().List(ctx, api.WaitingInput{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for g := range out0.Groups {
		for i, it := range out0.Groups[g].Items {
			if it.Kind == api.KindChoose {
				out0.Groups[g].Items[i].Options = []string{"files", "sqlite"} // a choice offering its options
			}
		}
	}
	// A permission with the rules the tool proposes and a form question with its form.
	out0.Groups[0].Items = append(out0.Groups[0].Items,
		api.WaitingItem{Key: "inbox:m.TD-perm", Source: SourceInbox, Kind: api.KindPermission, Title: "May it run git push?", Todo: "m.TD-perm",
			Options: []string{"git push *", "git *"}},
		api.WaitingItem{Key: "inbox:m.TD-form", Source: SourceInbox, Kind: api.KindForm, Title: "Give the path", Todo: "m.TD-form", FormDef: pathForm})
	var areas []render.Node
	for _, g := range out0.Groups {
		for _, it := range g.Items {
			areas = append(areas, render.Node{"kind": "vbox", "children": controlsOf(ctx, it, "en")})
		}
	}
	raw, _ := json.Marshal(render.Node{"kind": "vbox", "children": areas})
	var tree any
	_ = json.Unmarshal(raw, &tree)
	forms := func(v any) (all, unresolved []string) {
		var walk func(v any)
		walk = func(v any) {
			switch n := v.(type) {
			case []any:
				for _, c := range n {
					walk(c)
				}
			case map[string]any:
				if n["kind"] == "form" {
					b, _ := json.Marshal(n["submit"])
					all = append(all, string(b))
					if _, has := n["ui"]; has {
						unresolved = append(unresolved, string(b))
					}
				}
				for _, c := range n {
					walk(c)
				}
			}
		}
		walk(v)
		return
	}
	before, _ := forms(tree)
	for _, want := range []string{`"verdict":"rejected"`, `"decision":"rejected"`, `"${value.choice}"`, `"${value.note}"`,
		`"answer":"${value.decision}"`, `"data":"${value}"`} {
		if !strings.Contains(strings.Join(before, "\n"), want) {
			t.Errorf("no card answers with %s; forms:\n%s", want, strings.Join(before, "\n"))
		}
	}
	if !render.HasComponents(tree) {
		t.Fatal("the host would skip resolving « À faire »: HasComponents sees no form in its cards")
	}
	out, err := render.ResolveComponents(tree, render.ComponentResolver{Fetch: func(ref render.SchemaRef, _, _ string) (map[string]any, error) {
		t.Errorf("a card's form names its schema (%s): cards answer through inline schemas", ref)
		return nil, nil
	}})
	if err != nil {
		t.Fatalf("ResolveComponents: %v", err)
	}
	if _, left := forms(out); len(left) > 0 {
		t.Errorf("%d of %d card forms still carry their ui block:\n%s", len(left), len(before), strings.Join(left, "\n"))
	}
}

// Asked with answers, EVERY QUESTION CARRIES ITS ANSWER NODES
// : the inbox's own and a contributed source's - its
// Controls -, so a screen that is not the inbox's draws them all.
func TestAskedWithAnswersEverySourcesQuestionCarriesItsControls(t *testing.T) {
	ctx := world(t)
	out, err := New().List(ctx, api.WaitingInput{Answers: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	seen := 0
	for _, g := range out.Groups {
		for _, it := range g.Items {
			if it.Source != "planification" {
				continue
			}
			seen++
			if len(it.Answer) != 1 || it.Answer[0]["content"] != "control of "+it.Key {
				t.Errorf("%s: answer = %v; want its source's controls", it.Key, it.Answer)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no contributed source's question listed")
	}
}
