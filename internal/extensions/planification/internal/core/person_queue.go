// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	challengesapi "github.com/katyoti/katyoti-fwk/internal/extensions/challenges/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	decisionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE PERSON'S QUEUE — « À faire »: everything waiting on the person, each
// item asking exactly ONE action of the closed set (api.PersonActions). It is
// read from four sources, never stored:
//   - the plans: a person's step ready (validate, challenge → validate;
//     decide → decide; provide → answer), a requester asked a question
//     (answer), a step failed or a request blocked (unblock), a deliverable
//     no check verifies that its request waits on (test) - only what a
//     person alone moves: an agent's failed step under the cap is agent work,
//     a GUI walk an open batch lists waits on that batch
//   - the open decisions ready to decide — options and a proposal —, planned
//     or not (decide); one not prepared is an agent's to prepare first;
//   - the Inbox: an entry nothing above carries (answer or validate by what
//     it expects; anything but an approval request is a notice).
// One step, one action: a validation whose request has deliverables only a
// hand test verifies is two items or more — each test first, the validation
// waiting on them (WaitsOn) until each passed.

// proposalOf is what the queue reads of a decision: its options and proposal.
type proposalOf struct {
	Options   []string                        `json:"options,omitempty"`
	Proposal  *decisionsapi.Proposal          `json:"proposal,omitempty"`
	Challenge *decisionsapi.ProposalChallenge `json:"challenge,omitempty"`
	// Owner is whose the decision is: an agent's is taken by an agent, never
	// asked of the person.
	Owner string `json:"owner,omitempty"`
}

// PersonQueue answers the person's queue, scoped to a module or an object.
func (v *Views) PersonQueue(ctx context.Context, in api.PersonQueueInput) (api.PersonQueueOutput, error) {
	base, err := readQueueBase(withReadCache(ctx), in.Language)
	if err != nil {
		return api.PersonQueueOutput{}, err
	}
	return scopedQueue(base, in), nil
}

// queueBase is the whole queue before its scope: every item waiting on the
// person, the inbox's list (whose queue, its counts by kind) and why the
// inbox's questions could not be read. Read only, once computed: a count
// joining a computation shares it (sharedQueue).
type queueBase struct {
	items  []api.PersonItem
	list   inboxapi.WaitingOutput
	unread string
}

// readQueueBase computes the whole queue: planification's items, then the
// inbox's that none of them carries, in the queue's order.
func readQueueBase(ctx context.Context, language string) (queueBase, error) {
	queueComputations.Add(1)
	items, err := personQueue(ctx)
	if err != nil {
		return queueBase{}, err
	}
	// Every waiting item is in it: the inbox's human
	// tasks and the conversations too.
	carried := map[string]bool{}
	for _, it := range items {
		if it.Ask != "" {
			carried[it.Ask] = true
		}
		if it.Object != "" {
			carried[it.Object] = true
		}
	}
	base := queueBase{}
	more, list, err := inboxQueueItems(ctx, language, carried)
	if err != nil {
		base.unread = err.Error()
	}
	base.list = list
	if len(more) > 0 {
		if holds, err := holdsOf(ctx); err == nil {
			for i := range more {
				if h, ok := holds[more[i].Key]; ok {
					more[i].Held = &h
				}
			}
		}
		items = append(items, more...)
		sortQueue(items)
	}
	base.items = items
	return base, nil
}

// scopedQueue is the queue of the scope asked (a module, an object; every
// item when none), its counts, and the kind chosen. It never changes base.
func scopedQueue(base queueBase, in api.PersonQueueInput) api.PersonQueueOutput {
	scope := in.Target
	if scope == "" {
		scope = in.ID
	}
	waiting := base.list
	out := api.PersonQueueOutput{Items: []api.PersonItem{}, ByAction: map[string]int{}, Unread: base.unread}
	out.Person, out.PersonName, out.Kinds, out.Overdue = waiting.Person, waiting.Name, slices.Clone(waiting.Kinds), waiting.Overdue
	if len(waiting.Conversations) > 0 {
		out.Kinds = append(out.Kinds, inboxapi.KindCount{Kind: api.ActionConversation, Label: ql(in.Language, api.ActionConversation), Count: len(waiting.Conversations)})
	}
	for _, it := range base.items {
		if !inScope(it, scope) {
			continue
		}
		out.Items = append(out.Items, it)
		if it.Held != nil {
			out.Held++
			continue
		}
		out.ByAction[it.Action]++
		if it.Action != api.ActionNotice && it.Action != api.ActionConversation {
			out.Count++
		}
	}
	// A kind chosen (its chip): only its items, every count kept.
	if in.Kind != "" {
		out.Items = ofKind(out.Items, in.Kind)
	}
	return out
}

// ObjectQueueItems are the items of an object's « À faire » - exactly
// PersonQueue's for that target - read from what can be of it only
// : the demands whose work goes to it, and the inbox's
// own questions without planification's again. A release's pane waits on it;
// the counts of the whole queue are not read.
func ObjectQueueItems(ctx context.Context, id, language string) ([]api.PersonItem, error) {
	ctx = inboxapi.WithoutSources(withDemandScope(withReadCache(ctx), id), WaitingSourceName)
	q, err := NewViews().PersonQueue(ctx, api.PersonQueueInput{Target: id, Language: language})
	return q.Items, err
}

// PersonQueueCount answers how many items wait on the person: the rail's badge.
// A count asked while the queue is being computed - for the page or another
// count - takes that computation's answer.
func (v *Views) PersonQueueCount(ctx context.Context, in api.PersonQueueInput) (api.PersonQueueCountOutput, error) {
	q, err := sharedPersonQueue(ctx, in, true)
	if err != nil {
		return api.PersonQueueCountOutput{}, err
	}
	return api.PersonQueueCountOutput{Count: q.Count}, nil
}

// inScope keeps an item of the scope: every one when empty, a module's (an id
// with no dot), or an object's — the item is about it or moves it on.
func inScope(it api.PersonItem, scope string) bool {
	switch {
	case scope == "":
		return true
	case !strings.Contains(scope, "."):
		// A module: bare, any project's of that name; <key>:<module>, that
		// project's only.
		if key := keyOf(scope); key != "" {
			return it.Project == key && it.Module == bareID(scope)
		}
		return it.Module == scope
	default:
		return it.Object == scope || it.Request == scope || it.Increment == scope || it.Release == scope ||
			slices.ContainsFunc(it.Wants, func(w api.Target) bool { return w.ID == scope })
	}
}

// placeItems gives each item the increment and the release of the work it
// moves on - its request, else what it is about - read as the to-dos' labels
// are (releaseOfWork), with their titles.
func placeItems(ctx context.Context, cwm cwmapi.CWMService, items []api.PersonItem) {
	titles := map[string]string{}
	title := func(id string) string {
		if id == "" {
			return ""
		}
		if t, ok := titles[id]; ok {
			return t
		}
		titles[id] = titleOfID(ctx, cwm, id)
		return titles[id]
	}
	for i := range items {
		about := items[i].Request
		if about == "" {
			about = items[i].Object
		}
		o, _, err := loadAny(ctx, cwm, about)
		if err != nil {
			continue
		}
		inc, rel := releaseOfWork(ctx, cwm, o)
		// The increment and the release of the project holding it.
		inc, rel = inKeyOf(about, inc), inKeyOf(about, rel)
		items[i].Increment, items[i].IncrementTitle = inc, title(inc)
		items[i].Release, items[i].ReleaseTitle = rel, title(rel)
	}
}

// personQueue reads every item waiting on the person, in the queue's order:
// by action, then oldest first; the items on hold last, in the order they
// were held.
func personQueue(ctx context.Context) ([]api.PersonItem, error) {
	return personItems(ctx, true)
}

// personItems reads the items waiting on the person: planification's own - its
// plans' steps, the decisions, the wants to accept or confirm - and, with
// withInbox, the Inbox's entries nothing else carries (« À faire » reads
// those itself).
func personItems(ctx context.Context, withInbox bool) ([]api.PersonItem, error) {
	open, err := New().Demands(ctx, api.DemandsInput{})
	if err != nil {
		return nil, err
	}
	var demands []api.Demand
	var walk func(ds []api.Demand)
	walk = func(ds []api.Demand) {
		for _, d := range ds {
			demands = append(demands, d)
			walk(d.Parts)
		}
	}
	walk(open.Demands)
	all, err := todosOf(ctx, "")
	if err != nil {
		return nil, err
	}
	// Several projects served, the demands are keyed by the qualified id
	// (Demands): the to-dos join them so.
	refs := newProjectRefs(ctx, slices.ContainsFunc(demands, func(d api.Demand) bool { return keyOf(d.ID) != "" }))
	byRequest := map[string][]api.WorkTodo{}
	for _, t := range all {
		r := refs.todo(t)
		byRequest[r] = append(byRequest[r], t)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return nil, err
	}
	// What holds a stopped step is the one reader's to say.
	hold, err := newHoldReader(ctx, cwm)
	if err != nil {
		return nil, err
	}

	var items []api.PersonItem
	carried := map[string]bool{} // inbox entries an item already carries
	keys := map[string]bool{}
	add := func(it api.PersonItem) {
		if keys[it.Key] {
			return
		}
		keys[it.Key] = true
		if it.Ask != "" {
			carried[it.Ask] = true
		}
		items = append(items, it)
	}
	questioned := openQuestions(ctx, refs)
	for _, d := range demands {
		if planEnded(d.State) || d.State == api.PlanSplit {
			continue
		}
		for _, it := range requestItems(ctx, cwm, hold, d, byRequest[d.ID], questioned[d.ID]) {
			add(it)
		}
	}
	for _, it := range decisionItems(ctx, cwm) {
		add(it)
	}
	for _, it := range wantItems(ctx, cwm) {
		add(it)
	}
	for _, it := range confirmItems(ctx, cwm) {
		add(it)
	}
	if withInbox {
		for _, it := range inboxItems(ctx, carried) {
			add(it)
		}
	}

	placeItems(ctx, cwm, items)

	holds, err := holdsOf(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if h, ok := holds[items[i].Key]; ok {
			items[i].Held, items[i].HoldIsBlock = &h, false
		}
	}

	sortQueue(items)
	return items, nil
}

// sortQueue orders the queue: by action - the conversations after the
// notices -, then oldest first; the items on hold last, in the order they
// were held.
func sortQueue(items []api.PersonItem) {
	rank := map[string]int{}
	for i, a := range api.PersonActions {
		rank[a] = i
	}
	rank[api.ActionNotice] = len(api.PersonActions)
	rank[api.ActionConversation] = len(api.PersonActions) + 1
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if (a.Held != nil) != (b.Held != nil) {
			return b.Held != nil
		}
		if a.Held != nil && a.Held.At != b.Held.At {
			return a.Held.At < b.Held.At
		}
		if rank[a.Action] != rank[b.Action] {
			return rank[a.Action] < rank[b.Action]
		}
		// The conversations keep the inbox's order: those waiting first.
		if a.Action == api.ActionConversation && b.Action == api.ActionConversation {
			return false
		}
		if a.Since != b.Since {
			return a.Since < b.Since
		}
		return a.Key < b.Key
	})
}

// requestItems is what one open request waits on the person for; questioned
// says an intake question of the request is still open (a human task).
func requestItems(ctx context.Context, cwm cwmapi.CWMService, hold *holdReader, d api.Demand, todos []api.WorkTodo, questioned bool) []api.PersonItem {
	sort.SliceStable(todos, func(i, j int) bool { return todos[i].Position < todos[j].Position })
	proven := provenOf(d)
	base := func(action, key string) api.PersonItem {
		return api.PersonItem{Key: action + ":" + key, Action: action, Object: d.ID, Request: d.ID, Module: bareID(d.Module),
			Project: keyOf(d.ID), Title: d.Title, Proven: proven, Since: d.FiledAt}
	}
	var out []api.PersonItem
	failedOne := false
	// A question to its requester still waiting: the
	// person answers it - one card - and the step blocked on it is not shown
	// as a failure to unblock. Asked as a human task
	// or in an inbox entry asked before.
	asking := questioned || len(d.AskedIn) > 0 && (planIdle(d) || askPending(ctx, d.AskedIn[len(d.AskedIn)-1]))
	for _, t := range todos {
		stop, _ := stopOf(t)
		if stop == "" || asking && stop == schedapi.StatusBlocked {
			continue
		}
		failedOne = true
		// The one reader says who moves it: a step
		// held by an outside party, by an agent or by other work is not the
		// person's - that work's root shows where it is the person's.
		if hold == nil || !hold.onThePerson(ctx, cwmapi.Object{ID: d.ID, Name: d.Title}, t) {
			continue
		}
		verb, summary := stopVerb(t)
		it := base(api.ActionUnblock, t.ID)
		it.Todo, it.Since = t.ID, t.Since
		it.Asks = fmt.Sprintf("Its step %s %s: %s. Try it again once the cause is fixed, or drop the request.", t.Packet.Step, verb, summary)
		out = append(out, it)
	}
	for _, t := range todos {
		if t.For != schedapi.ForPerson || t.State != schedapi.StateReady {
			continue
		}
		out = append(out, personStepItems(ctx, cwm, d, t, base)...)
	}
	// An inbox entry's question is carried here; a human task's is its own
	// card, from the inbox's list.
	if asking && len(d.AskedIn) > 0 && !questioned {
		it := base(api.ActionAnswer, d.ID)
		it.Ask, it.Expects = d.AskedIn[len(d.AskedIn)-1], inboxapi.ExpectsText
		it.Asks = "Its requester is asked a question: answer it."
		out = append(out, it)
	}
	if d.State == api.PlanBlocked && !failedOne && !asking {
		why := d.Why
		if why == "" && d.Plan != nil {
			why = d.Plan.Note
		}
		refused := d.Plan != nil && endRefused(*d.Plan)
		if e, own := ownHold(d.Plan); own {
			// The person's own block, with their reason: their hold - under
			// On hold, out of the count - taken back by resuming the plan
			//
			it := base(api.ActionUnblock, d.ID)
			it.Held = &api.Hold{Key: it.Key, Object: d.ID, Reason: why, By: e.By, At: e.At}
			it.HoldIsBlock = true
			it.Asks = "You put it on hold: " + why + " — resume it once that is settled, or drop it."
			out = append(out, it)
		} else if refused && hold != nil && endWaitsOn(ctx, cwm, hold, d.ID) != "" {
			// Its end waits on open work linked to it: that work moves it,
			// shown where it is the person's.
		} else if !refused {
			it := base(api.ActionUnblock, d.ID)
			it.Resume = true
			it.Asks = "It is blocked: " + why + " — resume it once that is settled, or drop it."
			out = append(out, it)
		} else {
			before, batched := len(out), false
			for _, del := range d.Deliverables {
				switch {
				case del.Verified:
				case inGUIBatch(ctx, cwm, hold, d.ID, del):
					// Its GUI walk waits on the batch that runs it, not on
					// the person: shown on the batch
					// request and the increment's Blockers tab.
					batched = true
				case strings.TrimSpace(del.Check) == "":
					out = append(out, testItem(base, del))
				default:
					it := base(api.ActionUnblock, d.ID+"#"+del.Title)
					it.Checks, it.Deliverable = true, del.Title
					it.Asks = fmt.Sprintf("“%s”: its check has not passed — run the checks once it is fixed, or revise it.", del.Title)
					out = append(out, it)
				}
			}
			// Its end refused for another cause than a deliverable (what it
			// is about cannot move to done): unblock it all the same - the
			// deck's Needs you lists it, To do must too.
			// ... unless its cause is gone: a person already moved it done
			// (confirmed in its Lifecycle tab; EndConfirmed ends the plan) -
			// or it is a built screen feature waiting only for its GUI test,
			// which the batch runs: nothing for the person to unblock before
			// it.
			if len(out) == before && !batched && !confirmedDone(ctx, cwm, d.ID, why) && !awaitsItsGUITest(ctx, cwm, d.ID) {
				it := base(api.ActionUnblock, d.ID)
				it.Resume = true
				it.Asks = "Its end was refused: " + why + " — settle it (confirm what it is about done in its Lifecycle tab), then resume it, or drop it."
				out = append(out, it)
			}
		}
	}
	return out
}

// endWaitsOn is the open work linked to a request blocked by its refused
// end, read by the one reader (holdReader.endWaitsOn); "" when none.
func endWaitsOn(ctx context.Context, cwm cwmapi.CWMService, hold *holdReader, id string) string {
	if cwm == nil {
		return ""
	}
	o, err := loadObject(ctx, cwm, id)
	if err != nil {
		return ""
	}
	return hold.endWaitsOn(ctx, o)
}

// inGUIBatch says a deliverable not verified is proved by live GUI walks
// that an open GUI batch request other than its own lists, each not yet
// verified there: it waits on that batch.
func inGUIBatch(ctx context.Context, cwm cwmapi.CWMService, hold *holdReader, self string, del api.Deliverable) bool {
	scripts := deliverableScripts(del)
	if len(scripts) == 0 || cwm == nil {
		return false
	}
	var batches []cwmapi.Object
	if hold != nil {
		batches = hold.guiBatches(ctx)
	} else {
		batches = guiBatchRequests(ctx, cwm)
	}
	for _, s := range scripts {
		if batchListing(batches, self, s) == "" {
			return false
		}
	}
	return true
}

// awaitsItsGUITest says the plan is about a built screen feature whose only
// missing proof is its GUI test (builtAwaitingGUI).
func awaitsItsGUITest(ctx context.Context, cwm cwmapi.CWMService, id string) bool {
	if cwm == nil {
		return false
	}
	o, err := loadObject(ctx, cwm, id)
	return err == nil && builtAwaitingGUI(ctx, cwm, o)
}

// confirmedDone says a plan blocked by its refused end is about an object a
// person has since moved done: the refusal's cause is gone.
func confirmedDone(ctx context.Context, cwm cwmapi.CWMService, id, why string) bool {
	if cwm == nil || !strings.Contains(why, "but "+id+" cannot become "+api.StateDone) {
		return false
	}
	o, err := loadObject(ctx, cwm, id)
	return err == nil && ownState(o) == api.StateDone
}

// personStepItems is a person's ready step as the actions it asks: a
// validation after the hand tests it waits on, a decision, an answer.
func personStepItems(ctx context.Context, cwm cwmapi.CWMService, d api.Demand, t api.WorkTodo, base func(string, string) api.PersonItem) []api.PersonItem {
	p := t.Packet
	switch p.Interaction {
	case api.InteractionValidate, api.InteractionChallenge:
		var out []api.PersonItem
		var tests []string
		for _, del := range handTests(d.Deliverables, p.Verifies) {
			out = append(out, testItem(base, del))
			tests = append(tests, "“"+del.Title+"”")
		}
		it := base(api.ActionValidate, t.ID)
		it.Todo, it.Ask, it.Expects, it.Since = t.ID, p.AskedIn, inboxapi.ExpectsVerdict, t.Since
		it.Asks = stepAsks(p, "look at the result against the request; accept it, or send it back saying what to change.")
		it.Done = doneBefore(d)
		if len(tests) > 0 {
			it.WaitsOn = "Test by hand first: " + strings.Join(tests, ", ")
		}
		return append(out, it)
	case api.InteractionDecide:
		on := inKeyOf(d.ID, endsOnObject(t))
		if p.EndsOn != "" && on != "" {
			if o, err := loadObject(ctx, cwm, on); err == nil && o.Type == decisionsapi.DecisionType {
				if ownState(o) == p.EndsOn {
					return nil // decided already: its step ends at the next settle, nothing is asked
				}
				it := decisionItem(o)
				it.Refs = challengesOf(ctx, cwm, o.ID)
				it.Request, it.Todo, it.Ask, it.Since = d.ID, t.ID, p.AskedIn, t.Since
				it.Proven = append(it.Proven, provenOf(d)...)
				return []api.PersonItem{it}
			}
		}
		it := base(api.ActionDecide, t.ID)
		it.Todo, it.Ask, it.Expects, it.Since = t.ID, p.AskedIn, inboxapi.ExpectsText, t.Since
		it.Asks = stepAsks(p, "decide, and say what you chose and why.")
		return []api.PersonItem{it}
	default:
		it := base(api.ActionAnswer, t.ID)
		it.Todo, it.Ask, it.Expects, it.Since = t.ID, p.AskedIn, inboxapi.ExpectsText, t.Since
		it.Asks = stepAsks(p, "give what is asked; your answer passes to what comes next.")
		return []api.PersonItem{it}
	}
}

// stepAsks is what a person's step asks, in its own words (its
// instructions), else the action's.
func stepAsks(p api.WorkPacket, fallback string) string {
	if s := strings.TrimSpace(p.Instructions); s != "" {
		return "Step " + p.Step + ": " + s
	}
	return "Step " + p.Step + ": " + fallback
}

// handTests is the deliverables a validation waits on a hand test for: not
// verified, no check verifies them, and the approval does not verify them —
// or they say how a person tests them (by_hand), which is a test of its own.
func handTests(ds []api.Deliverable, verifies string) []api.Deliverable {
	approved := map[string]bool{}
	for _, title := range approvedBy(verifies, ds) {
		approved[title] = true
	}
	var out []api.Deliverable
	for _, d := range ds {
		if d.Verified || strings.TrimSpace(d.Check) != "" {
			continue
		}
		if !approved[d.Title] || strings.TrimSpace(d.ByHand) != "" {
			out = append(out, d)
		}
	}
	return out
}

// testItem is a hand test of one deliverable.
func testItem(base func(string, string) api.PersonItem, del api.Deliverable) api.PersonItem {
	it := base(api.ActionTest, "")
	it.Key = api.ActionTest + ":" + it.Request + "#" + del.Title
	it.Deliverable, it.ByHand = del.Title, strings.TrimSpace(del.ByHand)
	it.Asks = fmt.Sprintf("Test “%s” by hand: say whether it passes, or how it fails.", del.Title)
	return it
}

// provenOf is what is already proven on a request: its deliverables verified
// and what its agents' last steps came to.
func provenOf(d api.Demand) []string {
	var out []string
	for _, del := range d.Deliverables {
		switch {
		case !del.Verified:
		case del.VerifiedBy != nil:
			out = append(out, fmt.Sprintf("“%s” — verified by %s", del.Title, del.VerifiedBy.ID))
		default:
			out = append(out, fmt.Sprintf("“%s” — its check passed", del.Title))
		}
	}
	if d.Plan == nil {
		return out
	}
	var steps []string
	for _, s := range d.Plan.Steps {
		if s.Outcome != schedapi.StatusSuccess || s.Doer == api.DoerPerson || strings.TrimSpace(s.Result) == "" {
			continue
		}
		steps = append(steps, s.Step+": "+firstLine(s.Result))
	}
	if len(steps) > 3 {
		steps = steps[len(steps)-3:]
	}
	return append(out, steps...)
}

// doneBefore is what the work before a validation did, to judge it by: its
// last agent step's result, with its details.
func doneBefore(d api.Demand) string {
	if d.Plan == nil {
		return ""
	}
	for i := len(d.Plan.Steps) - 1; i >= 0; i-- {
		s := d.Plan.Steps[i]
		if s.Outcome != schedapi.StatusSuccess || s.Doer == api.DoerPerson || strings.TrimSpace(s.Result) == "" {
			continue
		}
		out := "**" + s.Step + "** — " + strings.TrimSpace(s.Result)
		if det := strings.TrimSpace(s.Details); det != "" {
			out += "\n\n" + det
		}
		return out
	}
	return ""
}

// challengesOf is the decisions challenging one, each to open.
func challengesOf(ctx context.Context, cwm cwmapi.CWMService, id string) []api.Ref {
	ids, err := incoming(ctx, cwm, id, challengesapi.EdgeChallenges)
	if err != nil {
		return nil
	}
	var out []api.Ref
	for _, c := range ids {
		out = append(out, api.Ref{ID: c, Label: "Challenged by"})
	}
	return out
}

// decisionItems is every open decision of the person's ready to decide,
// planned or not: its options and the one proposed - an agent's is not asked
// (an agent takes it). One not prepared is not the person's yet — an agent
// prepares it first (the decision-prepared rule files its decide request, or
// the plan carrying it prepares it).
func decisionItems(ctx context.Context, cwm cwmapi.CWMService) []api.PersonItem {
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: decisionsapi.DecisionType})
	if err != nil {
		return nil
	}
	whole := cwmapi.NewWholeReader(ctx, cwm, decisionsapi.DecisionType)
	var out []api.PersonItem
	for _, o := range qualifyListed(list.Objects, list.Projects) {
		if st := ownState(o); st != "" && st != decisionsapi.StatusOpen {
			continue // the index says it is settled: no need to read it
		}
		w, err := whole.One(o)
		if err != nil {
			continue
		}
		if st := ownState(w); st != "" && st != decisionsapi.StatusOpen {
			continue
		}
		if agentsDecision(w) {
			continue // an agent's: an agent takes it, with its reasons; the person may reopen it
		}
		it := decisionItem(w)
		if !readyToDecide(it) {
			continue
		}
		it.Refs = challengesOf(ctx, cwm, w.ID)
		out = append(out, it)
	}
	return out
}

// agentsDecision says a decision is marked an agent's: « À faire » holds only
// the person's decisions.
func agentsDecision(o cwmapi.Object) bool {
	p, _, _ := cwmapi.DecodeCapability[proposalOf](o, decisionsapi.CapDecision)
	return p.Owner == decisionsapi.OwnerAgent
}

// readyToDecide says a decision is prepared for a person: two options at
// least, and the one proposed.
func readyToDecide(it api.PersonItem) bool {
	return it.Proposal != "" && len(it.Options) >= 2
}

// decisionItem is an open decision as an item: choose one of its options.
func decisionItem(o cwmapi.Object) api.PersonItem {
	p, _, _ := cwmapi.DecodeCapability[proposalOf](o, decisionsapi.CapDecision)
	module, _ := cwmapi.ModuleOfObjectID(bareID(o.ID))
	it := api.PersonItem{Key: api.ActionDecide + ":" + o.ID, Action: api.ActionDecide, Object: o.ID, Module: module,
		Project: keyOf(o.ID), Title: o.DisplayTitle(), Options: p.Options}
	prep := decisionsapi.ReadPrepared(o.Body, p.Options)
	it.Reasoning = prep.Reasoning
	if prep.Any() {
		it.Prepared = prep.Options
	}
	it.Asks = "Choose one of its options, saying why — no proposal was made."
	if p.Proposal != nil {
		it.Proposal, it.ProposalWhy = p.Proposal.Option, p.Proposal.Why
		it.Asks = fmt.Sprintf("Choose one of its options, saying why — “%s” is proposed.", p.Proposal.Option)
	}
	if p.Challenge != nil {
		for _, pt := range p.Challenge.Points {
			it.Challenged = append(it.Challenged, pt+" — "+p.Challenge.By)
		}
	}
	return it
}

// openQuestions are the requests an open intake question asks about, from
// the view's one listing of the scheduler (scheduledTodos), each named as
// refs names a to-do's request (bare when refs is nil); none when it cannot
// be read.
func openQuestions(ctx context.Context, refs *projectRefs) map[string]bool {
	out := map[string]bool{}
	todos, err := scheduledTodos(ctx)
	if err != nil {
		return out
	}
	for _, t := range todos {
		switch t.State {
		case schedapi.StateReady, schedapi.StateClaimed, schedapi.StateReported:
		default:
			continue
		}
		if ask, ok := inboxapi.AskOf(t); ok {
			for _, it := range ask.Satisfies {
				if r, ok := questionedRequest(it); ok {
					out[refs.ref(r, t.Repository)] = true
				}
			}
		}
	}
	return out
}

// askPending: the Inbox entry is still waiting for its answer.
func askPending(ctx context.Context, entry string) bool {
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		return false
	}
	got, err := inbox.Get(ctx, inboxapi.GetInput{ID: entry})
	return err == nil && (got.Entry.Status == "" || got.Entry.Status == inboxapi.StatusPending)
}

// inboxItems is every pending Inbox entry no other item carries: an approval
// request asks a verdict (validate) or an answer; anything else is a notice.
func inboxItems(ctx context.Context, carried map[string]bool) []api.PersonItem {
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		return nil
	}
	tri, err := inbox.Triage(ctx, inboxapi.TriageInput{})
	if err != nil {
		return nil
	}
	var out []api.PersonItem
	for _, e := range tri.Entries {
		if carried[e.ID] || strings.HasPrefix(e.RequestedBy, api.Owner+":") {
			// Planification's own questions are their steps' items; one no
			// step carries any more was taken another way.
			continue
		}
		module, _ := cwmapi.ModuleOfObjectID(e.About)
		it := api.PersonItem{Object: e.ID, Module: module, Title: e.Title, Since: e.RequestedAt}
		switch {
		case e.Type != inboxapi.RequestType:
			it.Action, it.Asks = api.ActionNotice, "For your information."
		case e.Status != "" && e.Status != inboxapi.StatusPending:
			continue
		case e.Expects == inboxapi.ExpectsVerdict:
			it.Action, it.Ask, it.Expects = api.ActionValidate, e.ID, inboxapi.ExpectsVerdict
			it.Asks = "Approve it, or send it back saying what to change."
		default:
			it.Action, it.Ask, it.Expects = api.ActionAnswer, e.ID, inboxapi.ExpectsText
			it.Asks = "Answer the question."
		}
		if e.About != "" && strings.Contains(e.About, ".") {
			it.Request = e.About
			if module == "" {
				it.Module, _ = cwmapi.ModuleOfObjectID(e.About)
			}
		}
		it.Key = it.Action + ":" + e.ID
		out = append(out, it)
	}
	return out
}
