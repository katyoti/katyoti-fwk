// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// nodeCounter generates unique node IDs within a workflow.
var nodeCounter atomic.Int64

// Resource caps enforced at loader time to prevent a malicious or buggy
// workflow YAML from requesting unbounded resources (R6 — audit
// security.md high-finding-2). Caps are mutable package-level vars so
// tests can tighten them and a future Settings.yaml block
// (tasks.workflow.caps.*) can override the defaults at boot.
var (
	MaxLoopTimes          = 10_000 // upper bound on `loop.times`
	MaxWhileIterations    = 10_000 // upper bound on `while.max_iterations`
	MaxParallelBranches   = 64     // upper bound on `parallel.branches` count
	MaxForeachConcurrency = 64     // upper bound on `foreach.max_concurrency`
	MinForeachConcurrency = 1      // lower bound — 0 is an explicit error for parallel mode

	DefaultCallTimeoutMs = 60_000  // `call.timeout_ms` default when absent — a blocking invocation is never unbounded
	MaxCallTimeoutMs     = 600_000 // upper bound on `call.timeout_ms` (10 min)
)

func nextNodeID(prefix string) string {
	n := nodeCounter.Add(1)
	return fmt.Sprintf("%s_%d", prefix, n)
}

// LoadNodes converts a workflow config map into a Node tree.
// The config must contain a "steps" key with an array of step definitions.
// The result is always a Sequence node wrapping the steps.
func LoadNodes(config map[string]any) (*Node, error) {
	stepsRaw, ok := config["steps"]
	if !ok {
		return nil, fmt.Errorf("workflow config: missing 'steps'")
	}

	rawList, ok := stepsRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("workflow config: steps must be a list, got %T", stepsRaw)
	}

	steps := make([]Node, 0, len(rawList))
	for i, item := range rawList {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("step[%d]: must be a map, got %T", i, item)
		}
		node, err := loadStepNode(m)
		if err != nil {
			return nil, fmt.Errorf("step[%d]: %w", i, err)
		}
		steps = append(steps, *node)
	}

	root := &Node{
		ID:   nextNodeID("seq"),
		Kind: NodeSequence,
		Sequence: &SequenceNode{
			Steps: steps,
		},
	}
	assignStableIDs(root, "w")
	return root, nil
}

// assignStableIDs names every node after its place in the tree ("dlg@w.1",
// "seq@w.2.then"): the same workflow, loaded in any process, has the same
// node ids — a saved workflow's frames name nodes its restored machine has.
// (The load counter gave ids that depended on what the process had loaded
// before: a workflow restored after a restart pointed at nodes that did not
// exist.) Its walk is indexTree's.
func assignStableIDs(n *Node, path string) {
	prefix, _, _ := strings.Cut(n.ID, "_")
	n.ID = prefix + "@" + path
	if n.Sequence != nil {
		for i := range n.Sequence.Steps {
			assignStableIDs(&n.Sequence.Steps[i], fmt.Sprintf("%s.%d", path, i))
		}
	}
	if n.Parallel != nil {
		for i := range n.Parallel.Branches {
			assignStableIDs(&n.Parallel.Branches[i], fmt.Sprintf("%s.%d", path, i))
		}
	}
	if n.Race != nil {
		for i := range n.Race.Branches {
			assignStableIDs(&n.Race.Branches[i], fmt.Sprintf("%s.%d", path, i))
		}
	}
	if n.If != nil {
		assignStableIDs(&n.If.Then, path+".then")
		if n.If.Else != nil {
			assignStableIDs(n.If.Else, path+".else")
		}
	}
	if n.Switch != nil {
		keys := make([]string, 0, len(n.Switch.Cases))
		for k := range n.Switch.Cases {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			c := n.Switch.Cases[k]
			assignStableIDs(&c, path+".case:"+k)
			n.Switch.Cases[k] = c // cases are held by value
		}
		if n.Switch.Default != nil {
			assignStableIDs(n.Switch.Default, path+".default")
		}
	}
	if n.Loop != nil {
		assignStableIDs(&n.Loop.Body, path+".body")
	}
	if n.While != nil {
		assignStableIDs(&n.While.Body, path+".body")
	}
	if n.Foreach != nil {
		assignStableIDs(&n.Foreach.Body, path+".body")
	}
	if n.OnError != nil && n.OnError.Fallback != nil {
		assignStableIDs(n.OnError.Fallback, path+".fallback")
	}
}

// loadStepNode converts a single step map into a Node.
// A step with a "task" block (or "taskName") and no "kind" is sugar for a
// Delegate node (the former implicit spawn). Explicit kinds dispatch to
// their loaders.
func loadStepNode(m map[string]any) (*Node, error) {
	name := strVal(m, "name")

	// Check for explicit kind.
	if kind, ok := m["kind"].(string); ok {
		switch NodeKind(kind) {
		case NodeReturn:
			return &Node{
				ID:      nextNodeID("ret"),
				Kind:    NodeReturn,
				Return:  &ReturnNode{Value: m["value"]},
				OnError: loadOnError(m),
			}, nil
		case NodeDelegate:
			return loadDelegateNode(m)
		case NodeCall:
			return loadCallNode(m)
		case NodeSequence:
			return loadSequenceNode(m)
		case NodeParallel:
			return loadParallelNode(m)
		case NodeRace:
			return loadRaceNode(m)
		case NodeIf:
			return loadIfNode(m)
		case NodeSwitch:
			return loadSwitchNode(m)
		case NodeLoop:
			return loadLoopNode(m)
		case NodeWhile:
			return loadWhileNode(m)
		case NodeForeach:
			return loadForeachNode(m)
		case NodeWait:
			return loadWaitNode(m)
		case NodeAssign:
			return loadAssignNode(m)
		case NodePanel:
			return loadPanelNode(m)
		}
	}

	// Default: step with a "task"/"taskName" → Delegate node (sugar).
	if _, hasTask := m["task"].(map[string]any); !hasTask {
		if _, hasTaskName := m["taskName"].(string); !hasTaskName {
			return nil, fmt.Errorf("step %q: must have 'task' (inline) or 'taskName' (reference)", name)
		}
	}
	return loadDelegateNode(m)
}

// loadDelegateNode loads the delegate primitive — explicit `kind: delegate`
// or the implicit `task:` step sugar. Exactly ONE addressing form: `uri:`
// (action URI), `taskName:` (registry reference), or `task:` (inline
// definition).
func loadDelegateNode(m map[string]any) (*Node, error) {
	name := strVal(m, "name")

	taskDef, hasTask := m["task"].(map[string]any)
	taskName, hasTaskName := m["taskName"].(string)
	uri := strVal(m, "uri")

	forms := 0
	if hasTask {
		forms++
	}
	if hasTaskName {
		forms++
	}
	if uri != "" {
		forms++
	}
	if forms != 1 {
		return nil, fmt.Errorf("delegate %q: exactly one of 'uri', 'taskName', or 'task' is required (got %d)", name, forms)
	}

	d := &DelegateNode{
		ChildName:    name,
		Mode:         strVal(m, "mode"),
		MaxDurationS: intVal(m, "max_duration_s"),
		MaxBudgetUSD: floatVal(m, "max_budget_usd"),
	}
	if err := checkStepMode(name, d.Mode); err != nil {
		return nil, err
	}

	switch {
	case hasTask:
		// Inline task: extract type and config fields.
		d.ChildKind = strVal(taskDef, "type")
		d.Config = make(map[string]any)
		for k, v := range taskDef {
			if k != "type" {
				d.Config[k] = v
			}
		}
	case hasTaskName:
		// Named task reference — resolved at runtime.
		d.ChildKind = "task"
		d.TaskName = taskName
		d.Config = map[string]any{"_taskName": taskName}
	default:
		// Action URI — resolved at runtime through ActionService; must be
		// an async (task) action. Sync actions belong to `kind: call`.
		d.URI = uri
	}

	// Step-level input bindings.
	if inputMap, ok := m["input"].(map[string]any); ok {
		d.InputBinds = inputMap
	}

	if name == "" {
		name = d.ChildKind
	}
	d.ChildName = name

	return &Node{
		ID:       nextNodeID("dlg"),
		Kind:     NodeDelegate,
		Delegate: d,
		OnError:  loadOnError(m),
	}, nil
}

// loadCallNode loads the call primitive: ONE synchronous action invocation.
// Dimension params are delegate-only — their presence on a call is an error,
// not a silent ignore.
func loadCallNode(m map[string]any) (*Node, error) {
	name := strVal(m, "name")
	uri := strVal(m, "uri")
	if uri == "" {
		return nil, fmt.Errorf("call %q: missing 'uri'", name)
	}
	for _, dim := range []string{"mode", "max_duration_s", "max_budget_usd", "task", "taskName"} {
		if _, present := m[dim]; present {
			return nil, fmt.Errorf("call %q: %q is not valid on a call — dimensions and task forms belong to 'delegate'", name, dim)
		}
	}
	timeout := intVal(m, "timeout_ms")
	if timeout <= 0 {
		timeout = DefaultCallTimeoutMs
	}
	if timeout > MaxCallTimeoutMs {
		return nil, fmt.Errorf("call %q: 'timeout_ms' %d exceeds cap %d", name, timeout, MaxCallTimeoutMs)
	}
	if name == "" {
		return nil, fmt.Errorf("call: missing 'name' (it keys the result and the call.* events)")
	}
	c := &CallNode{Name: name, URI: uri, TimeoutMs: timeout}
	if inputMap, ok := m["input"].(map[string]any); ok {
		c.InputBinds = inputMap
	}
	return &Node{
		ID:      nextNodeID("call"),
		Kind:    NodeCall,
		Call:    c,
		OnError: loadOnError(m),
	}, nil
}

// loadSequenceNode loads a sequence node from a step map with "steps" array.
func loadSequenceNode(m map[string]any) (*Node, error) {
	stepsRaw, ok := m["steps"].([]any)
	if !ok {
		return nil, fmt.Errorf("sequence node: missing or invalid 'steps'")
	}

	steps := make([]Node, 0, len(stepsRaw))
	for i, item := range stepsRaw {
		sm, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("sequence step[%d]: must be a map", i)
		}
		node, err := loadStepNode(sm)
		if err != nil {
			return nil, err
		}
		steps = append(steps, *node)
	}

	return &Node{
		ID:   nextNodeID("seq"),
		Kind: NodeSequence,
		Sequence: &SequenceNode{
			Steps: steps,
		},
		OnError: loadOnError(m),
	}, nil
}

// loadOnError parses the on_error field from a step map.
// Supports:
//   - string: "continue", "fail" → OnErrorDef{Strategy: ...}
//   - map with "strategy", "max_retries", "fallback"
func loadOnError(m map[string]any) *OnErrorDef {
	raw, ok := m["on_error"]
	if !ok {
		return nil
	}

	switch v := raw.(type) {
	case string:
		return &OnErrorDef{Strategy: OnErrorStrategy(v)}

	case map[string]any:
		def := &OnErrorDef{
			Strategy: OnErrorStrategy(strVal(v, "strategy")),
		}
		if retries, ok := v["max_retries"].(int); ok {
			def.MaxRetries = retries
		}
		if retries, ok := v["max_retries"].(float64); ok {
			def.MaxRetries = int(retries)
		}
		def.BackoffMs = intVal(v, "backoff_ms")
		def.MaxBackoffMs = intVal(v, "max_backoff_ms")
		if fb, ok := v["fallback"].(map[string]any); ok {
			def.Strategy = OnErrorFallback
			fbNode, err := loadStepNode(fb)
			if err == nil {
				def.Fallback = fbNode
			}
		}
		return def
	}

	return nil
}

func loadParallelNode(m map[string]any) (*Node, error) {
	branches, err := loadBranchList(m, "branches", "parallel")
	if err != nil {
		return nil, fmt.Errorf("parallel: %w", err)
	}
	if len(branches) > MaxParallelBranches {
		return nil, fmt.Errorf("parallel: 'branches' has %d entries (cap %d)", len(branches), MaxParallelBranches)
	}
	onErr := strVal(m, "on_branch_error")
	if onErr == "" {
		onErr = "fail_fast"
	}
	// Validate: only fail_fast and collect are supported.
	if onErr != "fail_fast" && onErr != "collect" {
		return nil, fmt.Errorf("parallel: invalid on_branch_error %q (must be 'fail_fast' or 'collect')", onErr)
	}
	return &Node{
		ID:       nextNodeID("par"),
		Kind:     NodeParallel,
		Parallel: &ParallelNode{Branches: branches, OnBranchError: onErr},
		OnError:  loadOnError(m),
	}, nil
}

func loadRaceNode(m map[string]any) (*Node, error) {
	branches, err := loadBranchList(m, "branches", "race")
	if err != nil {
		return nil, fmt.Errorf("race: %w", err)
	}
	timeoutMs := intVal(m, "timeout_ms")
	if timeoutMs < 0 {
		return nil, fmt.Errorf("race: 'timeout_ms' must be >= 0 (got %d)", timeoutMs)
	}
	return &Node{
		ID:      nextNodeID("race"),
		Kind:    NodeRace,
		Race:    &RaceNode{Branches: branches, TimeoutMs: timeoutMs},
		OnError: loadOnError(m),
	}, nil
}

func loadIfNode(m map[string]any) (*Node, error) {
	cond := strVal(m, "condition")
	if cond == "" {
		return nil, fmt.Errorf("if: missing 'condition'")
	}
	thenRaw, ok := m["then"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("if: missing 'then'")
	}
	thenNode, err := loadStepNode(thenRaw)
	if err != nil {
		return nil, fmt.Errorf("if.then: %w", err)
	}
	ifn := &IfNode{Condition: cond, Then: *thenNode}
	if elseRaw, ok := m["else"].(map[string]any); ok {
		elseNode, err := loadStepNode(elseRaw)
		if err != nil {
			return nil, fmt.Errorf("if.else: %w", err)
		}
		ifn.Else = elseNode
	}
	return &Node{ID: nextNodeID("if"), Kind: NodeIf, If: ifn, OnError: loadOnError(m)}, nil
}

func loadSwitchNode(m map[string]any) (*Node, error) {
	on := strVal(m, "on")
	if on == "" {
		return nil, fmt.Errorf("switch: missing 'on'")
	}
	casesRaw, ok := m["cases"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("switch: missing 'cases'")
	}
	cases := make(map[string]Node, len(casesRaw))
	for key, val := range casesRaw {
		cm, ok := val.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("switch.cases[%s]: must be a map", key)
		}
		cn, err := loadStepNode(cm)
		if err != nil {
			return nil, fmt.Errorf("switch.cases[%s]: %w", key, err)
		}
		cases[key] = *cn
	}
	sw := &SwitchNode{On: on, Cases: cases}
	if defRaw, ok := m["default"].(map[string]any); ok {
		dn, err := loadStepNode(defRaw)
		if err != nil {
			return nil, fmt.Errorf("switch.default: %w", err)
		}
		sw.Default = dn
	}
	return &Node{ID: nextNodeID("sw"), Kind: NodeSwitch, Switch: sw, OnError: loadOnError(m)}, nil
}

func loadLoopNode(m map[string]any) (*Node, error) {
	times := intVal(m, "times")
	if times <= 0 {
		return nil, fmt.Errorf("loop: 'times' must be > 0")
	}
	if times > MaxLoopTimes {
		return nil, fmt.Errorf("loop: 'times' %d exceeds cap %d", times, MaxLoopTimes)
	}
	body, err := loadBodyNode(m)
	if err != nil {
		return nil, fmt.Errorf("loop: %w", err)
	}
	return &Node{ID: nextNodeID("loop"), Kind: NodeLoop, Loop: &LoopNode{Times: times, Body: *body}, OnError: loadOnError(m)}, nil
}

func loadWhileNode(m map[string]any) (*Node, error) {
	cond := strVal(m, "condition")
	if cond == "" {
		return nil, fmt.Errorf("while: missing 'condition'")
	}
	maxIter := intVal(m, "max_iterations")
	if maxIter <= 0 {
		return nil, fmt.Errorf("while: 'max_iterations' is mandatory and must be > 0")
	}
	if maxIter > MaxWhileIterations {
		return nil, fmt.Errorf("while: 'max_iterations' %d exceeds cap %d", maxIter, MaxWhileIterations)
	}
	body, err := loadBodyNode(m)
	if err != nil {
		return nil, fmt.Errorf("while: %w", err)
	}
	return &Node{ID: nextNodeID("while"), Kind: NodeWhile, While: &WhileNode{Condition: cond, Body: *body, MaxIterations: maxIter}, OnError: loadOnError(m)}, nil
}

func loadForeachNode(m map[string]any) (*Node, error) {
	items := strVal(m, "items")
	if items == "" {
		return nil, fmt.Errorf("foreach: missing 'items'")
	}
	body, err := loadBodyNode(m)
	if err != nil {
		return nil, fmt.Errorf("foreach: %w", err)
	}
	mode := strVal(m, "mode")
	if mode == "" {
		mode = "sequential"
	}
	maxConcurrency := intVal(m, "max_concurrency")
	// For parallel mode, max_concurrency is mandatory and bounded — `0` used
	// to mean "unlimited", which lets a workflow request 100k concurrent
	// agents. Now `0` is an error and the cap is enforced (R6 — threat-T4).
	// Sequential mode ignores the field.
	if mode == "parallel" {
		if maxConcurrency < MinForeachConcurrency {
			return nil, fmt.Errorf("foreach: 'max_concurrency' is mandatory for mode=parallel and must be >= %d (got %d; unlimited is no longer allowed)", MinForeachConcurrency, maxConcurrency)
		}
		if maxConcurrency > MaxForeachConcurrency {
			return nil, fmt.Errorf("foreach: 'max_concurrency' %d exceeds cap %d", maxConcurrency, MaxForeachConcurrency)
		}
	}
	return &Node{
		ID:      nextNodeID("fe"),
		Kind:    NodeForeach,
		Foreach: &ForeachNode{Items: items, Body: *body, Mode: mode, MaxConcurrency: maxConcurrency},
		OnError: loadOnError(m),
	}, nil
}

func loadWaitNode(m map[string]any) (*Node, error) {
	return &Node{
		ID:      nextNodeID("wait"),
		Kind:    NodeWait,
		Wait:    &WaitNode{DurationMs: intVal(m, "duration_ms"), ForEvent: strVal(m, "for_event"), TimeoutMs: intVal(m, "timeout_ms")},
		OnError: loadOnError(m),
	}, nil
}

// loadPanelNode reads a panel step: its name and exactly one of open,
// progress, ask, complete.
func loadPanelNode(m map[string]any) (*Node, error) {
	name := strVal(m, "name")
	if name == "" {
		return nil, fmt.Errorf("panel: missing 'name' (it names the panel in the workflow and keys an ask's answer)")
	}
	var action string
	var spec map[string]any
	for _, a := range []string{"open", "progress", "ask", "complete"} {
		if v, ok := m[a]; ok {
			if action != "" {
				return nil, fmt.Errorf("panel %q: one of open, progress, ask, complete per step, not %s and %s", name, action, a)
			}
			action = a
			spec, _ = v.(map[string]any)
			if spec == nil {
				spec = map[string]any{}
			}
		}
	}
	if action == "" {
		return nil, fmt.Errorf("panel %q: open, progress, ask or complete", name)
	}
	return &Node{
		ID:      nextNodeID("panel"),
		Kind:    NodePanel,
		Panel:   &PanelNode{Name: name, Action: action, Spec: spec},
		OnError: loadOnError(m),
	}, nil
}

func loadAssignNode(m map[string]any) (*Node, error) {
	target := strVal(m, "target")
	if target == "" {
		return nil, fmt.Errorf("assign: missing 'target'")
	}
	return &Node{
		ID:      nextNodeID("asgn"),
		Kind:    NodeAssign,
		Assign:  &AssignNode{Target: target, Value: m["value"]},
		OnError: loadOnError(m),
	}, nil
}

func loadBodyNode(m map[string]any) (*Node, error) {
	bodyRaw, ok := m["body"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing 'body'")
	}
	return loadStepNode(bodyRaw)
}

func loadBranchList(m map[string]any, key, owner string) ([]Node, error) {
	raw, ok := m[key].([]any)
	if !ok {
		return nil, fmt.Errorf("missing or invalid '%s' list", key)
	}
	nodes := make([]Node, 0, len(raw))
	for i, item := range raw {
		sm, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%d]: must be a map", key, i)
		}
		n, err := loadStepNode(sm)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", key, i, err)
		}
		// Every concurrency branch must be a delegate. Rejecting at LOAD time
		// beats the handler's runtime reject (authors fail at parse, not
		// mid-run); the handler check stays as defense-in-depth. Composite
		// branch logic is authored explicitly as a nested workflow child —
		// `task: {type: workflow, steps: [...]}` — which keeps the scope
		// contract (own results/state, inputs passed explicitly) visible.
		if n.Kind != NodeDelegate {
			return nil, fmt.Errorf(
				"%s[%d]: kind %q is not supported as a %s branch — branches must be delegate steps (task:/taskName:/uri:); wrap composite logic in task: {type: workflow, steps: [...]}",
				key, i, n.Kind, owner)
		}
		nodes = append(nodes, *n)
	}
	return nodes, nil
}

func intVal(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

func floatVal(m map[string]any, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 0
}

// checkStepMode refuses, when the workflow is loaded, a step mode no child
// can start with: an unknown one, and the forks — a fork carries its
// parent's conversation, and a workflow has none.
func checkStepMode(step, mode string) error {
	if mode == "" {
		return nil
	}
	if !presets.IsKnownMode(presets.Mode(mode)) {
		known := make([]string, 0)
		for _, m := range presets.KnownModes() {
			known = append(known, string(m))
		}
		sort.Strings(known)
		return fmt.Errorf("delegate %q: unknown mode %q (known: %s)", step, mode, strings.Join(known, ", "))
	}
	switch presets.Mode(mode) {
	case presets.ModeFork, presets.ModeForkIsolated:
		return fmt.Errorf("delegate %q: mode %q forks its parent's conversation, and a workflow has none — use isolated or background", step, mode)
	}
	return nil
}
