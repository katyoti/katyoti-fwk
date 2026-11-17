// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// A TOOL'S PERMISSION IS A HUMAN TASK: with IN-039's switch on
// a tool's Ask is asked by the runtime as one `permission` human task - a
// child run of the agent's run - whatever the channel; its answer allows or
// denies, once or always, an « always » keeping its rule.

// permTasks is a task service that runs human tasks: Run records what was
// asked, Status and Result answer what the test set; statusErr fails Status
// (the run is gone).
type permTasks struct {
	taskapi.TaskService
	mu        sync.Mutex
	started   []taskapi.TaskRunRequest
	status    taskapi.RunStatus
	statusErr error
	result    taskapi.ResultResponse
}

func (p *permTasks) Run(_ context.Context, in taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.started = append(p.started, in)
	return taskapi.TaskRunResponse{RunID: "ht-perm-1"}, nil
}

func (p *permTasks) Status(_ context.Context, in taskapi.StatusRequest) (taskapi.StatusResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.statusErr != nil {
		return taskapi.StatusResponse{}, p.statusErr
	}
	return taskapi.StatusResponse{RunID: in.RunID, Status: p.status}, nil
}

func (p *permTasks) Result(_ context.Context, in taskapi.ResultRequest) (taskapi.ResultResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r := p.result
	r.RunID = in.RunID
	return r, nil
}

// answer sets the human task's end: the person's answer.
func (p *permTasks) answer(status, verdict, answer string, more map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = taskapi.RunStatusCompleted
	p.result = taskapi.ResultResponse{Status: "completed", Output: inboxapi.HumanAnswer{
		Task: "m.TD-1", Status: status, Verdict: verdict, Answer: answer, Summary: "said so", More: more,
		By: cwmapi.Actor{ID: "jdoe", Kind: "person"},
	}}
}

// permTaskCtx is a run's dispatch context with IN-039's switch on: the
// application holds the task service, the console's HumanGate, the inbox and
// the permission profiles (each nil-able), on the given channel.
func permTaskCtx(t *testing.T, channel string, tasks *permTasks, gate hgapi.HumanGate, inbox inboxapi.InboxService, profiles permrules.Profiles, machineWaits bool) context.Context {
	t.Helper()
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)))
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	if tasks != nil {
		application.PutService[taskapi.TaskService](app, "default", tasks)
	}
	if gate != nil {
		application.PutService[hgapi.HumanGate](app, "default", gate)
	}
	if inbox != nil {
		application.PutService[inboxapi.InboxService](app, "default", inbox)
	}
	if profiles != nil {
		application.PutService[permrules.Profiles](app, "default", profiles)
	}
	ctx := agentapi.WithRunID(application.WithApplication(context.Background(), app), "agent-run-7")
	ctx = agentapi.WithUserCommsInput(ctx, channel)
	if machineWaits {
		ctx = agentapi.WithMachineWaits(ctx)
	}
	return ctx
}

var askingWrite = toolsapi.PermissionResult{
	Behavior:    toolsapi.PermissionAsk,
	Message:     "write the file?",
	Suggestions: []toolsapi.PermissionSuggestion{{Label: "this file", Rule: "t_write.path:notes.md"}, {Label: "any file", Rule: "t_write:**"}},
}

// In every channel - the chat's console and the inbox alike - a tool's Ask is
// one permission human task started by the runtime, a child of the agent's
// run, naming the tool, what the call would do and the rules an « always »
// keeps; the console is never prompted, no inbox request is filed, and a loop
// that waits as a machine awaits the human task with the call's input kept.
func TestAToolsPermissionIsAskedAsOnePermissionHumanTask(t *testing.T) {
	for _, channel := range []string{"direct", "inbox", "none"} {
		t.Run(channel, func(t *testing.T) {
			d, tool := dispatcherWithTool("t_write", askingWrite)
			tasks := &permTasks{status: taskapi.RunStatusRunning}
			gate := &fakeHumanGate{decision: hgapi.PermissionDecisionAllowOnce}
			inbox := &fakeInbox{}
			ctx := permTaskCtx(t, channel, tasks, gate, inbox, nil, true)

			out := applyPermissionGate(ctx, d, "t_write", json.RawMessage(`{"path":"notes.md"}`))
			if len(gate.permissionCalls) != 0 || len(inbox.filed) != 0 {
				t.Fatalf("the console was prompted %d time(s), the inbox filed %d: want neither", len(gate.permissionCalls), len(inbox.filed))
			}
			if len(tasks.started) != 1 {
				t.Fatalf("started %d human task(s), want exactly one", len(tasks.started))
			}
			if out.allow || out.awaitTask != "ht-perm-1" || out.awaitRequest != "" {
				t.Fatalf("outcome = %+v, want awaiting the human task, not allowed", out)
			}
			if string(out.effectiveInput) != `{"path":"notes.md"}` || strings.Join(out.rules, " ") != "t_write.path:notes.md t_write:**" {
				t.Errorf("kept input %s, rules %v: want the call's input and the tool's rules", out.effectiveInput, out.rules)
			}
			if tool.callCount != 0 {
				t.Error("the tool ran before the person answered")
			}
			run := tasks.started[0]
			if run.TaskDef["type"] != inboxapi.HumanTaskStrategy || run.ParentRunID != "agent-run-7" {
				t.Errorf("run %+v: want a human task, child of the agent's run", run)
			}
			if run.Input["kind"] != inboxapi.KindPermission || run.Input["title"] != "Allow t_write?" || run.Input["asked_by"] != "agent-run-7" {
				t.Errorf("asked %v: want a permission question about t_write, asked by the agent's run", run.Input)
			}
			if opts, _ := run.Input["options"].([]any); len(opts) != 2 || opts[0] != "t_write.path:notes.md" || opts[1] != "t_write:**" {
				t.Errorf("options %v: want the tool's rules, for the card to offer", run.Input["options"])
			}
			q, _ := run.Input["question"].(string)
			for _, want := range []string{"write the file?", "notes.md", "always", "t_write.path:notes.md", "t_write:**"} {
				if !strings.Contains(q, want) {
					t.Errorf("question %q: want %q", q, want)
				}
			}
		})
	}
}

// The answer decides: approved allows, rejected denies with the person's
// words, once - or always, keeping the rule its data names, else the tool's
// first; an answer that neither allows nor denies, a status other than
// success, or a human task gone denies; still running waits.
func TestThePermissionsAnswerAllowsOrDeniesOnceOrAlways(t *testing.T) {
	for _, c := range []struct {
		name                    string
		status, verdict, answer string
		more                    map[string]any
		running                 bool
		gone                    bool
		want                    permissionAnswer
		msg                     string
		allowRule, denyRule     string
	}{
		{name: "approved", status: "success", verdict: "approved", want: permissionAllowed},
		{name: "rejected", status: "success", verdict: "rejected", want: permissionDenied, msg: "the person denied t_write: said so"},
		{name: "approved always", status: "success", verdict: "approved", answer: "always", want: permissionAllowed, allowRule: "t_write.path:notes.md"},
		{name: "allow-always with its rule", status: "success", answer: "allow-always", more: map[string]any{"rule": "t_write:**"}, want: permissionAllowed, allowRule: "t_write:**"},
		{name: "deny always", status: "success", answer: "deny_always", want: permissionDenied, msg: "denied t_write", denyRule: "t_write.path:notes.md"},
		{name: "allow once named", status: "success", answer: "Allow once", want: permissionAllowed},
		{name: "the kind's data-type", status: "success", more: map[string]any{"decision": "deny-always", "rule": "t_write:**"}, want: permissionDenied, msg: "denied t_write", denyRule: "t_write:**"},
		{name: "its data's decision first", status: "success", answer: "allow-once", more: map[string]any{"decision": "allow-always"}, want: permissionAllowed, allowRule: "t_write.path:notes.md"},
		{name: "unreadable answer", status: "success", answer: "maybe", want: permissionDenied, msg: "neither allows nor denies"},
		{name: "blocked", status: "blocked", verdict: "approved", want: permissionDenied, msg: "did not allow t_write (blocked)"},
		{name: "running", running: true, want: permissionPending},
		{name: "gone", gone: true, want: permissionDenied, msg: "no answer will come"},
	} {
		t.Run(c.name, func(t *testing.T) {
			tasks := &permTasks{status: taskapi.RunStatusRunning}
			if c.gone {
				tasks.statusErr = errors.New("unknown run")
			} else if !c.running {
				tasks.answer(c.status, c.verdict, c.answer, c.more)
			}
			profiles := &fakeProfiles{profile: &permrules.Profile{Name: "test"}}
			ctx := permTaskCtx(t, "direct", tasks, nil, nil, profiles, true)

			got, msg := resumePermissionTask(ctx, "t_write", "ht-perm-1", []string{"t_write.path:notes.md", "t_write:**"})
			if got != c.want || !strings.Contains(msg, c.msg) {
				t.Fatalf("resumePermissionTask = %v %q, want %v containing %q", got, msg, c.want, c.msg)
			}
			rules := func(rs []permrules.Rule) string {
				var s []string
				for _, r := range rs {
					s = append(s, r.String())
				}
				return strings.Join(s, " ")
			}
			if a := rules(profiles.addedAllow); a != c.allowRule {
				t.Errorf("allow rules kept %q, want %q", a, c.allowRule)
			}
			if d := rules(profiles.addedDeny); d != c.denyRule {
				t.Errorf("deny rules kept %q, want %q", d, c.denyRule)
			}
		})
	}
}

// An « always » keeps its rule: the next matching call is allowed by the
// profile without asking anyone again.
func TestAnAlwaysKeepsItsRuleAndTheNextCallIsNotAsked(t *testing.T) {
	d, _ := dispatcherWithTool("t_write", toolsapi.PermissionResult{
		Behavior: toolsapi.PermissionAsk, Message: "write the file?",
		Suggestions: []toolsapi.PermissionSuggestion{{Label: "this file", Rule: "t_write.path:notes.md"}},
		Dimensions:  map[string]string{"path": "notes.md"},
	})
	tasks := &permTasks{}
	tasks.answer("success", "approved", "always", nil)
	profiles := &fakeProfiles{profile: &permrules.Profile{Name: "test"}}
	// A loop that blocks: the gate waits for the answer and decides.
	ctx := permTaskCtx(t, "direct", tasks, nil, nil, profiles, false)

	if out := applyPermissionGate(ctx, d, "t_write", json.RawMessage(`{"path":"notes.md"}`)); !out.allow || out.awaitTask != "" {
		t.Fatalf("first call: %+v, want allowed once answered", out)
	}
	if out := applyPermissionGate(ctx, d, "t_write", json.RawMessage(`{"path":"notes.md"}`)); !out.allow {
		t.Fatalf("second call: %+v, want allowed by the kept rule", out)
	}
	if len(tasks.started) != 1 {
		t.Errorf("asked %d time(s), want once: the kept rule answers the next call", len(tasks.started))
	}
}

// Without a task service the call is denied - a tool that asked must be
// answered, never let through.
func TestWithoutAHumanTaskTheCallIsDenied(t *testing.T) {
	d, _ := dispatcherWithTool("t_write", askingWrite)
	ctx := permTaskCtx(t, "direct", nil, nil, nil, nil, true)
	if out := applyPermissionGate(ctx, d, "t_write", json.RawMessage(`{}`)); out.allow || out.awaitTask != "" || out.errMsg == "" {
		t.Fatalf("no task service: %+v, want denied", out)
	}
}

// The loop waits on the permission's human task as a machine - the call not
// run, the batch saved across a restart - and goes on with its answer:
// running waits again, nothing asked anew; approved runs the tool once with
// its input, never showing it the permission's resume; rejected answers the
// call denied.
func TestTheLoopAwaitsThePermissionsHumanTaskAcrossARestart(t *testing.T) {
	for _, c := range []struct {
		verdict string
		runs    int
		want    string
	}{
		{verdict: "approved", runs: 1, want: "notes.md"},
		{verdict: "rejected", runs: 0, want: "the person denied t_write"},
	} {
		t.Run(c.verdict, func(t *testing.T) {
			tool := &resumeSeeingTool{permissionTestTool: newPermTool("t_write", askingWrite)}
			set := NewPluginSet()
			set.Add(toolsapi.NewToolPlugin(tool))
			m := &AgentLoopMachine{plugins: set, hooks: &HookChain{}, loop: NewLoopContext()}
			call := llm.ToolCall{ID: "1", Type: "function", FunctionCall: &llm.FunctionCall{Name: "t_write", Arguments: `{"path":"notes.md"}`}}
			as := &agentState{StartedAt: time.Now(), LastResponse: &llm.ContentResponse{Choices: []*llm.ContentChoice{{ToolCalls: []llm.ToolCall{call}}}}}
			tasks := &permTasks{status: taskapi.RunStatusRunning}
			ctx := permTaskCtx(t, "direct", tasks, nil, nil, nil, true)
			step := func() *machine.StepResult {
				t.Helper()
				action, err := m.stepToolDispatch(ctx, as, false)
				if err != nil {
					t.Fatal(err)
				}
				res, err := action(ctx)
				if err != nil {
					t.Fatal(err)
				}
				return res
			}

			res := step()
			if res.NextState != "awaiting" || res.Wait == nil || len(res.Wait.Conditions) != 1 || len(tasks.started) != 1 {
				t.Fatalf("after the ask: %+v (asked %d), want awaiting the human task", res, len(tasks.started))
			}
			if cond := res.Wait.Conditions[0]; cond.Topic != machine.RunEndedTopic("ht-perm-1") {
				t.Errorf("condition = %+v, want the human task's end", cond)
			}

			// Saved and read back, as across a restart.
			b, err := json.Marshal(as)
			if err != nil {
				t.Fatal(err)
			}
			var restored any
			if err := json.Unmarshal(b, &restored); err != nil {
				t.Fatal(err)
			}
			if as, err = m.ensureTyped(&machine.State{Data: restored}); err != nil {
				t.Fatal(err)
			}

			if res = step(); res.NextState != "awaiting" || len(tasks.started) != 1 {
				t.Fatalf("still running: %s (asked %d), want awaiting again, nothing asked anew", res.NextState, len(tasks.started))
			}
			tasks.answer("success", c.verdict, "", nil)
			if res = step(); res.NextState != "thinking" {
				t.Fatalf("answered: %s, want the next turn", res.NextState)
			}
			if tool.callCount != c.runs || tool.sawResume {
				t.Errorf("tool ran %d time(s) (saw resume: %v), want %d without a resume", tool.callCount, tool.sawResume, c.runs)
			}
			if conv, _ := json.Marshal(as.Messages); !strings.Contains(string(conv), c.want) {
				t.Errorf("conversation %s: want %q", conv, c.want)
			}
		})
	}
}
