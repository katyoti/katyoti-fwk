// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// The panel step: a workflow opens, updates, asks through and
// completes a display panel of its run. Its events are the step's
// StepResult.Panels - published after the checkpoint - numbered by the
// panel's track in the workflow's state; an ask waits for the person's input
// as a durable wait on the panel's input (engine.PanelInputWait) and keeps the
// input as results.<name>.
//
// The ask's wait is rechecked: an answer given while no run
// listened - the run down, restored later, or the bus disconnected - is in
// the panel's log the panel server keeps. On the recheck the
// ask reads PanelService.History from its ask's sequence and takes the first
// input sent after it; none there, it waits again.
//
// An ask naming a data type asks by that type's form; on a
// panel not opened yet it opens a question panel (panel.TypeQuestion) itself
// and completes it once answered. Its answer is validated against the data
// type with the SchemaService: one that does not fit is refused with its
// reason and asked again. A cancel fails the node, to its on_error; the
// panel it asked on ends first (cancelled, or failed on any other failure),
// whether the ask opened it or an earlier open step did - except a panel it
// did not open when on_error retries: the fresh ask goes on it. An ask naming
// a data type coming back on an ended panel - a retry, a loop - opens a new
// question panel.
//
//	- kind: panel
//	  name: publication
//	  open: {type: "ktw-example:/panel-type/example-publish", title: "Publication"}
//	- kind: panel
//	  name: publication
//	  ask: {question: "Overwrite the pages?", actions: [{name: yes}, {name: no}]}
//	- kind: panel
//	  name: publication
//	  complete: {status: ok}
//	- kind: panel
//	  name: owner
//	  ask: {question: "Who owns it?", data_type: "ktw-x:/data-type/owner"}

type handlerPanel struct{}

func (handlerPanel) Kind() NodeKind { return NodePanel }

// panelInputLabel labels the panel input an ask waits for.
const panelInputLabel = "panel-input"

// The ask's own state, kept in its frame (checkpointed): the data type its
// answer must fit, and whether it opened its question panel itself.
const (
	panelDataTypeKey   = "_panel_data_type"
	panelSelfOpenedKey = "_panel_self_opened"
)

// resolvePanelService is the seam to the PanelService a rechecked ask reads
// its panel's log through - package-level so tests substitute a fake.
var resolvePanelService = func(ctx context.Context) (panelapi.PanelService, error) {
	return application.UseService[panelapi.PanelService](ctx)
}

// resolveSchemaService is the seam to the SchemaService an ask naming a data
// type validates its answer with - package-level so tests substitute a stub.
var resolveSchemaService = func(ctx context.Context) (schemaapi.SchemaService, error) {
	return application.UseService[schemaapi.SchemaService](ctx)
}

func (handlerPanel) Step(
	_ context.Context, m *workflowMachine, ws *WorkflowState,
	frame *ActivationFrame, node *Node, event machine.Event,
) (machine.ActionFunc, error) {
	p := node.Panel
	switch frame.Phase {
	case "":
		return func(_ context.Context) (*machine.StepResult, error) {
			spec, err := m.resolveInputBinds(p.Spec, ws)
			if err != nil {
				return m.handleNodeError(ws, frame, node, fmt.Sprintf("panel %s: %v", p.Name, err))
			}
			ev, err := panelEvent(p.Action, spec)
			if err != nil {
				return m.handleNodeError(ws, frame, node, fmt.Sprintf("panel %s: %v", p.Name, err))
			}
			// An ask naming a data type asks by its form.
			dataType, _ := spec["data_type"].(string)
			if p.Action == "ask" && dataType != "" {
				ev.Ask.Form = askForm(dataType)
			}
			if ws.Panels == nil {
				ws.Panels = map[string]panel.Track{}
			}
			var events []panel.Event
			selfOpened := false
			tr, opened := ws.Panels[p.Name]
			if opened && p.Action == "ask" && dataType != "" && (tr.State == panel.StateComplete || tr.State == panel.StateClosed) {
				// Asked again on a panel already ended - a retry after a
				// cancel, a loop after an answer: a new question panel.
				opened = false
			}
			if !opened {
				switch {
				case p.Action == "open":
					id, _ := spec["id"].(string)
					tr = panel.NewTrack(id)
					if ev.Open.Owner == "" {
						ev.Open.Owner = m.runID
					}
				case p.Action == "ask" && dataType != "":
					// It opens its question panel itself, then asks.
					tr = panel.NewTrack("")
					o, err := tr.Next(panel.Event{Kind: panel.KindOpen, Open: &panel.Open{Type: panel.TypeQuestion, Title: ev.Ask.Question, Owner: m.runID}})
					if err != nil {
						return m.handleNodeError(ws, frame, node, err.Error())
					}
					events = append(events, o)
					selfOpened = true
				default:
					return m.handleNodeError(ws, frame, node, fmt.Sprintf("panel %s: %s before the panel is opened", p.Name, p.Action))
				}
			}
			out, err := tr.Next(ev)
			if err != nil {
				return m.handleNodeError(ws, frame, node, err.Error())
			}
			events = append(events, out)
			ws.Panels[p.Name] = tr
			res := &machine.StepResult{NextState: "running", Data: ws, Panels: events}
			if p.Action == "ask" {
				frame.Phase = "asking"
				if frame.Local == nil {
					frame.Local = map[string]any{}
				}
				frame.Local[panelDataTypeKey] = dataType
				frame.Local[panelSelfOpenedKey] = selfOpened
				res.Wait = panelAskWait(tr.ID)
				return res, nil
			}
			ws.PopFrame()
			return res, nil
		}, nil

	case "asking": // Resumed: the person's input - or a recheck, a restart.
		return func(ctx context.Context) (*machine.StepResult, error) {
			tr := ws.Panels[p.Name]
			var in panel.Input
			ok := false
			if event.Label == panelInputLabel {
				in, ok = panelInputOf(event.Payload)
				ok = ok && in.Panel == tr.ID
			}
			if !ok {
				// A recheck (a restore's tick, the bus reconnected) or not
				// this panel's input: the answer may have come while no run
				// listened - the panel's log has it; else it waits on.
				if in, ok = loggedAnswer(ctx, tr); !ok {
					return &machine.StepResult{NextState: "running", Data: ws, Wait: panelAskWait(tr.ID)}, nil
				}
			}
			dataType, _ := frame.Local[panelDataTypeKey].(string)
			selfOpened, _ := frame.Local[panelSelfOpenedKey].(bool)
			if in.Kind == panel.InputCancel {
				// The person cancelled: the node fails, to its on_error.
				return m.failAsk(ws, frame, node, tr, selfOpened, "cancelled", fmt.Sprintf("panel %s: cancelled", p.Name))
			}
			if dataType != "" {
				why, err := answerMisfit(ctx, dataType, in.Value)
				if err != nil {
					return m.failAsk(ws, frame, node, tr, selfOpened, "failed", fmt.Sprintf("panel %s: %v", p.Name, err))
				}
				if why != "" {
					return m.askAgain(ws, frame, node, tr, in, why, selfOpened)
				}
			}
			tr.Answered()
			var events []panel.Event
			if selfOpened {
				c, err := tr.Next(panel.Event{Kind: panel.KindComplete, Complete: &panel.Complete{Status: "ok", Result: in.Value}})
				if err != nil {
					return m.handleNodeError(ws, frame, node, err.Error())
				}
				events = append(events, c)
			}
			ws.Panels[p.Name] = tr
			ws.Results[p.Name] = map[string]any{"kind": string(in.Kind), "name": in.Name, "value": in.Value, "text": in.Text, "by": in.By}
			ws.PopFrame()
			return &machine.StepResult{NextState: "running", Data: ws, Panels: events}, nil
		}, nil
	}
	return nil, fmt.Errorf("panel: unexpected phase %q", frame.Phase)
}

// failAsk fails an asking node to its on_error: the panel it asked on ends
// first with status (cancelled, failed), so none is left asking after the
// node - whether the ask opened it or an earlier open step did. Only a retry
// keeps a panel the ask did not open: the fresh ask goes on it. A question
// panel the ask opened itself always ends; a retry or a loop asking again
// opens a new one.
func (m *workflowMachine) failAsk(ws *WorkflowState, frame *ActivationFrame, node *Node, tr panel.Track, selfOpened bool, status, reason string) (*machine.StepResult, error) {
	tr.Answered()
	var events []panel.Event
	if selfOpened || !willRetry(frame, node) {
		if c, err := tr.Next(panel.Event{Kind: panel.KindComplete, Complete: &panel.Complete{Status: status}}); err == nil {
			events = append(events, c)
		}
	}
	ws.Panels[node.Panel.Name] = tr
	res, err := m.handleNodeError(ws, frame, node, reason)
	if res != nil {
		res.Panels = append(events, res.Panels...)
	}
	return res, err
}

// willRetry tells whether the node's on_error runs it again on this failure
// (strategy retry, retries left) - handleNodeError's own test.
func willRetry(frame *ActivationFrame, node *Node) bool {
	if node.OnError == nil || node.OnError.Strategy != OnErrorRetry {
		return false
	}
	max := node.OnError.MaxRetries
	if max <= 0 {
		max = 1
	}
	return loadRetryCounter(frame).Count < max
}

// askForm is the form an ask naming a data type puts: the inline form node
// of that data type.
func askForm(dataType string) *render.Spec {
	s := render.InlineSpec(render.Node{"kind": "form", "schema": dataType})
	return &s
}

// answerMisfit checks the answer against its data type with the
// SchemaService: why it does not fit ("" when it does); an error when it
// cannot be checked - an answer is never taken unvalidated.
func answerMisfit(ctx context.Context, dataType string, value any) (string, error) {
	svc, err := resolveSchemaService(ctx)
	if err != nil || svc == nil {
		return "", fmt.Errorf("the answer cannot be checked against %s: no SchemaService (%v)", dataType, err)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("the answer: %w", err)
	}
	res, err := svc.ValidateAgainst(ctx, schemaapi.ValidateAgainstInput{URI: dataType, Body: body})
	if err != nil {
		return "", fmt.Errorf("check the answer against %s: %w", dataType, err)
	}
	if res.Valid {
		return "", nil
	}
	var why []string
	for _, d := range res.Diagnostics {
		if string(d.Severity) == "warning" {
			continue
		}
		if d.Path != "" {
			why = append(why, d.Path+": "+d.Message)
		} else {
			why = append(why, d.Message)
		}
	}
	if len(why) == 0 {
		why = append(why, "the answer does not fit "+dataType)
	}
	return strings.Join(why, "; "), nil
}

// askAgain refuses an answer that does not fit: the reason shown - in place
// of the question panel's context box, or before the question on a panel of
// another type - and the ask put again with the value given; it waits on.
func (m *workflowMachine) askAgain(ws *WorkflowState, frame *ActivationFrame, node *Node, tr panel.Track, in panel.Input, why string, questionPanel bool) (*machine.StepResult, error) {
	p := node.Panel
	spec, err := m.resolveInputBinds(p.Spec, ws)
	if err != nil {
		return m.failAsk(ws, frame, node, tr, questionPanel, "failed", fmt.Sprintf("panel %s: %v", p.Name, err))
	}
	ev, err := panelEvent(p.Action, spec)
	if err != nil {
		return m.failAsk(ws, frame, node, tr, questionPanel, "failed", fmt.Sprintf("panel %s: %v", p.Name, err))
	}
	dataType, _ := frame.Local[panelDataTypeKey].(string)
	ev.Ask.Form = askForm(dataType)
	ev.Ask.Value = in.Value
	var events []panel.Event
	switch {
	case questionPanel:
		box := render.Node{"kind": "vbox", "id": panel.QuestionNode, "children": []any{
			render.Node{"kind": "text", "content": why, "color": "red"},
		}}
		o, err := tr.Next(panel.Event{Kind: panel.KindOps, Ops: []render.Op{{Op: render.OpReplace, Target: panel.QuestionNode, Node: box}}})
		if err != nil {
			return m.failAsk(ws, frame, node, tr, questionPanel, "failed", err.Error())
		}
		events = append(events, o)
	case ev.Ask.Question != "":
		ev.Ask.Question = why + "\n\n" + ev.Ask.Question
	default:
		ev.Ask.Question = why
	}
	a, err := tr.Next(ev)
	if err != nil {
		return m.failAsk(ws, frame, node, tr, questionPanel, "failed", err.Error())
	}
	events = append(events, a)
	ws.Panels[p.Name] = tr
	return &machine.StepResult{NextState: "running", Data: ws, Panels: events, Wait: panelAskWait(tr.ID)}, nil
}

// panelAskWait is an ask's durable wait on its panel's input, rechecked on a
// restore and on a bus reconnection (see loggedAnswer).
func panelAskWait(panelID string) *machine.WaitSpec {
	return &machine.WaitSpec{Conditions: []machine.WaitCondition{machine.PanelInputWait(panelID, panelInputLabel)}, Mode: machine.WaitAny, RecheckOnResume: true}
}

// loggedAnswer reads the asking panel's log and answers the first input the
// person sent after the ask - while asking, the ask is the panel's last event
// (tr.Seq), so an input logged after it (Record.After >= tr.Seq) answers it.
// No PanelService, no log yet, or no input: false, the ask waits on.
func loggedAnswer(ctx context.Context, tr panel.Track) (panel.Input, bool) {
	svc, err := resolvePanelService(ctx)
	if err != nil || svc == nil {
		return panel.Input{}, false
	}
	out, err := svc.History(ctx, panelapi.HistoryInput{Panel: tr.ID, From: tr.Seq})
	if err != nil {
		slog.Debug("panel ask: the panel's log cannot be read; it waits on", "panel", tr.ID, "err", err)
		return panel.Input{}, false
	}
	for _, r := range out.Records {
		if r.Input != nil && r.Input.Panel == tr.ID && r.After >= tr.Seq && r.Input.Validate() == nil {
			return *r.Input, true
		}
	}
	return panel.Input{}, false
}

// panelEvent reads a panel step's body as its event (not numbered yet).
func panelEvent(action string, spec map[string]any) (panel.Event, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return panel.Event{}, err
	}
	var e panel.Event
	switch action {
	case "open":
		e = panel.Event{Kind: panel.KindOpen, Open: &panel.Open{}}
		err = json.Unmarshal(raw, e.Open)
	case "progress":
		e = panel.Event{Kind: panel.KindProgress, Progress: &panel.Progress{}}
		err = json.Unmarshal(raw, e.Progress)
	case "ask":
		e = panel.Event{Kind: panel.KindAsk, Ask: &panel.Ask{}}
		err = json.Unmarshal(raw, e.Ask)
	case "complete":
		e = panel.Event{Kind: panel.KindComplete, Complete: &panel.Complete{}}
		err = json.Unmarshal(raw, e.Complete)
	default:
		return panel.Event{}, fmt.Errorf("unknown panel action %q", action)
	}
	if err != nil {
		return panel.Event{}, fmt.Errorf("its %s: %w", action, err)
	}
	return e, nil
}

// panelInputOf reads the person's input from a resumed event's payload (the
// bus message as the event bridge republished it).
func panelInputOf(payload any) (panel.Input, bool) {
	var raw []byte
	switch v := payload.(type) {
	case []byte:
		raw = v
	case json.RawMessage:
		raw = v
	case string:
		raw = []byte(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return panel.Input{}, false
		}
		raw = b
	}
	in, err := panel.DecodeInput(raw)
	return in, err == nil
}

func init() { registerNodeHandler(handlerPanel{}) }
