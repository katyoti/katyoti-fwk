// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"fmt"
	"time"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

type handlerRace struct{}

func (handlerRace) Kind() NodeKind { return NodeRace }

// racePhase tracks the in-flight progress of a race node: how many branches
// were spawned (pending) and how many have failed so far. A branch failure
// does NOT settle the race — failed branches are eliminated and the
// survivors keep racing (the redundant-sources use case: the cache branch
// failing must not kill the live branch). The race fails only when every
// branch has failed, subject to the node's on_error strategy.
// Stored in frame.Local so the JSON checkpoint shape stays plain.
type racePhase struct {
	Pending     int
	Failed      int
	LastFailure string
}

func loadRacePhase(f *ActivationFrame) racePhase {
	return racePhase{
		Pending:     getInt(f.Local, "pending"),
		Failed:      getInt(f.Local, "failed"),
		LastFailure: getString(f.Local, "last_failure"),
	}
}

func (p racePhase) save(f *ActivationFrame) {
	if f.Local == nil {
		f.Local = map[string]any{}
	}
	f.Local["pending"] = p.Pending
	f.Local["failed"] = p.Failed
	f.Local["last_failure"] = p.LastFailure
}

func (handlerRace) Step(
	_ context.Context, m *workflowMachine, ws *WorkflowState,
	frame *ActivationFrame, node *Node, event machine.Event,
) (machine.ActionFunc, error) {
	race := node.Race

	switch frame.Phase {
	case "": // Spawn all branches.
		return func(ctx context.Context) (*machine.StepResult, error) {
			if m.spawner == nil {
				return nil, fmt.Errorf("race: no spawner available")
			}
			spawned := 0
			for i := range race.Branches {
				branch := &race.Branches[i]
				if branch.Kind != NodeDelegate || branch.Delegate == nil {
					// Non-delegate branches are not supported (yet) inside race —
					// silently dropping one (e.g. a documented kind:wait
					// timeout branch) would change the race's meaning without
					// any signal to the author.
					return m.handleNodeError(ws, frame, node,
						fmt.Sprintf("race: branch %d (kind %s) is not a delegate — non-delegate branches are not supported", i, branch.Kind))
				}
				if _, err := m.spawnChild(ctx, ws, frame, branch.Delegate); err != nil {
					return nil, fmt.Errorf("race spawn %s: %w", branch.Delegate.ChildName, err)
				}
				spawned++
			}
			if spawned == 0 {
				return m.handleNodeError(ws, frame, node, "race: no spawnable branches")
			}
			frame.Phase = "awaiting"
			racePhase{Pending: spawned}.save(frame)
			// Its deadline is absolute: waiting again keeps it.
			if race.TimeoutMs > 0 {
				setDeadline(frame, time.Duration(race.TimeoutMs)*time.Millisecond, "race-timeout")
			}
			return &machine.StepResult{NextState: "running", Data: ws, Wait: childWait(frame)}, nil
		}, nil

	case "awaiting": // Resumed on each child event.
		return func(_ context.Context) (*machine.StepResult, error) {
			label := event.Label
			if label == "race-timeout" {
				// No branch settled within the window — cancel everything
				// still running and fail the race (on_error: fallback gives
				// the timeout-default pattern).
				m.cancelOwnChildren(frame, "", "race timed out")
				return m.handleNodeError(ws, frame, node,
					fmt.Sprintf("race: timed out after %dms with no winner", race.TimeoutMs))
			}
			if len(label) > 5 && label[:5] == "done:" {
				// First successful branch wins — cancel the losers.
				childName := label[5:]
				var output any
				if evt, ok := event.Payload.(*machine.ChildDoneEvent); ok {
					output = evt.Output
				} else {
					output = event.Payload
				}
				ws.Results[childName] = output
				m.cancelOwnChildren(frame, childName, "race lost") // its own losers only
				ws.PopFrame()
				return &machine.StepResult{NextState: "running", Data: ws}, nil
			}
			if len(label) > 7 && label[:7] == "failed:" {
				phase := loadRacePhase(frame)
				phase.Failed++
				if evt, ok := event.Payload.(*machine.ChildFailedEvent); ok && evt.Error != "" {
					phase.LastFailure = label[7:] + ": " + evt.Error
				} else {
					phase.LastFailure = label[7:]
				}
				if phase.Failed >= phase.Pending {
					// No branch left to win.
					return m.handleNodeError(ws, frame, node,
						fmt.Sprintf("race: all %d branches failed (last: %s)", phase.Pending, phase.LastFailure))
				}
				phase.save(frame)
			}
			// Survivors keep racing: the same wait goes on (WaitCustom).
			return &machine.StepResult{NextState: "running", Data: ws, KeepWaiting: true}, nil
		}, nil
	}

	return nil, fmt.Errorf("race: unexpected phase %q", frame.Phase)
}

func init() { registerNodeHandler(handlerRace{}) }
