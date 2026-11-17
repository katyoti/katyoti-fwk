// SPDX-License-Identifier: Apache-2.0

// Package commslocal implements commsapi.Service — the parent↔child
// user-communication channel — and registers it as a process singleton.
//
// The service composes three substrates, resolved per-call from ctx:
//   - the askBroker (PRIVATE to this package) — routes question metadata,
//   - the task SuspensionManager — the child's suspend/resume wait,
//   - the engine Channel — the wake events (ChildSuspended/Progress) that let a
//     parent's Await react instantly instead of polling.
//
// Both the agent (via ask-user / await-child / answer-child / stop-child tool
// wrappers) and non-agent tasks (the simulator) call this one service, so the
// conversation — and the events the console renders — are identical regardless
// of task type.
package commslocal

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	commsapi "github.com/katyoti/katyoti-fwk/internal/extensions/comms/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// awaitDefaultTimeoutS is the fallback Await park duration when the caller does
// not specify one. The wake events make this a rare ceiling, not the common
// path — a healthy conversation never reaches it.
const awaitDefaultTimeoutS = 300

// A run awaiting as a machine is called again at least this often, event or
// not: an answer or a child's end told while no channel reached it is still
// seen.
const machineRecheck = time.Minute

// The keys a call awaiting as a machine keeps in its Resume.
const (
	resumeQuestion = "question"
	resumeDeadline = "deadline"
	resumeSeen     = "seen"
)

type service struct {
	broker *askBroker
}

// Extension registers the comms service as a process singleton. Parent + child
// run in the same core, so a shared in-memory broker suffices (v1 — no
// cross-restart persistence).
var Extension = application.DeclareSimpleService[commsapi.Service](
	func(_ context.Context) (commsapi.Service, error) {
		return &service{broker: newAskBroker()}, nil
	},
)

var _ commsapi.Service = (*service)(nil)

// ── Child side ─────────────────────────────────────────────────────────

// Ask is a child's question to its parent - a human task: a
// run of the human-task strategy waiting on the parent's holding list, whose
// one member is the parent's run, an agent answering as a person would. The
// question's id is that run's; the broker only shows it to the parent and
// wakes it. The child waits on the run's end - as a machine, holding nothing,
// when its loop waits so - and its answer is the run's result.
func (s *service) Ask(ctx context.Context, req commsapi.AskRequest) (commsapi.Answer, error) {
	// Called again after awaiting as a machine: the answer, or await again.
	if resume, again := agentapi.AwaitResumeFrom(ctx); again {
		if qid, _ := resume[resumeQuestion].(string); qid != "" {
			return s.answerOf(ctx, qid)
		}
	}
	childTaskID := firstNonEmpty(req.CallerRunID, agentapi.RunIDFromContext(ctx), engine.RunIDFromContext(ctx))
	pc := agentapi.PluginContextFrom(ctx)
	parentRunID := firstNonEmpty(req.ParentRunID, str(pc["_parent_run_id"]))
	childName := firstNonEmpty(req.CallerName, str(pc["_child_name"]))

	if parentRunID == "" {
		return commsapi.Answer{}, fmt.Errorf("%w [no parent run id — this is a root run]", commsapi.ErrNoParentChannel)
	}
	if childTaskID == "" {
		return commsapi.Answer{}, fmt.Errorf("%w [no caller run id]", commsapi.ErrNoParentChannel)
	}
	ts, err := taskService(ctx)
	if err != nil {
		return commsapi.Answer{}, err
	}
	resp, err := ts.Run(ctx, taskapi.TaskRunRequest{
		TaskDef:     map[string]any{"name": "child-question", "type": inboxapi.HumanTaskStrategy, "description": "A child's question to its parent"},
		Input:       questionTask(childTaskID, childName, parentRunID, req),
		ParentRunID: childTaskID,
	})
	if err != nil {
		return commsapi.Answer{}, fmt.Errorf("comms: ask the parent %s: %w", parentRunID, err)
	}
	questionID := resp.RunID
	s.broker.Post(parentRunID, agentapi.ChildQuestion{
		ID:          questionID,
		ChildTaskID: childTaskID,
		ChildName:   childName,
		Question:    req.Question,
		Options:     req.Options,
	})
	publishAsked(ctx, childName, childTaskID, parentRunID, questionID, req.Question)

	// A run that waits as a machine awaits the answer holding nothing — no
	// slot, no goroutine.
	if agentapi.MachineWaitsFrom(ctx) {
		return commsapi.Answer{}, awaitAnswer(questionID)
	}
	result, err := ts.Result(ctx, taskapi.ResultRequest{RunID: questionID})
	if err != nil {
		return commsapi.Answer{}, fmt.Errorf("comms: cancelled while awaiting parent answer for %s: %w", questionID, err)
	}
	return answerFrom(questionID, result)
}

// questionTask is the human task a child's question is: an answer to give
// (a choice when it offers options) that only its parent's run - the one
// member of the parent's holding list - may answer.
func questionTask(childTaskID, childName, parentRunID string, req commsapi.AskRequest) map[string]any {
	kind := inboxapi.KindForm
	question := req.Question
	if len(req.Options) > 0 {
		kind = inboxapi.KindChoose
		question = strings.TrimSpace(question + "\n\nOptions:\n- " + strings.Join(req.Options, "\n- "))
	}
	title, _, _ := strings.Cut(strings.TrimSpace(req.Question), "\n")
	if len(title) > 120 {
		title = title[:117] + "..."
	}
	if title == "" {
		title = "A question from " + firstNonEmpty(childName, childTaskID)
	}
	spec := map[string]any{
		"kind": kind, "title": title, "question": question, "candidates": commsapi.ParentList(parentRunID),
		"answerable_by": inboxapi.AnswerableAgent, "asked_by": childTaskID, "priority": "P1",
	}
	if len(req.Options) > 0 {
		spec["options"] = req.Options
	}
	return spec
}

// answerOf is a machine-awaited question checked again: its answer once its
// human task has ended, else await again; a question whose run is gone is
// lost (its process restarted before it was kept).
func (s *service) answerOf(ctx context.Context, questionID string) (commsapi.Answer, error) {
	ts, err := taskService(ctx)
	if err != nil {
		return commsapi.Answer{}, err
	}
	st, err := ts.Status(ctx, taskapi.StatusRequest{RunID: questionID})
	if err != nil {
		return commsapi.Answer{}, fmt.Errorf("comms: question %s is no longer asked (its parent's process restarted?) — ask it again", questionID)
	}
	switch st.Status {
	case taskapi.RunStatusCompleted, taskapi.RunStatusFailed, taskapi.RunStatusCancelled, taskapi.RunStatusKilled:
	default:
		return commsapi.Answer{}, awaitAnswer(questionID)
	}
	result, err := ts.Result(ctx, taskapi.ResultRequest{RunID: questionID})
	if err != nil {
		return commsapi.Answer{}, fmt.Errorf("comms: the answer to %s: %w", questionID, err)
	}
	return answerFrom(questionID, result)
}

// answerFrom reads the parent's answer from the question's human task result.
func answerFrom(questionID string, result taskapi.ResultResponse) (commsapi.Answer, error) {
	if result.Error != "" {
		return commsapi.Answer{}, fmt.Errorf("comms: question %s ended without an answer: %s", questionID, result.Error)
	}
	var a inboxapi.HumanAnswer
	raw, err := json.Marshal(result.Output)
	if err == nil {
		err = json.Unmarshal(raw, &a)
	}
	if err != nil {
		return commsapi.Answer{}, fmt.Errorf("comms: the answer to %s does not read: %w", questionID, err)
	}
	return commsapi.Answer{Type: "text", Text: firstNonEmpty(a.Answer, a.Summary)}, nil
}

func awaitAnswer(questionID string) error {
	return &agentapi.ErrAwait{
		Topic:        engine.RunEndedTopic(questionID),
		Resume:       map[string]any{resumeQuestion: questionID},
		Reason:       "the parent's answer to " + questionID,
		RecheckEvery: machineRecheck,
	}
}

// publishAsked wakes a parent awaiting its children — blocked in Await on this
// child's suspended event, or awaiting as a machine — so it reacts to the
// just-posted question without busy-polling. A nil Channel (no engine context)
// just means no one is awaiting on the wire — the parent's PendingQuestions
// drain still picks the question up next turn.
func publishAsked(ctx context.Context, childName, childTaskID, parentRunID, questionID, question string) {
	ch := engine.ChannelFrom(ctx)
	if ch == nil {
		return
	}
	evt := &engine.ChildSuspendedEvent{
		ChildName:   childName,
		RunID:       childTaskID,
		ParentRunID: parentRunID,
		QuestionID:  questionID,
		Question:    question,
	}
	evt.Correlation = engine.Correlation{RunID: childTaskID, ParentRunID: parentRunID}
	_ = ch.Bus().Publish(engine.ChildSuspendedTopic(childTaskID), evt)
	_ = ch.Bus().Publish(engine.ChildEventsTopic(parentRunID), evt.Correlation)
}

func (s *service) Notify(ctx context.Context, req commsapi.NotifyRequest) error {
	callerRunID := firstNonEmpty(req.CallerRunID, agentapi.RunIDFromContext(ctx), engine.RunIDFromContext(ctx))
	pc := agentapi.PluginContextFrom(ctx)
	parentRunID := firstNonEmpty(req.ParentRunID, str(pc["_parent_run_id"]))
	callerName := firstNonEmpty(req.CallerName, str(pc["_child_name"]))

	ch := engine.ChannelFrom(ctx)
	if ch == nil || callerRunID == "" {
		return nil // nothing listening / no identity — a no-op, not an error
	}
	evt := &engine.ChildProgressEvent{
		ChildName: callerName,
		RunID:     callerRunID,
		Message:   req.Text,
	}
	evt.Correlation = engine.Correlation{RunID: callerRunID, ParentRunID: parentRunID}
	return ch.Bus().Publish(engine.ChildProgressTopic(callerRunID), evt)
}

// ── Parent side ────────────────────────────────────────────────────────

func (s *service) Await(ctx context.Context, req commsapi.AwaitRequest) (commsapi.Event, error) {
	parentID := firstNonEmpty(req.ParentRunID, agentapi.RunIDFromContext(ctx), engine.RunIDFromContext(ctx))

	timeout := time.Duration(awaitDefaultTimeoutS) * time.Second
	if req.TimeoutS > 0 {
		timeout = time.Duration(req.TimeoutS) * time.Second
	}
	if agentapi.MachineWaitsFrom(ctx) && parentID != "" {
		return s.awaitAsMachine(ctx, parentID, req.ChildTaskID, timeout)
	}

	// `asked` wake comes from the BROKER — it is hit on every Post regardless of
	// whether the asking child's engine Channel is reachable (a delegated agent
	// child's bubble can run with a nil Channel, so its `child.suspended` emit is
	// unreliable; the broker is not). Subscribe BEFORE the pre-checks so a Post
	// in the gap is not lost.
	postCh, unsubBroker := s.broker.Subscribe(parentID)
	defer unsubBroker()

	// `completed`/`failed` wake comes from the engine Channel — the bgManager
	// emits those directly on its own channel and the PARENT's await ctx carries
	// the channel reliably (a nil channel just means we lean on the terminal
	// pre-check on the next await).
	wake := make(chan string, 4)
	if ch := engine.ChannelFrom(ctx); ch != nil {
		unsub := ch.Bus().SubscribeAny(func(topic string, payload any) {
			action := childTopicAction(topic)
			if action == "" || action == commsapi.EventAsked {
				return // `asked` is the broker's job now (avoid double-counting)
			}
			if parentID != "" && !childEventMatchesParent(payload, parentID) {
				return
			}
			trySend(wake, action)
		})
		defer unsub()
	}

	// Pre-checks (no lost wakeup): a child already terminal, or a question
	// already outstanding, before we blocked.
	if req.ChildTaskID != "" {
		if ts, err := taskService(ctx); err == nil {
			if rec, err := ts.BackgroundManager().TaskStatus(req.ChildTaskID); err == nil {
				switch rec.Status {
				case taskapi.TaskCompleted:
					return commsapi.Event{Kind: commsapi.EventCompleted}, nil
				case taskapi.TaskFailed:
					return commsapi.Event{Kind: commsapi.EventFailed}, nil
				case taskapi.TaskKilled:
					return commsapi.Event{Kind: commsapi.EventFailed}, nil
				}
			}
		}
	}
	if s.broker.HasOutstanding(parentID) {
		return commsapi.Event{Kind: commsapi.EventAsked}, nil
	}

	select {
	case <-postCh:
		return commsapi.Event{Kind: commsapi.EventAsked}, nil
	case action := <-wake:
		return commsapi.Event{Kind: action}, nil
	case <-time.After(timeout):
		return commsapi.Event{Kind: commsapi.EventTimeout}, nil
	case <-ctx.Done():
		return commsapi.Event{Kind: commsapi.EventCancelled}, nil
	}
}

// awaitAsMachine is Await for a parent that waits as a machine: it says what
// happened since the first call — the child (any child, when none is named)
// ended, a question is asked, the time is up — or awaits its children's
// events, holding nothing. Its deadline and the children already ended at the
// first call ride on the Resume, so they survive a restart.
func (s *service) awaitAsMachine(ctx context.Context, parentID, childTaskID string, timeout time.Duration) (commsapi.Event, error) {
	ts, err := taskService(ctx)
	if err != nil { // its children could never be checked: it would wait to its deadline
		return commsapi.Event{}, fmt.Errorf("its children cannot be checked: %w", err)
	}
	bg := ts.BackgroundManager()
	seen := map[string]bool{}
	var deadline time.Time
	if resume, again := agentapi.AwaitResumeFrom(ctx); again {
		deadline, _ = time.Parse(time.RFC3339Nano, str(resume[resumeDeadline]))
		switch ids := resume[resumeSeen].(type) {
		case []string: // as returned, still in memory
			for _, id := range ids {
				seen[id] = true
			}
		case []any: // as read back from the saved state
			for _, id := range ids {
				seen[str(id)] = true
			}
		}
	} else {
		deadline = time.Now().Add(timeout)
		if childTaskID == "" && bg != nil {
			for _, c := range bg.Children(parentID) {
				if ended(c.Status) != "" {
					seen[c.ID] = true
				}
			}
		}
	}

	if childTaskID != "" && bg != nil {
		rec, err := bg.TaskStatus(childTaskID)
		if err != nil {
			return commsapi.Event{}, fmt.Errorf("child %s is not known (its process restarted?) — check it with task-status", childTaskID)
		}
		if kind := ended(rec.Status); kind != "" {
			return commsapi.Event{Kind: kind}, nil
		}
	} else if bg != nil {
		for _, c := range bg.Children(parentID) {
			if kind := ended(c.Status); kind != "" && !seen[c.ID] {
				return commsapi.Event{Kind: kind}, nil
			}
		}
	}
	if s.broker.HasOutstanding(parentID) {
		return commsapi.Event{Kind: commsapi.EventAsked}, nil
	}
	left := time.Until(deadline)
	if left <= 0 {
		return commsapi.Event{Kind: commsapi.EventTimeout}, nil
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return commsapi.Event{}, &agentapi.ErrAwait{
		Topic:        engine.ChildEventsTopic(parentID),
		Resume:       map[string]any{resumeDeadline: deadline.Format(time.RFC3339Nano), resumeSeen: ids},
		Reason:       "its children's next event",
		RecheckEvery: min(left, machineRecheck),
	}
}

// ended is the Await kind of a child's ended status ("" while it runs).
func ended(status taskapi.TaskLifecycle) string {
	switch status {
	case taskapi.TaskCompleted:
		return commsapi.EventCompleted
	case taskapi.TaskFailed, taskapi.TaskKilled:
		return commsapi.EventFailed
	}
	return ""
}

// Answer is the parent's answer to a child's question, given through the
// human task's input: its to-do, on the parent's holding list
// is taken and reported by the parent's run - an agent answering as a person
// would; the inbox keeps the answer and ends the task, whose end the child
// waits on.
func (s *service) Answer(ctx context.Context, req commsapi.AnswerRequest) (commsapi.Answered, error) {
	q, posted := s.broker.Take(req.QuestionID)
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return commsapi.Answered{}, fmt.Errorf("comms: no scheduler to answer %s: %w", req.QuestionID, err)
	}
	todo, found, err := questionTodo(ctx, todos, req.QuestionID)
	if err != nil {
		return commsapi.Answered{}, err
	}
	if !found {
		return commsapi.Answered{Delivered: false, Detail: fmt.Sprintf("question %q not found (already answered or expired)", req.QuestionID)}, nil
	}
	parent, _ := commsapi.ParentOfList(todo.Group)
	by := commsapi.ParentActor(parent)
	if todo.Claim == nil || todo.Claim.By != by {
		pulled, err := todos.Pull(ctx, schedapi.PullInput{ID: todo.ID, By: by})
		if err != nil {
			return commsapi.Answered{}, fmt.Errorf("comms: take %s to answer it: %w", req.QuestionID, err)
		}
		if pulled.Todo == nil {
			return commsapi.Answered{Delivered: false, Detail: fmt.Sprintf("question %q cannot be answered now: %s", req.QuestionID, pulled.Nothing)}, nil
		}
	}
	if _, err := todos.Report(ctx, schedapi.ReportInput{ID: todo.ID, Result: schedapi.Result{
		Status: schedapi.StatusSuccess, Answer: req.Answer, Summary: firstLine(req.Answer, 200), By: by,
	}}); err != nil {
		return commsapi.Answered{}, fmt.Errorf("comms: answer %s: %w", req.QuestionID, err)
	}
	if !posted {
		ask, _ := inboxapi.AskOf(todo)
		q = agentapi.ChildQuestion{ID: req.QuestionID, ChildTaskID: ask.AskedBy}
	}
	// Surface the answer on the conversation channel so consoles/GUIs render it
	// in the child's lane (the child just received it).
	if ch := engine.ChannelFrom(ctx); ch != nil {
		evt := &engine.ChildProgressEvent{
			ChildName: q.ChildName,
			RunID:     q.ChildTaskID,
			Message:   "↩ answered: " + firstLine(req.Answer, 80),
		}
		evt.Correlation = engine.Correlation{RunID: q.ChildTaskID}
		_ = ch.Bus().Publish(engine.ChildProgressTopic(q.ChildTaskID), evt)
	}
	return commsapi.Answered{Delivered: true, Detail: firstLine(req.Answer, 100)}, nil
}

// questionWait bounds how long Answer waits for a question's to-do: its human
// task puts it at its first step, just after the child asked.
var questionWait = 10 * time.Second

// questionTodo is the open to-do of the question's human task (keyed by its
// run), waited on briefly while the task has not put it yet; found false when
// it is answered already or never came.
func questionTodo(ctx context.Context, todos schedapi.TodoService, questionID string) (schedapi.Todo, bool, error) {
	key := inboxapi.HumanTaskStrategy + ":" + questionID
	deadline := time.Now().Add(questionWait)
	for {
		all, err := todos.List(ctx, schedapi.ListInput{})
		if err != nil {
			return schedapi.Todo{}, false, fmt.Errorf("comms: find the question %s: %w", questionID, err)
		}
		for _, t := range all.Todos {
			if t.Key == key && t.Owner == inboxapi.Owner {
				return t, t.Result == nil, nil
			}
		}
		if time.Now().After(deadline) {
			return schedapi.Todo{}, false, nil
		}
		select {
		case <-ctx.Done():
			return schedapi.Todo{}, false, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (s *service) Stop(ctx context.Context, req commsapi.StopRequest) error {
	if req.ChildTaskID == "" {
		return fmt.Errorf("comms: Stop requires a child task id")
	}
	ts, err := taskService(ctx)
	if err != nil {
		return err
	}
	return ts.BackgroundManager().Kill(req.ChildTaskID)
}

func (s *service) PendingQuestions(ctx context.Context, parentRunID string) []agentapi.ChildQuestion {
	id := firstNonEmpty(parentRunID, agentapi.RunIDFromContext(ctx), engine.RunIDFromContext(ctx))
	if id == "" {
		return nil
	}
	return s.broker.Drain(id)
}

// ── helpers ────────────────────────────────────────────────────────────

func suspensionManager(ctx context.Context) (taskapi.SuspensionManager, error) {
	ts, err := taskService(ctx)
	if err != nil {
		return nil, err
	}
	sm := ts.SuspensionManager()
	if sm == nil {
		return nil, fmt.Errorf("%w [SuspensionManager: nil]", commsapi.ErrNoParentChannel)
	}
	return sm, nil
}

func taskService(ctx context.Context) (taskapi.TaskService, error) {
	ts, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		return nil, fmt.Errorf("%w [TaskService: %v]", commsapi.ErrNoParentChannel, err)
	}
	return ts, nil
}

// childTopicAction maps a `child.<id>.<verb>` topic to an Await event kind.
// Empty string = not a child-lifecycle topic we react to.
func childTopicAction(topic string) string {
	switch {
	case strings.HasSuffix(topic, ".suspended"):
		return commsapi.EventAsked
	case strings.HasSuffix(topic, ".done"):
		return commsapi.EventCompleted
	case strings.HasSuffix(topic, ".failed"):
		return commsapi.EventFailed
	default:
		return ""
	}
}

type correlated interface {
	CorrelationRef() *engine.Correlation
}

// childEventMatchesParent reports whether a child-lifecycle payload belongs to
// the given parent run — the correlation match that decouples the wake from the
// child's task-id (which differs between the bgManager record and the child's
// own loop/run id).
func childEventMatchesParent(payload any, parentID string) bool {
	if c, ok := payload.(correlated); ok {
		return c.CorrelationRef().ParentRunID == parentID
	}
	return false
}

func trySend(ch chan string, v string) {
	select {
	case ch <- v:
	default:
	}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func firstLine(s string, max int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if max > 0 && len(s) > max {
		s = s[:max] + "…"
	}
	return s
}
