// SPDX-License-Identifier: Apache-2.0

// Package core implements the inbox service over the CWM Object API. A request
// is an approval-request object under the inbox object; its state is
// content.status, its payload the body the approval-request data-type declares.
// Nothing is stored here.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	decisionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person"
)

// systemInboxModule is where Ensure files the project's inbox (the system
// module holding the inbox's live, committed state).
const systemInboxModule = cwmapi.SystemModulePrefix + "inbox"

// decisionsResolveURI is the method a choice's option runs: the decision the
// entry is about is resolved there, as everywhere else (its panel, the CLI).
const decisionsResolveURI = "ktw-methods:decisions/resolve"

// requestBody is the approval-request body (ktw-agents:/data-type/approval-request).
type requestBody struct {
	Title        string       `json:"title"`
	Description  string       `json:"description,omitempty"`
	RequestedBy  string       `json:"requested_by,omitempty"`
	RequestedAt  string       `json:"requested_at,omitempty"`
	Decision     string       `json:"decision,omitempty"`
	Urgency      string       `json:"urgency,omitempty"`
	BlocksTask   bool         `json:"blocks_task,omitempty"`
	DecisionNote string       `json:"decision_note,omitempty"`
	ResolvedBy   string       `json:"resolved_by,omitempty"`
	ResolvedAt   string       `json:"resolved_at,omitempty"`
	Relayed      *api.Relayed `json:"relayed,omitempty"`
	Expects      string       `json:"expects,omitempty"`
	Panel        string       `json:"panel,omitempty"`
	Options      []string     `json:"options,omitempty"`
	Choice       string       `json:"choice,omitempty"`
}

// Service implements api.InboxService.
type Service struct {
	now func() time.Time
}

// New returns the inbox service.
func New() *Service { return &Service{now: time.Now} }

var _ api.InboxService = (*Service)(nil)

func (s *Service) stamp() string { return s.now().UTC().Format(time.RFC3339) }

func cwmOf(ctx context.Context) (cwmapi.CWMService, error) {
	return application.UseService[cwmapi.CWMService](ctx)
}

// findInbox returns the project's inbox: the one in sys-inbox, else the first
// inbox object there is. Empty when there is none.
func findInbox(ctx context.Context, cwm cwmapi.CWMService) (string, error) {
	out, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: api.InboxType})
	if err != nil {
		return "", err
	}
	if len(out.Objects) == 0 {
		return "", nil
	}
	for _, o := range out.Objects {
		if m, _ := cwmapi.ModuleOfObjectID(o.ID); m == systemInboxModule {
			return o.ID, nil
		}
	}
	return out.Objects[0].ID, nil
}

// requireInbox is the inbox, created when the project has none yet.
func requireInbox(ctx context.Context, cwm cwmapi.CWMService) (string, error) {
	id, _, err := ensureInbox(ctx, cwm)
	return id, err
}

// ensureInbox returns the project's inbox, creating `sys-inbox` and its inbox
// when there is none. A concurrent Ensure that created them first is not an
// error — the bootstrap hook and a verb needing the inbox may race.
func ensureInbox(ctx context.Context, cwm cwmapi.CWMService) (string, bool, error) {
	id, err := findInbox(ctx, cwm)
	if err != nil || id != "" {
		return id, false, err
	}
	if err := ensureSystemModule(ctx, cwm); err != nil {
		return "", false, err
	}
	inboxID := cwmapi.QualifyObjectID(systemInboxModule, api.InboxType)
	inbox := cwmapi.Object{ID: inboxID, Type: api.InboxType}
	inbox.Name = "Inbox"
	inbox.Parent = systemInboxModule
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: systemInboxModule, Object: inbox}); err != nil {
		if errors.Is(err, cwmapi.ErrAlreadyExists) {
			return inboxID, false, nil
		}
		return "", false, fmt.Errorf("create the inbox: %w", err)
	}
	return inboxID, true, nil
}

// Relocate moves an inbox kept outside sys-inbox, and its entries, into
// sys-inbox. Each move is an ObjectMove into the module — a new id, every
// reference rewritten, the move journaled — so nothing here writes a file.
// An inbox already in sys-inbox, or none at all, is nothing to do.
func (s *Service) Relocate(ctx context.Context, in api.RelocateInput) (api.RelocateOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RelocateOutput{}, err
	}
	id, err := findInbox(ctx, cwm)
	if err != nil {
		return api.RelocateOutput{}, err
	}
	out := api.RelocateOutput{Inbox: id}
	if m, _ := cwmapi.ModuleOfObjectID(id); id == "" || m == systemInboxModule {
		return out, nil
	}
	kids, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: id})
	if err != nil {
		return api.RelocateOutput{}, err
	}
	rename := func(old string) string {
		_, local, _ := cwmapi.SplitObjectID(old)
		return cwmapi.QualifyObjectID(systemInboxModule, local)
	}
	out.Inbox = rename(id)
	out.Moves = append(out.Moves, api.Relocation{From: id, To: out.Inbox})
	for _, k := range kids.Objects {
		out.Moves = append(out.Moves, api.Relocation{From: k.ID, To: rename(k.ID)})
	}
	if !in.Apply {
		return out, nil
	}
	if err := ensureSystemModule(ctx, cwm); err != nil {
		return api.RelocateOutput{}, err
	}
	const why = "every project keeps its inbox in sys-inbox"
	moved, err := cwm.ObjectMove(ctx, cwmapi.ObjectMoveInput{ID: id, NewModule: systemInboxModule, NewParent: systemInboxModule, Reason: why})
	if err != nil {
		return api.RelocateOutput{}, fmt.Errorf("move the inbox %s: %w", id, err)
	}
	out.Inbox, out.Moves[0].To = moved.ID, moved.ID
	for i, k := range kids.Objects {
		m, err := cwm.ObjectMove(ctx, cwmapi.ObjectMoveInput{ID: k.ID, NewModule: systemInboxModule, NewParent: moved.ID, Reason: why})
		if err != nil {
			return api.RelocateOutput{}, fmt.Errorf("move %s (the inbox is already at %s): %w", k.ID, moved.ID, err)
		}
		out.Moves[i+1].To = m.ID
	}
	out.Applied = true
	return out, nil
}

// ensureSystemModule creates the sys-inbox module object unless it exists.
func ensureSystemModule(ctx context.Context, cwm cwmapi.CWMService) error {
	_, err := cwmapi.EnsureSystemModule(ctx, cwm, systemInboxModule,
		"The inbox's operational space: the inbox and the messages filed into it (committed).")
	return err
}

// Ensure gives the project its inbox (idempotent).
func (s *Service) Ensure(ctx context.Context, _ api.EnsureInput) (api.EnsureOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.EnsureOutput{}, err
	}
	id, created, err := ensureInbox(ctx, cwm)
	if err != nil {
		return api.EnsureOutput{}, err
	}
	return api.EnsureOutput{Inbox: id, Created: created}, nil
}

// load reads an entry in full and refuses one that is not in the inbox.
func load(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, string, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return cwmapi.Object{}, "", err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if errors.Is(err, cwmapi.ErrNotFound) {
		return cwmapi.Object{}, "", fmt.Errorf("%w: %s: %w", api.ErrGone, id, err)
	}
	if err != nil {
		return cwmapi.Object{}, "", err
	}
	inbox, err := requireInbox(ctx, cwm)
	if err != nil {
		return cwmapi.Object{}, "", err
	}
	if got.Object.ParentID() != inbox {
		return cwmapi.Object{}, "", fmt.Errorf("%w: %s is not in the inbox %s: %w", api.ErrGone, id, inbox, cwmapi.ErrInvalidInput)
	}
	return got.Object, module, nil
}

func bodyOf(o cwmapi.Object) requestBody {
	var b requestBody
	if raw := o.FieldsJSON(); len(raw) > 0 {
		_ = json.Unmarshal(raw, &b)
	}
	return b
}

// statusOf is what a request's Entry says: pending while its lifecycle waits,
// dismissed, or — answered — its decision (approved when none was given).
func statusOf(o cwmapi.Object) string {
	life, _ := o.Lifecycle()
	switch life.State {
	case "", api.StatePending:
		return api.StatusPending
	case api.StateDismissed:
		return api.StatusDismissed
	}
	if b := bodyOf(o); b.Decision != "" {
		return b.Decision
	}
	return api.StatusApproved
}

// File creates a pending approval-request under the inbox.
func (s *Service) File(ctx context.Context, in api.FileInput) (api.FileOutput, error) {
	if strings.TrimSpace(in.Title) == "" {
		return api.FileOutput{}, fmt.Errorf("%w: inbox.File: title required", cwmapi.ErrInvalidInput)
	}
	if err := checkFiled(in); err != nil {
		return api.FileOutput{}, fmt.Errorf("%w: inbox.File: %s", cwmapi.ErrInvalidInput, err.Error())
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.FileOutput{}, err
	}
	inbox, err := requireInbox(ctx, cwm)
	if err != nil {
		return api.FileOutput{}, err
	}
	module, err := cwmapi.ModuleOfObjectID(inbox)
	if err != nil {
		return api.FileOutput{}, err
	}
	urgency := in.Urgency
	if urgency == "" {
		urgency = "medium"
	}
	body, err := json.Marshal(requestBody{
		Title: in.Title, Description: in.Description, RequestedBy: in.RequestedBy,
		RequestedAt: s.stamp(), Urgency: urgency, BlocksTask: in.BlocksTask, Expects: in.Expects, Panel: in.Panel,
		Options: in.Options,
	})
	if err != nil {
		return api.FileOutput{}, err
	}
	// No id: the type declares no prefix, so CWM mints a time-ordered one —
	// requests are filed concurrently, and a sequence would need a lock.
	o := cwmapi.Object{Type: api.RequestType}
	o.Parent = inbox
	o.Name = in.Title
	o.SetFieldsJSON(body) // it starts pending: its workflow's initial state
	if in.About != "" {
		// What it is about, by CWM's own edge (any object to any): the Inbox
		// opens it, and the object lists the question among its links.
		o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: "about", To: in.About})
	}
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o})
	if err != nil {
		return api.FileOutput{}, err
	}
	return api.FileOutput{Request: toEntry(inbox, out.Object), Path: out.Path}, nil
}

// checkFiled is the filing rule: what a request expects is one of the kinds;
// a choice is about the decision it decides and offers it at least two
// distinct, non-empty options; no other kind carries options.
func checkFiled(in api.FileInput) error {
	switch in.Expects {
	case "", api.ExpectsVerdict, api.ExpectsText, api.ExpectsNotice:
		if len(in.Options) > 0 {
			return fmt.Errorf("options are a choice's: a request expecting %q takes none", in.Expects)
		}
		return nil
	case api.ExpectsChoice:
	default:
		return fmt.Errorf("expects is verdict, text, choice or notice, not %q", in.Expects)
	}
	if strings.TrimSpace(in.About) == "" {
		return errors.New("a choice is about the decision it decides: about required")
	}
	if len(in.Options) < 2 {
		return fmt.Errorf("a choice offers at least two options, not %d", len(in.Options))
	}
	seen := map[string]bool{}
	for _, o := range in.Options {
		if strings.TrimSpace(o) == "" {
			return errors.New("an option has no label")
		}
		if seen[o] {
			return fmt.Errorf("the option %q is offered twice", o)
		}
		seen[o] = true
	}
	return nil
}

// Get reads one entry of the inbox.
func (s *Service) Get(ctx context.Context, in api.GetInput) (api.GetOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.GetOutput{}, err
	}
	o, _, err := load(ctx, cwm, in.ID)
	if err != nil {
		return api.GetOutput{}, err
	}
	return api.GetOutput{Entry: toEntry(o.ParentID(), o)}, nil
}

// refuseAgent is the rule of who answers: an inbox entry is a question for a
// person — an agent asks it, the person answers or dismisses it. A call whose
// cause is an agent (an agent run, a command an agent's session runs) is
// refused, whatever by says — what waits on the answer (a person's to-do, a
// gate's confirmation) is then reported as the person's — unless it relays
// that person's answer, confirmed, with relaying on (iam's one rule,
// person.AcceptRelayed); the answer then records the relay.
func refuseAgent(ctx context.Context, verb, id, by string) error {
	if _, err := person.AcceptRelayed(ctx, by, strings.ToLower(verb)+":"+id, fmt.Sprintf("inbox.%s: %s is answered by the person themselves (the Inbox, the request view, or their own command)", verb, id)); err != nil {
		return fmt.Errorf("%w: %v", cwmapi.ErrInvalidInput, err)
	}
	return nil
}

// Answer resolves a pending request — by the answer rule (checkAnswer): a
// rejection carries a note, a request expecting text its text, a choice one
// of its options. The ObjectUpdate publishes the update event
// (api.UpdatedEventURI) the asking task waits on, which resumes it.
//
// An option of a choice decides first: the decision the entry is about is
// resolved with it, as the person (decide), and only then is the answer
// written — a refused resolve ("already decided by ...") is returned as it
// came, the entry left pending. What moves on the decision may close the
// entry meanwhile (the step it ends reports it): the entry is then returned
// as it is.
func (s *Service) Answer(ctx context.Context, in api.AnswerInput) (api.AnswerOutput, error) {
	if err := refuseAgent(ctx, "Answer", in.ID, in.By); err != nil {
		return api.AnswerOutput{}, err
	}
	decision := in.Decision
	if decision == "" {
		decision = api.StatusApproved
	}
	if decision == api.StatusPending {
		return api.AnswerOutput{}, fmt.Errorf("%w: inbox.Answer: pending is not an answer", cwmapi.ErrInvalidInput)
	}
	got, gerr := s.Get(ctx, api.GetInput{ID: in.ID})
	if gerr == nil && got.Entry.Status != "" && got.Entry.Status != api.StatusPending {
		return alreadyAnswered(got.Entry), nil
	}
	// A held catch-up's card (catchup.go): its approval releases the
	// per-object events, before the answer is written - a failed release
	// leaves the card pending, answered again once CWM can release.
	if eventID, _, ok := catchUpOf(got.Entry.RequestedBy); gerr == nil && ok && decision == api.StatusApproved {
		if err := releaseCatchUp(ctx, eventID); err != nil {
			return api.AnswerOutput{}, err
		}
	}
	decided := false
	if in.Choice != "" {
		cwm, err := cwmOf(ctx)
		if err != nil {
			return api.AnswerOutput{}, err
		}
		o, _, err := load(ctx, cwm, in.ID)
		if err != nil {
			return api.AnswerOutput{}, err
		}
		if err := pendingRequest(o); err != nil {
			return api.AnswerOutput{}, err
		}
		b := bodyOf(o)
		if err := checkAnswer(b.Expects, b.Options, decision, in.Choice, in.Note); err != nil {
			return api.AnswerOutput{}, fmt.Errorf("%w: %s: %s", cwmapi.ErrInvalidInput, in.ID, err.Error())
		}
		if err := decide(ctx, o.EdgeTo("about"), in.ID, in.Choice, in.Note, in.By); err != nil {
			return api.AnswerOutput{}, err
		}
		decided = true
	}
	o, err := s.resolve(ctx, in.ID, decision, in.Choice, in.Note, in.By)
	if err != nil {
		if decided && errors.Is(err, cwmapi.ErrTypeRuleViolation) {
			if got, gerr := s.Get(ctx, api.GetInput{ID: in.ID}); gerr == nil && got.Entry.Status != api.StatusPending {
				return api.AnswerOutput{Request: got.Entry}, nil
			}
		}
		return api.AnswerOutput{}, err
	}
	return api.AnswerOutput{Request: toEntry(o.ParentID(), o)}, nil
}

// alreadyAnswered is the answer to a second answer: nothing changes, and it
// says by whom, when and what the entry was answered.
func alreadyAnswered(e api.Entry) api.AnswerOutput {
	no := false
	what := e.Status
	if e.Choice != "" {
		what += " (" + e.Choice + ")"
	}
	msg := fmt.Sprintf("Already %s", what)
	if e.Status == api.StatusDismissed {
		msg = "Already dismissed"
	}
	if e.ResolvedBy != "" {
		msg += " by " + e.ResolvedBy
	}
	if e.ResolvedAt != "" {
		msg += " on " + e.ResolvedAt
	}
	if n := strings.TrimSpace(e.DecisionNote); n != "" {
		msg += ": " + n
	}
	return api.AnswerOutput{Request: e, OK: &no, Message: msg + "."}
}

// decide resolves the decision a choice is about with the option chosen, as
// who answers (resolver), through ktw-methods' decisions/resolve — the one
// way a decision is decided. Its refusal comes back unchanged.
func decide(ctx context.Context, about, id, choice, note, by string) error {
	if about == "" {
		return fmt.Errorf("%w: inbox.Answer: %s is about no decision: its choice decides nothing", cwmapi.ErrInvalidInput, id)
	}
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil || actions == nil {
		return fmt.Errorf("inbox.Answer: %s decides %s through %s, and no ActionService is reachable here: %v", id, about, decisionsResolveURI, err)
	}
	_, actor := resolver(ctx, by)
	raw, err := json.Marshal(decisionsapi.ResolveInput{ID: about, Actor: actor, Selected: []string{choice}, Rationale: note})
	if err != nil {
		return err
	}
	// A relayed answer decides as part of itself: the decision is the act the
	// confirmation covered.
	_, err = actions.Run(person.ForAct(ctx, "decide:"+about), actionsapi.RunInput{URI: decisionsResolveURI, Input: raw})
	return err
}

// pendingRequest refuses an entry that is no request, or one already closed.
func pendingRequest(o cwmapi.Object) error {
	if o.Type != api.RequestType {
		return fmt.Errorf("%w: %s is a %q, not an approval-request", cwmapi.ErrInvalidInput, o.ID, o.Type)
	}
	if st := statusOf(o); st != "" && st != api.StatusPending {
		return fmt.Errorf("%w: %s is already %s", cwmapi.ErrTypeRuleViolation, o.ID, st)
	}
	return nil
}

// resolver is who resolves a request, from the call's cause — never guessed
// from by: a person's cause is that person (by, when given, is the name
// recorded; the cause's handle otherwise); any other cause (a reaction, a hook:
// what asked withdrawing its question) is no person, recorded as its cause; a
// call with no cause is the person by names ("user" when it names none).
// Agent causes are refused before (refuseAgent), but for a relayed act: its
// person resolves it.
func resolver(ctx context.Context, by string) (string, cwmapi.Actor) {
	c, ok := cause.CauseFrom(ctx)
	switch {
	case ok && c.Kind == cause.KindAgent && c.Relay != nil:
		if by == "" {
			by = c.Relay.Person
		}
		return by, cwmapi.Actor{ID: c.Relay.Person, Kind: cwmapi.ActorPerson}
	case ok && c.Kind == cause.KindPerson:
		if by == "" {
			by = c.ID
		}
		return by, cwmapi.Actor{ID: c.ID, Kind: cwmapi.ActorPerson}
	case ok:
		who := c.Kind + ":" + c.ID
		if by == "" {
			by = who
		}
		return by, cwmapi.Actor{ID: who, Kind: cwmapi.ActorAgent}
	}
	if by == "" {
		by = "user"
	}
	return by, cwmapi.Actor{ID: by, Kind: cwmapi.ActorPerson}
}

// resolve closes a pending request: the answer (decision, note, who, when) is
// written to its body, then the request moves out of pending by a transition —
// answered, or dismissed — whose actor is the call's cause (resolver). The one
// write path Answer and Dismiss share; the transition's update is the event the
// waiting task resumes on.
func (s *Service) resolve(ctx context.Context, id, status, choice, note, by string) (cwmapi.Object, error) {
	by, actor := resolver(ctx, by)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return cwmapi.Object{}, err
	}
	o, module, err := load(ctx, cwm, id)
	if err != nil {
		return cwmapi.Object{}, err
	}
	if err := pendingRequest(o); err != nil {
		return cwmapi.Object{}, err
	}
	if status != api.StatusDismissed {
		b := bodyOf(o)
		if err := checkAnswer(b.Expects, b.Options, status, choice, note); err != nil {
			return cwmapi.Object{}, fmt.Errorf("%w: %s: %s", cwmapi.ErrInvalidInput, id, err.Error())
		}
	}
	// The fields are updated as a map so the ones this service does not know survive.
	body := map[string]any{}
	if raw := o.FieldsJSON(); len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			return cwmapi.Object{}, fmt.Errorf("%w: %s: the fields are not an object: %v", cwmapi.ErrInvalidInput, id, err)
		}
	}
	if status != api.StatusDismissed {
		body["decision"] = status
	}
	if note != "" {
		body["decision_note"] = note
	}
	if choice != "" {
		body["choice"] = choice
	}
	body["resolved_by"] = by
	body["resolved_at"] = s.stamp()
	if c, ok := cause.CauseFrom(ctx); ok && c.Kind == cause.KindAgent && c.Relay != nil {
		body["relayed"] = api.Relayed{By: c.ID, Confirmed: c.Relay.Confirmed, At: c.Relay.At}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return cwmapi.Object{}, err
	}
	o.SetFieldsJSON(raw)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o}); err != nil {
		return cwmapi.Object{}, err
	}
	to := api.StateAnswered
	if status == api.StatusDismissed {
		to = api.StateDismissed
	}
	moved, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{
		ID: id, To: to, By: actor, Note: note,
	})
	if err != nil {
		return cwmapi.Object{}, err
	}
	return moved.Object, nil
}

// checkAnswer is the answer rule: a rejection says what to change, whatever
// the request expects; a request expecting text is answered with that text —
// or dismissed — never rejected; a choice is answered with one of its options
// (approved), or none — rejected, the note saying why it goes back.
func checkAnswer(expects string, options []string, status, choice, note string) error {
	empty := strings.TrimSpace(note) == ""
	if expects == api.ExpectsChoice {
		switch status {
		case api.StatusApproved:
			if choice == "" {
				return fmt.Errorf("a choice is answered with one of its options (%s), or none, rejected with a note", strings.Join(options, ", "))
			}
			if !slices.Contains(options, choice) {
				return fmt.Errorf("%q is not one of the options (%s)", choice, strings.Join(options, ", "))
			}
		case api.StatusRejected:
			if choice != "" {
				return fmt.Errorf("none is no option: a rejection takes no choice (%q)", choice)
			}
			if empty {
				return errors.New("none sends the decision back: the note says why, and is empty")
			}
		default:
			return fmt.Errorf("a choice is answered with an option (approved) or none (rejected), not %s", status)
		}
		return nil
	}
	if choice != "" {
		return fmt.Errorf("only a request expecting a choice is answered with one (%q)", choice)
	}
	if expects == api.ExpectsText {
		if status == api.StatusRejected {
			return errors.New("a question is answered or dismissed, not rejected")
		}
		if empty {
			return errors.New("the answer is empty: write it, or dismiss the question")
		}
		return nil
	}
	if status == api.StatusRejected && empty {
		return errors.New("a rejection says what to change: the note is empty")
	}
	return nil
}

// Triage lists the inbox's entries: every child of the inbox except the
// requests already answered, which stay as a record but leave the list.
func (s *Service) Triage(ctx context.Context, _ api.TriageInput) (api.TriageOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.TriageOutput{}, err
	}
	inbox, err := findInbox(ctx, cwm)
	if err != nil {
		return api.TriageOutput{}, err
	}
	out := api.TriageOutput{Inbox: inbox, Entries: []api.Entry{}}
	if inbox == "" {
		return out, nil // no inbox yet: an empty state, not an error
	}
	kids, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: inbox})
	if err != nil {
		return api.TriageOutput{}, err
	}
	for _, resident := range kids.Objects {
		obj := resident
		m, _ := cwmapi.ModuleOfObjectID(resident.ID)
		if got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: m, ID: resident.ID}); err == nil {
			obj = got.Object // the body is not resident
		}
		if obj.Type == api.RequestType {
			if st := statusOf(obj); st != "" && st != api.StatusPending {
				continue
			}
		}
		out.Entries = append(out.Entries, toEntry(inbox, obj))
	}
	sort.SliceStable(out.Entries, func(i, j int) bool {
		a, b := out.Entries[i], out.Entries[j]
		if a.RequestedAt != b.RequestedAt {
			return a.RequestedAt < b.RequestedAt
		}
		return a.ID < b.ID
	})
	return out, nil
}

// Dismiss clears an entry. A request is closed as dismissed and kept, so the
// task waiting on it resumes with that answer and the record stays; any other
// kind has done its job by reaching a person and is deleted. Like an answer,
// never from an agent's session (refuseAgent): a dismissed person's to-do is
// reported as theirs.
func (s *Service) Dismiss(ctx context.Context, in api.DismissInput) (api.DismissOutput, error) {
	if err := refuseAgent(ctx, "Dismiss", in.ID, ""); err != nil {
		return api.DismissOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.DismissOutput{}, err
	}
	o, module, err := load(ctx, cwm, in.ID)
	if err != nil {
		return api.DismissOutput{}, err
	}
	if o.Type == api.RequestType {
		if _, err := s.resolve(ctx, in.ID, api.StatusDismissed, "", "", ""); err != nil {
			return api.DismissOutput{}, err
		}
		return api.DismissOutput{ID: in.ID}, nil
	}
	if _, err := cwm.ObjectDelete(ctx, cwmapi.ObjectDeleteInput{Module: module, ID: in.ID}); err != nil {
		return api.DismissOutput{}, err
	}
	return api.DismissOutput{ID: in.ID, Removed: true}, nil
}

func toEntry(inbox string, o cwmapi.Object) api.Entry {
	b := bodyOf(o)
	e := api.Entry{ID: o.ID, Inbox: inbox, Type: o.Type, Title: o.DisplayTitle(), Status: statusOf(o), About: o.EdgeTo("about")}
	if o.Type == api.RequestType || len(o.Fields) > 0 {
		e.Description, e.RequestedBy, e.RequestedAt = b.Description, b.RequestedBy, b.RequestedAt
		e.Urgency, e.BlocksTask = b.Urgency, b.BlocksTask
		e.DecisionNote, e.ResolvedBy, e.ResolvedAt, e.Relayed = b.DecisionNote, b.ResolvedBy, b.ResolvedAt, b.Relayed
		e.Expects, e.Panel = b.Expects, b.Panel
		e.Options, e.Choice = b.Options, b.Choice
	}
	return e
}
