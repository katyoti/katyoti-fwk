// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// A TOOL'S PERMISSION IS A HUMAN TASK: a tool's Ask that no
// profile rule settles is asked by the runtime, never by the model, as one
// `permission` human task: a child run of the agent's run, in
// every channel. The person
// answers it once, from any screen that lists human tasks; its answer allows
// or denies the call, once or always, and an « always » keeps its rule in the
// active profile (the inbox's approval request lost it). A loop that waits as
// a machine waits on the human task's end - no slot held, a restart survived;
// any other loop blocks on its result.
//
// The answer reads, in order:
//   - a status other than success: denied, once;
//   - the decision its data names (more.decision, the permission kind's
//     answer data-type inboxapi.PermissionAnswer), else an answer naming it -
//     allow-once, allow-always, deny-once, deny-always (« - », « _ » or a
//     space alike);
//   - else the verdict - approved allows, rejected denies - once, or always
//     when the answer says « always »;
//   - nothing readable: denied (fail closed).
//
// An « always » keeps the rule its data names (more.rule), else the first rule
// the tool proposed; a rule that does not parse applies to this call only,
// with a warning on the run.

// permissionTaskKey is the key, in a saved batch's Resume, of the human task
// a tool's permission waits on; permissionRulesKey keeps the rules the tool
// proposed, for an « always » answered after a restart.
const (
	permissionTaskKey  = "permission_task"
	permissionRulesKey = "permission_rules"
)

// permissionTaskName is the task definition's name of a tool's permission.
const permissionTaskName = "tool-permission"

// askAsHumanTask starts the `permission` human task for a tool's Ask. In a
// loop that waits as a machine it returns the outcome awaiting its end
// (awaitTask); in any other it waits for the answer and decides. A human task
// that cannot be started denies the call: a tool that asked must be answered.
func askAsHumanTask(ctx context.Context, profiles permrules.Profiles, name string, input json.RawMessage, pres toolsapi.PermissionResult, dims map[string]string) permissionGateOutcome {
	eff := input
	if len(pres.ModifiedInput) > 0 {
		eff = pres.ModifiedInput
	}
	rules := suggestedRules(pres.Suggestions)
	ts, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil || ts == nil {
		machine.WarnRun(ctx, fmt.Sprintf("tool %q asked for a permission: no task service to ask it as a human task - denied", name))
		return permissionGateOutcome{allow: false, errMsg: fmt.Sprintf("tool %q needs a person's permission and it could not be asked here: the call is denied", name)}
	}
	spec, err := permissionTaskInput(ctx, name, pres, dims, rules)
	if err == nil {
		var resp taskapi.TaskRunResponse
		resp, err = ts.Run(ctx, taskapi.TaskRunRequest{
			TaskDef:     map[string]any{"name": permissionTaskName, "type": inboxapi.HumanTaskStrategy, "description": "A tool's permission an agent's run asks"},
			Input:       spec,
			ParentRunID: agentapi.RunIDFromContext(ctx),
		})
		if err == nil {
			slog.Info("permission: asked as a human task", "tool", name, "human_task", resp.RunID)
			if agentapi.MachineWaitsFrom(ctx) {
				return permissionGateOutcome{effectiveInput: eff, awaitTask: resp.RunID, rules: rules}
			}
			result, rerr := ts.Result(ctx, taskapi.ResultRequest{RunID: resp.RunID})
			if rerr != nil {
				return permissionGateOutcome{allow: false, errMsg: fmt.Sprintf("tool %q: the permission asked (%s) has no answer: %v - the call is denied", name, resp.RunID, rerr)}
			}
			decision, rule, msg := permissionAnswerOf(name, resp.RunID, result, rules)
			return outcomeOfDecision(ctx, profiles, name, pres.Message, eff, hgapi.PromptPermissionResponse{Decision: decision, SelectedRule: rule}, msg)
		}
	}
	machine.WarnRun(ctx, fmt.Sprintf("tool %q asked for a permission: the human task could not be started (%v) - denied", name, err))
	return permissionGateOutcome{allow: false, errMsg: fmt.Sprintf("tool %q needs a person's permission and it could not be asked: %v - the call is denied", name, err)}
}

// permissionTaskInput is the human task a tool's Ask is: of the permission
// kind, its question naming the tool, what the call would do and the rules
// an « always » keeps.
func permissionTaskInput(ctx context.Context, name string, pres toolsapi.PermissionResult, dims map[string]string, rules []string) (map[string]any, error) {
	ht := inboxapi.HumanTask{
		Kind:     inboxapi.KindPermission,
		Title:    fmt.Sprintf("Allow %s?", name),
		Question: permissionTaskQuestion(name, pres, dims, rules),
		// The rules an « always » may keep, for its card to offer
		// (the permission kind's answer).
		Options: rules,
		AskedBy: agentapi.RunIDFromContext(ctx),
	}
	raw, err := json.Marshal(ht)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	return m, json.Unmarshal(raw, &m)
}

// permissionTaskQuestion is the question's text: the tool's own, what the call
// would do, then how to answer - once or always, and the rule kept.
func permissionTaskQuestion(name string, pres toolsapi.PermissionResult, dims map[string]string, rules []string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSuffix(permissionDescription(name, pres, dims), "\n\nApprove to allow this call once; reject to deny it."))
	b.WriteString("\n\nApprove to allow this call, reject to deny it - once; answer `always` to keep the decision as a rule")
	if len(rules) > 0 {
		b.WriteString(" (by default `" + rules[0] + "`")
		if len(rules) > 1 {
			b.WriteString("; or name one of `" + strings.Join(rules[1:], "`, `") + "` as the answer's rule")
		}
		b.WriteString(")")
	} else {
		b.WriteString(", named as the answer's rule")
	}
	b.WriteString(".")
	return b.String()
}

// suggestedRules are the rules a tool proposes to keep, in its order.
func suggestedRules(s []toolsapi.PermissionSuggestion) []string {
	out := make([]string, 0, len(s))
	for _, x := range s {
		if r := strings.TrimSpace(x.Rule); r != "" {
			out = append(out, r)
		}
	}
	return out
}

// permissionTaskAwait is the wait of a call on its permission's human task.
func permissionTaskAwait(run string, rules []string) *agentapi.ErrAwait {
	resume := map[string]any{permissionTaskKey: run}
	if len(rules) > 0 {
		resume[permissionRulesKey] = rules
	}
	return &agentapi.ErrAwait{
		Topic:        machine.RunEndedTopic(run),
		Resume:       resume,
		Reason:       "a person's permission (human task " + run + ")",
		RecheckEvery: permissionRecheckEvery,
	}
}

// rulesOf reads the proposed rules a saved Resume kept.
func rulesOf(resume map[string]any) []string {
	switch v := resume[permissionRulesKey].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// resumePermissionTask reads the answer to a permission's human task once the
// loop is woken: still running → pending; ended → allowed or denied, an
// « always » keeping its rule in the active profile; gone → denied (no answer
// will come).
func resumePermissionTask(ctx context.Context, name, run string, rules []string) (permissionAnswer, string) {
	ts, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil || ts == nil {
		slog.Warn("permission: no task service to read the answer from, waiting on", "tool", name, "human_task", run, "error", err)
		return permissionPending, ""
	}
	st, err := ts.Status(ctx, taskapi.StatusRequest{RunID: run})
	if err != nil {
		return permissionDenied, fmt.Sprintf("the permission asked for %q (human task %s) is gone: no answer will come and the call is denied - ask again if you still need it", name, run)
	}
	switch st.Status {
	case taskapi.RunStatusCompleted, taskapi.RunStatusFailed, taskapi.RunStatusCancelled, taskapi.RunStatusKilled:
	default:
		return permissionPending, ""
	}
	result, err := ts.Result(ctx, taskapi.ResultRequest{RunID: run})
	if err != nil {
		slog.Warn("permission: reading the answer failed, waiting on", "tool", name, "human_task", run, "error", err)
		return permissionPending, ""
	}
	decision, rule, msg := permissionAnswerOf(name, run, result, rules)
	var profiles permrules.Profiles
	if p, err := application.UseService[permrules.Profiles](ctx); err == nil {
		profiles = p
	}
	out := outcomeOfDecision(ctx, profiles, name, "", nil, hgapi.PromptPermissionResponse{Decision: decision, SelectedRule: rule}, msg)
	if out.allow {
		return permissionAllowed, ""
	}
	return permissionDenied, out.errMsg
}

// permissionAnswerOf reads a permission's human task's result as the four-way
// decision, the rule an « always » keeps, and the denial's words.
func permissionAnswerOf(name, run string, result taskapi.ResultResponse, rules []string) (hgapi.PermissionDecision, string, string) {
	if result.Error != "" {
		return hgapi.PermissionDecisionDenyOnce, "", fmt.Sprintf("the permission asked for %q (human task %s) ended without an answer: %s - the call is denied", name, run, result.Error)
	}
	var a inboxapi.HumanAnswer
	raw, err := json.Marshal(result.Output)
	if err == nil {
		err = json.Unmarshal(raw, &a)
	}
	if err != nil {
		return hgapi.PermissionDecisionDenyOnce, "", fmt.Sprintf("the answer to the permission asked for %q (human task %s) does not read - the call is denied", name, run)
	}
	note := ""
	if s := strings.TrimSpace(a.Summary); s != "" {
		note = ": " + s
	}
	if a.Status != "" && a.Status != "success" {
		return hgapi.PermissionDecisionDenyOnce, "", fmt.Sprintf("the person did not allow %s (%s)%s", name, a.Status, note)
	}
	rule, _ := a.More["rule"].(string)
	rule = strings.TrimSpace(rule)
	if rule == "" && len(rules) > 0 {
		rule = rules[0]
	}
	given := a.Answer
	if d, _ := a.More["decision"].(string); strings.TrimSpace(d) != "" {
		// An answer shaped by the kind's data-type (inboxapi.PermissionAnswer).
		given = d
	}
	answer := strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(strings.TrimSpace(given)))
	decision, known := map[string]hgapi.PermissionDecision{
		"allow_once": hgapi.PermissionDecisionAllowOnce, "allow_always": hgapi.PermissionDecisionAllowAlways,
		"deny_once": hgapi.PermissionDecisionDenyOnce, "deny_always": hgapi.PermissionDecisionDenyAlways,
	}[answer]
	if !known {
		always := answer == "always"
		switch a.Verdict {
		case "approved":
			decision, known = hgapi.PermissionDecisionAllowOnce, true
			if always {
				decision = hgapi.PermissionDecisionAllowAlways
			}
		case "rejected":
			decision, known = hgapi.PermissionDecisionDenyOnce, true
			if always {
				decision = hgapi.PermissionDecisionDenyAlways
			}
		}
	}
	if !known {
		return hgapi.PermissionDecisionDenyOnce, "", fmt.Sprintf("the person's answer to %s (%q) neither allows nor denies it - the call is denied%s", name, a.Answer, note)
	}
	switch decision {
	case hgapi.PermissionDecisionDenyOnce, hgapi.PermissionDecisionDenyAlways:
		return decision, rule, fmt.Sprintf("the person denied %s%s", name, note)
	}
	return decision, rule, ""
}

// outcomeOfDecision is a person's four-way decision on a tool's Ask, from the
// console prompt or a human task: allowed with the call's input, or denied
// with the person's words (deniedMsg, else the tool's question); an « always »
// keeps its rule at the front of the active profile.
func outcomeOfDecision(ctx context.Context, profiles permrules.Profiles, name, question string, input json.RawMessage, resp hgapi.PromptPermissionResponse, deniedMsg string) permissionGateOutcome {
	keep := func(add func(permrules.Rule), verb string) {
		if resp.SelectedRule == "" {
			machine.WarnRun(ctx, fmt.Sprintf("tool %q: always %s, with no rule to keep - it applies to this call only", name, verb))
			return
		}
		if profiles == nil {
			return
		}
		rule, err := permrules.ParseRule(resp.SelectedRule)
		if err != nil {
			machine.WarnRun(ctx, fmt.Sprintf("the rule %q to always %s could not be saved — it applies to this call only: %v", resp.SelectedRule, verb, err))
			return
		}
		add(rule)
	}
	switch resp.Decision {
	case hgapi.PermissionDecisionAllowOnce:
		return permissionGateOutcome{allow: true, effectiveInput: input}
	case hgapi.PermissionDecisionAllowAlways:
		keep(func(r permrules.Rule) { profiles.AddSessionAllow(r) }, "allow")
		return permissionGateOutcome{allow: true, effectiveInput: input}
	case hgapi.PermissionDecisionDenyOnce:
		msg := deniedMsg
		if msg == "" {
			msg = fmt.Sprintf("user denied permission for %q", name)
			if question != "" {
				msg = "user denied: " + question
			}
		}
		return permissionGateOutcome{allow: false, errMsg: msg}
	case hgapi.PermissionDecisionDenyAlways:
		keep(func(r permrules.Rule) { profiles.AddSessionDeny(r) }, "deny")
		msg := deniedMsg
		if msg == "" {
			msg = fmt.Sprintf("user denied permission for %q", name)
		}
		return permissionGateOutcome{allow: false, errMsg: msg + " (and added deny rule)"}
	}
	return permissionGateOutcome{allow: false, errMsg: fmt.Sprintf("unknown permission decision %v from prompt", resp.Decision)}
}
