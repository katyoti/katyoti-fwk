// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// A STEP'S CHILDREN — a delegate, parallel or race step starts children and
// waits for them. Each child has a stable id: its site (the step's node, the
// iterations of the loops around it, its attempt) with the workflow's run
// gives the same run in every process, so a step started again after a
// restart finds its children instead of starting them twice. The step keeps
// the children it started (frame.Local["children"]: name → run) and waits on
// their runs' ends — its own runs, never a name another run of the workflow
// could share. What woke it is never trusted: the step settles each child
// from how it ended (Spawner.Outcome), so an end published before it listened,
// or while its process was down, is still seen.

const (
	childrenKey = "children"   // frame.Local: name → run id of the children it started
	settledKey  = "settled"    // frame.Local: name → true once its end was handed to the step
	spawnsKey   = "spawns"     // frame.Local: name → how many times the step started it
	deadlineKey = "deadlineMs" // frame.Local: the step's absolute deadline (unix ms), when it has one
	timeoutKey  = "timeoutLabel"
	endedLabel  = "ended:"
)

// childSite names where a step starts a child: the step's node, the child's
// name, the iterations of the loops around the step, the attempt.
func childSite(ws *WorkflowState, nodeID, name string, attempt int) string {
	var iterations []string
	for i := 0; i < len(ws.Active)-1; i++ {
		f := &ws.Active[i]
		switch f.Kind {
		case NodeLoop, NodeWhile:
			iterations = append(iterations, fmt.Sprintf("%s=%d", f.NodeID, getInt(f.Local, "iteration")))
		case NodeForeach:
			iterations = append(iterations, fmt.Sprintf("%s=%d", f.NodeID, getInt(f.Local, "index")))
		}
	}
	return fmt.Sprintf("%s|%s|i=%s|a=%d", nodeID, name, strings.Join(iterations, ","), attempt)
}

// spawnChild starts one of a step's children (a delegate) at its site and
// records it: in the workflow's children by name, and in the step's own. Its
// attempt is how many times the step started that name before — a retry, of
// the child or of the whole step, is a new run; a step replayed from its
// checkpoint counts the same and finds the run it started.
func (m *workflowMachine) spawnChild(ctx context.Context, ws *WorkflowState, frame *ActivationFrame, d *DelegateNode) (machine.ChildHandle, error) {
	attempt := spawnsOf(frame, d.ChildName)
	childKind, childConfig, err := resolveDelegateTarget(d)
	if err != nil {
		return machine.ChildHandle{}, err
	}
	// v1 dimension param: budget rides the child's config (consumed by the
	// agent strategy's MaxBudgetUSD decode) — agent-flavoured kinds only.
	if d.MaxBudgetUSD > 0 && (childKind == "agent" || childKind == "task") {
		merged := make(map[string]any, len(childConfig)+1)
		for k, v := range childConfig {
			merged[k] = v
		}
		merged[presets.OverrideKeyMaxBudgetUSD] = d.MaxBudgetUSD
		childConfig = merged
	}
	resolvedInput, err := m.resolveInputBinds(d.InputBinds, ws)
	if err != nil {
		return machine.ChildHandle{}, fmt.Errorf("%s: %w", d.ChildName, err)
	}
	handle, err := m.spawner.Spawn(ctx, machine.ChildSpec{
		Kind:                   childKind,
		Name:                   d.ChildName,
		Config:                 childConfig,
		Input:                  resolvedInput,
		Body:                   d.Body,
		ParentID:               m.runID, // its parent: lineage, journal, the stable id
		Site:                   childSite(ws, frame.NodeID, d.ChildName, attempt),
		Mode:                   d.Mode,          // the step's mode: spawn policies
		ParentMode:             m.mode,          // the workflow's mode (UC-053/054)
		ParentSpawnPolicyNames: m.spawnPolicies, // sticky policies, forwarded (UC-054)
		DryRunMode:             m.dryRunMode,
		CurrentNode:            m.currentNode, // kind:node inheritance
		MaxDurationS:           d.MaxDurationS,
	})
	if err != nil {
		return machine.ChildHandle{}, err
	}
	ws.Children[d.ChildName] = handle
	addFrameChild(frame, d.ChildName, handle.RunID)
	countSpawn(frame, d.ChildName)
	return handle, nil
}

// spawnsOf is how many times the step started a child of that name.
func spawnsOf(f *ActivationFrame, name string) int {
	switch v := f.Local[spawnsKey].(type) {
	case map[string]int:
		return v[name]
	case map[string]any: // read back from a checkpoint
		return getInt(v, name)
	}
	return 0
}

func countSpawn(f *ActivationFrame, name string) {
	if f.Local == nil {
		f.Local = map[string]any{}
	}
	out := map[string]any{}
	switch v := f.Local[spawnsKey].(type) {
	case map[string]int:
		for k, n := range v {
			out[k] = n
		}
	case map[string]any:
		for k, n := range v {
			out[k] = n
		}
	}
	out[name] = spawnsOf(f, name) + 1
	f.Local[spawnsKey] = out
}

// frameChildren is the children a step started: name → run id.
func frameChildren(f *ActivationFrame) map[string]string {
	out := map[string]string{}
	switch v := f.Local[childrenKey].(type) {
	case map[string]string:
		for k, id := range v {
			out[k] = id
		}
	case map[string]any: // read back from a checkpoint
		for k, id := range v {
			if s, ok := id.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}

func addFrameChild(f *ActivationFrame, name, runID string) {
	if f.Local == nil {
		f.Local = map[string]any{}
	}
	c := frameChildren(f)
	c[name] = runID
	f.Local[childrenKey] = c
	s := settledChildren(f)
	delete(s, name) // a child started again (a retry) is to be settled again
	f.Local[settledKey] = s
}

func settledChildren(f *ActivationFrame) map[string]bool {
	out := map[string]bool{}
	switch v := f.Local[settledKey].(type) {
	case map[string]bool:
		for k, b := range v {
			out[k] = b
		}
	case map[string]any:
		for k, b := range v {
			if ok, _ := b.(bool); ok {
				out[k] = true
			}
		}
	}
	return out
}

func markSettled(f *ActivationFrame, name string) {
	s := settledChildren(f)
	s[name] = true
	f.Local[settledKey] = s
}

// now is the clock a step's deadline is set and read on (a test moves it).
var now = time.Now

// setDeadline gives the step an absolute deadline: waiting again keeps it.
func setDeadline(f *ActivationFrame, d time.Duration, label string) {
	if f.Local == nil {
		f.Local = map[string]any{}
	}
	f.Local[deadlineKey] = now().Add(d).UnixMilli()
	f.Local[timeoutKey] = label
}

// deadlineLeft is what is left of the step's deadline — at least a
// nanosecond once passed, so the timeout fires at once; ok false when there
// is no frame or it holds none (a frame saved before its step kept one).
func deadlineLeft(f *ActivationFrame) (left time.Duration, label string, ok bool) {
	if f == nil {
		return 0, "", false
	}
	ms := getInt64(f.Local, deadlineKey)
	if ms <= 0 {
		return 0, "", false
	}
	left = time.UnixMilli(ms).Sub(now())
	if left <= 0 {
		left = time.Nanosecond
	}
	return left, getString(f.Local, timeoutKey), true
}

// childWait is the wait of a step awaiting its children: the end of each one
// not settled yet — rechecked when it is armed (an end published before), and
// the step's deadline if it has one.
func childWait(f *ActivationFrame) *machine.WaitSpec {
	spec := &machine.WaitSpec{Mode: machine.WaitCustom, RecheckOnResume: true}
	children := frameChildren(f)
	settled := settledChildren(f)
	names := make([]string, 0, len(children))
	for name := range children {
		if !settled[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		spec.Conditions = append(spec.Conditions, machine.WaitCondition{Topic: machine.RunEndedTopic(children[name]), Label: endedLabel + name})
	}
	if left, label, ok := deadlineLeft(f); ok {
		spec.Timeout, spec.TimeoutLabel = left, label
	}
	return spec
}

// awaitsChildren reports whether frame is a step waiting for children it
// started and keeps (a parallel, race or delegate in its awaiting phase).
func awaitsChildren(f *ActivationFrame) bool {
	switch f.Kind {
	case NodeDelegate, NodeParallel, NodeRace:
		return f.Phase == "awaiting" && len(frameChildren(f)) > 0
	}
	return false
}

// childEvent turns what woke a step awaiting its children into the event its
// handler reads — "done" / "failed" (a delegate), "done:<name>" /
// "failed:<name>" (parallel, race) — for one child that ended, settled from
// its outcome. ok false: none ended that the step has not seen yet; the step
// waits again. A timeout passes through.
func (m *workflowMachine) childEvent(ws *WorkflowState, f *ActivationFrame, event machine.Event) (machine.Event, bool) {
	if event.Label != "" && event.Label == getString(f.Local, timeoutKey) {
		return event, true
	}
	children := frameChildren(f)
	settled := settledChildren(f)
	names := make([]string, 0, len(children))
	if name, ok := strings.CutPrefix(event.Label, endedLabel); ok && !settled[name] {
		names = append(names, name) // what woke it, first
	}
	rest := make([]string, 0, len(children))
	for name := range children {
		if !settled[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	names = append(names, rest...)
	for _, name := range names {
		outcome, ended := m.spawner.Outcome(children[name])
		if !ended {
			continue
		}
		markSettled(f, name)
		if h, ok := ws.Children[name]; ok {
			h.Status = outcome.Status
			ws.Children[name] = h
		}
		suffix := ""
		if f.Kind != NodeDelegate {
			suffix = ":" + name
		}
		if outcome.Status == machine.ChildDone {
			return machine.Event{Type: event.Type, Label: "done" + suffix, Payload: &machine.ChildDoneEvent{ChildName: name, RunID: children[name], Output: outcome.Output}}, true
		}
		return machine.Event{Type: event.Type, Label: "failed" + suffix, Payload: &machine.ChildFailedEvent{ChildName: name, RunID: children[name], Error: outcome.Error}}, true
	}
	return event, false
}

// cancelOwnChildren stops the children a step started that have not ended —
// never another step's (a background delegate goes on).
func (m *workflowMachine) cancelOwnChildren(f *ActivationFrame, except, reason string) {
	settled := settledChildren(f)
	for name, id := range frameChildren(f) {
		if name == except || settled[name] {
			continue
		}
		_ = m.spawner.Cancel(id, reason)
	}
}

// getInt64 reads a number from a frame's Local (an int64 written, a float64
// read back from a checkpoint).
func getInt64(m map[string]any, key string) int64 {
	switch v := m[key].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}
