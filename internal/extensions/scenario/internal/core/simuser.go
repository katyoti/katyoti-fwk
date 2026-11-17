// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// THE SIMULATED USER — the person the case describes (its user.md), answering
// what the platform asks in the Inbox: a decision to take, a review to give, a
// fact to provide. Each pending request is answered by one run of the
// `scenario-user` task (an agent playing the persona) and the answer is given
// through the inbox service, as a person's answer in the GUI would be. A case
// whose user file scripts its answers (`answers:` in its front matter) is
// answered from them instead — no agent run, no LLM. Every question and answer
// goes into the run's report.

// SimUserTask is the task that plays the persona.
const SimUserTask = "scenario-user"

//go:embed simuser_task.md
var simUserTaskMD []byte

// approvalRequest is the inbox's object type.
const approvalRequest = "approval-request"

// answerInbox answers pending Inbox requests until the step's check holds.
func (e *Engine) answerInbox(ctx context.Context, b *Bus, step api.AnswerInboxStep, st *State, rep *report, deadline time.Time) error {
	if t := step.TimeoutMin; t > 0 {
		if d := time.Now().Add(time.Duration(t) * time.Minute); d.Before(deadline) {
			deadline = d
		}
	}
	persona := e.persona()
	answers := 0
	for {
		if step.Until != nil {
			if ok, _ := e.check(ctx, b, *step.Until, *st); ok {
				rep.linef("  until holds: %s", step.Until.Describe())
				return nil
			}
		}
		if step.Max > 0 && answers >= step.Max {
			return fmt.Errorf("answered %d requests (max) and %s does not hold", answers, describeUntil(step.Until))
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s (%d answers given)", describeUntil(step.Until), answers)
		}
		pending, err := e.pendingRequests(ctx, b, st)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			if step.Until == nil && answers > 0 {
				return nil // nothing more asked, and no condition to wait for
			}
			time.Sleep(10 * time.Second)
			continue
		}
		for _, q := range pending {
			if err := e.answerOne(ctx, b, q, persona, st, rep); err != nil {
				return err
			}
			answers++
			_ = saveState(e.RT, *st)
		}
	}
}

func describeUntil(c *api.Check) string {
	if c == nil {
		return "(no condition)"
	}
	return c.Describe()
}

// pendingRequests are the Inbox requests still pending and not yet answered,
// as the inbox lists what waits (its triage).
func (e *Engine) pendingRequests(ctx context.Context, b *Bus, st *State) ([]map[string]any, error) {
	raw, err := b.Call(ctx, "ktw-agents:inbox/Triage", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	var pending []map[string]any
	for _, q := range out.Entries {
		status := str(q["status"])
		if str(q["type"]) == approvalRequest && (status == "" || status == "pending") && !slices.Contains(st.Answered, str(q["id"])) {
			pending = append(pending, q)
		}
	}
	return pending, nil
}

// subjectOf is what a request asks about, as a person opens it from the
// Inbox: the object its asker names (`<asker>:<object id>`); "" when none.
func subjectOf(ctx context.Context, b *Bus, requestedBy string) string {
	_, id, ok := strings.Cut(requestedBy, ":")
	if !ok || id == "" {
		return ""
	}
	module, _, ok := strings.Cut(id, ".")
	if !ok {
		return ""
	}
	raw, err := b.Call(ctx, "ktw-agents:cwm/ObjectGet", map[string]any{"module": module, "id": id})
	if err != nil {
		return ""
	}
	const max = 12000
	if len(raw) > max {
		raw = raw[:max]
	}
	return "The object this is about (" + id + "):\n" + string(raw)
}

// answerOne has the persona answer one request, and gives the answer.
func (e *Engine) answerOne(ctx context.Context, b *Bus, q map[string]any, persona string, st *State, rep *report) error {
	id := str(q["id"])
	question := str(q["title"])
	rep.linef("  INBOX %s — %s", id, question)
	if len(e.Case.Answers) > 0 {
		i := scriptedAnswer(e.Case.Answers, st.Scripted, question+"\n"+str(q["description"]))
		if i < 0 {
			return fmt.Errorf("no scripted answer of the user matches %q", question)
		}
		a := e.Case.Answers[i]
		rep.linef("    user answered (scripted %d): %s — %s", i+1, a.Decision, a.Note)
		st.Scripted = append(st.Scripted, i)
		return e.giveAnswer(ctx, b, id, a.Decision, a.Note, st)
	}
	ctxText := strings.TrimSpace(str(q["description"]) + "\n\n" + subjectOf(ctx, b, str(q["requested_by"])))
	if hint := expectsHint(str(q["expects"])); hint != "" {
		ctxText = hint + "\n\n" + ctxText
	}
	decision, note, err := e.askPersona(ctx, b, persona, question, ctxText, rep)
	if err != nil {
		return err
	}
	err = e.giveAnswer(ctx, b, id, decision, note, st)
	if err == nil {
		return nil
	}
	// Refused (a rejection without a note, a question rejected or left
	// empty): the persona is asked once more, told why; refused again, the
	// stage fails with the refusal.
	rep.linef("    the inbox refused it: %v", err)
	again := "Your answer was refused: " + err.Error() +
		"\n\nA rejection says in the note what to change; a question is answered in the note, never rejected.\n\n" + ctxText
	if decision, note, err = e.askPersona(ctx, b, persona, question, again, rep); err != nil {
		return err
	}
	return e.giveAnswer(ctx, b, id, decision, note, st)
}

// expectsHint tells the persona what kind of answer a request expects (its
// expects: verdict or text); "" for a request that does not say.
func expectsHint(expects string) string {
	switch expects {
	case "verdict":
		return "This asks for your verdict: approved, or rejected with a note saying what to change."
	case "text":
		return "This asks for your answer: write it in the note (decision approved); it is never rejected."
	}
	return ""
}

// askPersona runs the persona on one question and reads its decision and note.
func (e *Engine) askPersona(ctx context.Context, b *Bus, persona, question, ctxText string, rep *report) (string, string, error) {
	o, err := b.Action(ctx, "ktw-agents:tasks/"+SimUserTask, map[string]any{"persona": persona, "question": question, "context": ctxText})
	if err != nil {
		return "", "", fmt.Errorf("the simulated user could not start: %w", err)
	}
	if o.TaskID == "" {
		return "", "", fmt.Errorf("the simulated user started no run")
	}
	result, err := e.waitRun(ctx, b, o.TaskID, 15*time.Minute)
	if err != nil {
		return "", "", fmt.Errorf("the simulated user (run %s): %w", o.TaskID, err)
	}
	decision, note := parseAnswer(result)
	rep.linef("    user answered: %s — %s (run %s)", decision, note, o.TaskID)
	return decision, note, nil
}

// giveAnswer answers a request through the inbox, as a person's answer.
func (e *Engine) giveAnswer(ctx context.Context, b *Bus, id, decision, note string, st *State) error {
	if _, err := b.Call(ctx, "ktw-agents:inbox/Answer", map[string]any{"id": id, "decision": decision, "note": note, "by": "sim-user"}); err != nil {
		return fmt.Errorf("answer %s: %w", id, err)
	}
	st.Answered = append(st.Answered, id)
	return nil
}

// scriptedAnswer is the first scripted answer not given yet whose When the
// question contains (case-insensitive; an empty When matches any); -1 when none.
func scriptedAnswer(answers []api.UserAnswer, used []int, question string) int {
	q := strings.ToLower(question)
	for i, a := range answers {
		if !slices.Contains(used, i) && strings.Contains(q, strings.ToLower(a.When)) {
			return i
		}
	}
	return -1
}

// waitRun waits for a run to end and answers its output.
func (e *Engine) waitRun(ctx context.Context, b *Bus, run string, timeout time.Duration) (any, error) {
	deadline := time.Now().Add(timeout)
	for {
		raw, err := b.Call(ctx, "ktw-agents:tasks/Status", map[string]any{"runId": run})
		if err == nil {
			var s struct {
				Status string `json:"status"`
			}
			_ = json.Unmarshal(raw, &s)
			switch s.Status {
			case "completed", "failed", "cancelled", "killed":
				raw, err := b.Call(ctx, "ktw-agents:tasks/Result", map[string]any{"runId": run})
				if err != nil {
					return nil, err
				}
				var r struct {
					Status string `json:"status"`
					Output any    `json:"output"`
					Error  string `json:"error"`
				}
				_ = json.Unmarshal(raw, &r)
				if s.Status != "completed" {
					return nil, fmt.Errorf("run %s %s: %s", run, s.Status, r.Error)
				}
				return r.Output, nil
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("run %s still going after %s", run, timeout)
		}
		time.Sleep(3 * time.Second)
	}
}

// parseAnswer reads {decision, note} from a run's output, wherever it sits.
func parseAnswer(out any) (string, string) {
	var walk func(v any) (string, string, bool)
	walk = func(v any) (string, string, bool) {
		switch x := v.(type) {
		case map[string]any:
			d, dok := x["decision"].(string)
			n, nok := x["note"].(string)
			if dok || nok {
				return d, n, true
			}
			for _, k := range []string{"output", "result", "data", "value"} {
				if inner, ok := x[k]; ok {
					if d, n, ok := walk(inner); ok {
						return d, n, true
					}
				}
			}
		case string:
			var m any
			if json.Unmarshal([]byte(x), &m) == nil {
				if d, n, ok := walk(m); ok {
					return d, n, true
				}
			}
			return "", x, true
		}
		return "", "", false
	}
	d, n, _ := walk(out)
	if d == "" {
		d = "approved"
	}
	return d, n
}

// persona is the case's user description.
func (e *Engine) persona() string {
	if e.Case.User == "" {
		return "You are the project's owner. Answer plainly and briefly."
	}
	raw, err := os.ReadFile(filepath.Join(e.Case.Dir, e.Case.User))
	if err != nil {
		return "You are the project's owner. Answer plainly and briefly."
	}
	return string(raw)
}
