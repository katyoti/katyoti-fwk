// SPDX-License-Identifier: Apache-2.0

// script.go gives the simulator a CONVERSATION: a list of steps it runs
// sequentially, driving the SAME comms service (Dimension 5) an agent does —
// with zero LLM. This makes the parent↔child channel (ask / await / answer /
// stop / notify) testable deterministically, and renders in the console exactly
// like an agent conversation because both emit the same child.* events.
//
// Step kinds (exactly one field set per step):
//   - say:      one-way message (comms.Notify) — visible in the console.
//   - ask:      ask the PARENT and block for the answer (comms.Ask, child side).
//   - wait_ms:  sleep (simulated think time).
//   - complete: finish with this output value (terminates the script).
//   - fail:     fail with this message.
//   - drive:    PARENT side — delegate a child (agent_as_human) and run the
//     await→answer loop against it (comms.Await + PendingQuestions + Answer,
//     plus Stop for a finish:driven child).

package simulator

import (
	"context"
	"fmt"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	commsapi "github.com/katyoti/katyoti-fwk/internal/extensions/comms/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// Step is one scripted action. Exactly one field is set.
type Step struct {
	Say      string     `json:"say,omitempty" yaml:"say"`
	Ask      string     `json:"ask,omitempty" yaml:"ask"`
	Options  []string   `json:"options,omitempty" yaml:"options"`
	WaitMs   int        `json:"wait_ms,omitempty" yaml:"wait_ms"`
	Complete any        `json:"complete,omitempty" yaml:"complete"`
	Fail     string     `json:"fail,omitempty" yaml:"fail"`
	Drive    *DriveStep `json:"drive,omitempty" yaml:"drive"`
}

// DriveStep is the parent side: delegate a child and steer it with scripted
// answers. The child runs concurrently (background) so the await→answer loop
// interleaves with the child's asks.
type DriveStep struct {
	Task    string         `json:"task" yaml:"task"`               // child task name
	Mode    string         `json:"mode,omitempty" yaml:"mode"`     // default agent_as_human
	Finish  string         `json:"finish,omitempty" yaml:"finish"` // self|driven, default self
	Input   map[string]any `json:"input,omitempty" yaml:"input"`
	Answers []string       `json:"answers,omitempty" yaml:"answers"` // consumed in order
	// AwaitTimeoutS bounds a single await in the drive loop. Default 30s — a
	// healthy child event wakes it far sooner; the bound just stops a hang.
	AwaitTimeoutS int `json:"await_timeout_s,omitempty" yaml:"await_timeout_s"`
}

// runScript executes the steps in order, returning the `complete` value (or a
// transcript if the script ends without one). A `fail` step returns an error.
func runScript(ctx context.Context, cfg SimulatorConfig, runtime taskapi.RuntimeContext) (any, error) {
	svc, err := application.UseService[commsapi.Service](ctx)
	if err != nil {
		return nil, fmt.Errorf("simulator script: comms service unavailable: %w", err)
	}
	transcript := make([]map[string]any, 0, len(cfg.Script))

	for i, step := range cfg.Script {
		switch {
		case step.Say != "":
			_ = svc.Notify(ctx, commsapi.NotifyRequest{Text: step.Say, ParentRunID: runtime.ParentRunID})
			transcript = append(transcript, map[string]any{"say": step.Say})

		case step.Ask != "":
			ans, err := svc.Ask(ctx, commsapi.AskRequest{
				Question:    step.Ask,
				Options:     step.Options,
				ParentRunID: runtime.ParentRunID,
			})
			if err != nil {
				return nil, fmt.Errorf("step %d (ask): %w", i, err)
			}
			transcript = append(transcript, map[string]any{"ask": step.Ask, "answer": ans.Text})

		case step.WaitMs > 0:
			select {
			case <-time.After(time.Duration(step.WaitMs) * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}

		case step.Fail != "":
			return nil, fmt.Errorf("%s", step.Fail)

		case step.Drive != nil:
			res, err := runDrive(ctx, svc, *step.Drive, runtime)
			if err != nil {
				return nil, fmt.Errorf("step %d (drive %s): %w", i, step.Drive.Task, err)
			}
			transcript = append(transcript, res)

		case step.Complete != nil:
			return step.Complete, nil
		}
	}

	return map[string]any{"simulator": true, "scripted": true, "transcript": transcript}, nil
}

// runDrive delegates a child in agent_as_human mode and runs the await→answer
// loop — the parent-side conversation, scripted. Mirrors the delegate tool's
// agent_as_human spawn (bgManager.Register) so the child runs concurrently and
// its bubbled questions route back here.
func runDrive(ctx context.Context, svc commsapi.Service, d DriveStep, runtime taskapi.RuntimeContext) (map[string]any, error) {
	ts, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		return nil, fmt.Errorf("comms drive: TaskService unavailable: %w", err)
	}
	parentID := engine.RunIDFromContext(ctx)
	mode := d.Mode
	if mode == "" {
		mode = "agent_as_human"
	}
	finish := d.Finish
	if finish == "" {
		finish = string(presets.FinishSelf)
	}
	timeoutS := d.AwaitTimeoutS
	if timeoutS <= 0 {
		timeoutS = 30
	}

	override := presets.BuildChildOverride(presets.ModeBackground, presets.ModeBackground)
	override[presets.OverrideKeyUserCommsInput] = string(presets.InputBubbleToParent)
	override[presets.OverrideKeyFinish] = finish
	maxDurS, _ := presets.SchedulingFieldsFromMode(presets.ModeBackground)

	taskID, err := ts.BackgroundManager().Register(parentID, taskapi.TaskRunRequest{
		TaskName:       d.Task,
		Input:          d.Input,
		ParentRunID:    parentID,
		SessionID:      runtime.SessionID,
		WorkspaceRoot:  runtime.WorkspaceRoot,
		ConfigOverride: override,
		MaxDurationS:   maxDurS,
	})
	if err != nil {
		return nil, fmt.Errorf("delegate %q: %w", d.Task, err)
	}

	exchanges := make([]map[string]any, 0, len(d.Answers))
	answerIdx := 0

	for {
		ev, err := svc.Await(ctx, commsapi.AwaitRequest{
			ChildTaskID: taskID,
			ParentRunID: parentID,
			TimeoutS:    timeoutS,
		})
		if err != nil {
			return nil, err
		}
		switch ev.Kind {
		case commsapi.EventAsked:
			for _, q := range svc.PendingQuestions(ctx, parentID) {
				answer := "ok"
				if answerIdx < len(d.Answers) {
					answer = d.Answers[answerIdx]
					answerIdx++
				}
				res, _ := svc.Answer(ctx, commsapi.AnswerRequest{QuestionID: q.ID, Answer: answer})
				exchanges = append(exchanges, map[string]any{"q": q.Question, "a": answer, "delivered": res.Delivered})
			}
			// A driven child never self-completes — once the scripted answers are
			// spent, the parent stops it (the finish:driven terminator).
			if finish == string(presets.FinishDriven) && answerIdx >= len(d.Answers) {
				_ = svc.Stop(ctx, commsapi.StopRequest{ChildTaskID: taskID, Reason: "drive: scripted answers exhausted"})
				return map[string]any{"drive": d.Task, "exchanges": exchanges, "ended": "stopped"}, nil
			}

		case commsapi.EventCompleted:
			return map[string]any{"drive": d.Task, "exchanges": exchanges, "ended": "completed"}, nil

		case commsapi.EventFailed:
			return map[string]any{"drive": d.Task, "exchanges": exchanges, "ended": "failed"}, nil

		case commsapi.EventTimeout:
			// The child went quiet past the bound — surface it rather than hang.
			return map[string]any{"drive": d.Task, "exchanges": exchanges, "ended": "timeout"}, nil

		case commsapi.EventCancelled:
			return nil, ctx.Err()
		}
	}
}
