// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// A TASK STARTED AS THE GUI DOES. A start_task step starts a
// task attended - the person is in its chat - through Terminal.StartTask, the
// GUI chat's own call: its ask-user-data forms then wait in the chat, not in
// the Inbox. An answer_form step reads the session's user journal for the
// form it shows and not yet answered, and submits its value through
// Terminal.Cmd, a ui_event submit on the form's artifact - what the GUI sends
// when the person presses « Envoyer ».

const (
	terminalStartTask = "ktw-agents:Terminal/start-task"
	terminalCmd       = "ktw-agents:Terminal/cmd"
	// defaultTaskSave is where a start_task step keeps its answer when it
	// names none, and where an answer_form step reads it.
	defaultTaskSave = "task"
)

// startTaskInput is Terminal.StartTask's input for a step: the task, its
// input, the context the GUI gives a chat (_attended, and _target on an
// object).
func startTaskInput(task, target string, input any) map[string]any {
	ctx := map[string]any{"_attended": true}
	if target != "" {
		ctx["_target"] = target
	}
	in, _ := input.(map[string]any)
	if in == nil {
		in = map[string]any{}
	}
	return map[string]any{"taskName": task, "input": in, "context": ctx, "clientLabel": "ktw-compose"}
}

// startTask starts the step's task attended and saves {runID, sessionID}.
func (e *Engine) startTask(ctx context.Context, b *Bus, step api.StartTaskStep, st *State, rep *report) error {
	target, _ := e.expand(step.Target, *st).(string)
	raw, err := b.Call(ctx, terminalStartTask, startTaskInput(step.Task, target, e.expand(step.Input, *st)))
	if err != nil {
		return fmt.Errorf("start_task %s: %w", step.Task, err)
	}
	var out struct {
		RunID     string `json:"runID"`
		SessionID string `json:"sessionID"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.RunID == "" || out.SessionID == "" {
		return fmt.Errorf("start_task %s: no run in its answer %s", step.Task, tail(string(raw), 200))
	}
	save := step.Save
	if save == "" {
		save = defaultTaskSave
	}
	if st.Saved == nil {
		st.Saved = map[string]any{}
	}
	st.Saved[save] = map[string]any{"runID": out.RunID, "sessionID": out.SessionID}
	rep.linef("  started %s attended: run %s, session %s", step.Task, out.RunID, out.SessionID)
	return nil
}

// shownForm is a form a run shows in its chat, from the session's user
// journal.
type shownForm struct {
	Emitter, ArtifactID, Question, Title string
	Seq                                  int
}

type journalLine struct {
	Type string `json:"type"`
	Seq  int    `json:"seq"`
	Data struct {
		Emitter    string `json:"emitter"`
		Question   string `json:"question"`
		Title      string `json:"title"`
		Channel    string `json:"channel"`
		ArtifactID string `json:"artifactId"`
	} `json:"data"`
}

// openForm is the last form of the user journal not yet answered (no
// user_input of its run after it) whose question or title holds question
// (any, when empty).
func openForm(journal []byte, question string) (shownForm, bool) {
	var shown []shownForm
	answered := map[string]int{} // emitter -> seq of its last answer
	sc := bufio.NewScanner(bytes.NewReader(journal))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var l journalLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		switch l.Type {
		case "prompt_shown":
			if l.Data.ArtifactID != "" && l.Data.Channel == "form" {
				shown = append(shown, shownForm{Emitter: l.Data.Emitter, ArtifactID: l.Data.ArtifactID, Question: l.Data.Question, Title: l.Data.Title, Seq: l.Seq})
			}
		case "user_input":
			answered[l.Data.Emitter] = l.Seq
		}
	}
	q := strings.ToLower(question)
	for i := len(shown) - 1; i >= 0; i-- {
		f := shown[i]
		if a, ok := answered[f.Emitter]; ok && a > f.Seq {
			continue
		}
		if q == "" || strings.Contains(strings.ToLower(f.Question), q) || strings.Contains(strings.ToLower(f.Title), q) {
			return f, true
		}
	}
	return shownForm{}, false
}

// formTaken is whether the run answered the form (a user_input of its run
// after it).
func formTaken(journal []byte, f shownForm) bool {
	sc := bufio.NewScanner(bytes.NewReader(journal))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var l journalLine
		if json.Unmarshal(sc.Bytes(), &l) == nil && l.Type == "user_input" && l.Data.Emitter == f.Emitter && l.Seq > f.Seq {
			return true
		}
	}
	return false
}

// submitCmd is Terminal.Cmd's input submitting value to the form, as the GUI
// does.
func submitCmd(runID string, f shownForm, value any) (map[string]any, error) {
	cmd, err := json.Marshal(map[string]any{"type": "ui_event", "params": map[string]any{"artifactId": f.ArtifactID, "eventType": "submit", "value": value}})
	if err != nil {
		return nil, err
	}
	return map[string]any{"runID": runID, "command": json.RawMessage(cmd)}, nil
}

// answerForm waits for the started task's open form, submits the step's
// value to it, and waits until its run took it.
func (e *Engine) answerForm(ctx context.Context, b *Bus, step api.AnswerFormStep, st *State, rep *report, deadline time.Time) error {
	if t := step.TimeoutMin; t > 0 {
		if d := time.Now().Add(time.Duration(t) * time.Minute); d.Before(deadline) {
			deadline = d
		}
	}
	name := step.Task
	if name == "" {
		name = defaultTaskSave
	}
	started, _ := st.Saved[name].(map[string]any)
	runID, _ := started["runID"].(string)
	sessionID, _ := started["sessionID"].(string)
	if runID == "" || sessionID == "" {
		return fmt.Errorf("answer_form: no task started under %q (a start_task step saves it)", name)
	}
	// The case's hub keeps its sessions in the case's data (it is started
	// with KTW_DATA = RT.Data(), procs.go): read the journal there.
	journal := filepath.Join(e.RT.Sessions(), sessionID, "user.jsonl")
	read := func() []byte { b, _ := os.ReadFile(journal); return b }
	var f shownForm
	for {
		var ok bool
		if f, ok = openForm(read(), step.Question); ok {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("answer_form: no form %q shown by %s in time (%s)", step.Question, name, journal)
		}
		time.Sleep(2 * time.Second)
	}
	rep.linef("  form « %s » (%s)", firstNonEmpty(f.Question, f.Title), f.ArtifactID)
	in, err := submitCmd(runID, f, e.expand(step.Value, *st))
	if err != nil {
		return err
	}
	if _, err := b.Call(ctx, terminalCmd, in); err != nil {
		return fmt.Errorf("answer_form: submit: %w", err)
	}
	for !formTaken(read(), f) {
		if time.Now().After(deadline) {
			return fmt.Errorf("answer_form: the run did not take the answer to « %s » in time", firstNonEmpty(f.Question, f.Title))
		}
		time.Sleep(time.Second)
	}
	rep.linef("  answered: %s", tail(jsonText(step.Value), 300))
	return nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
