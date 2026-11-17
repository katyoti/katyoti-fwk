// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// permissionProfileNameKey is the context key the run command uses to
// pass the active profile name (from --permission-profile) into the
// kernel gate. An empty value means "use the configured default."
type permissionProfileNameKey struct{}

// WithPermissionProfile stores the active profile name in context.
// Called by the run command at session startup.
func WithPermissionProfile(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, permissionProfileNameKey{}, name)
}

// permissionProfileNameFromCtx retrieves the active profile name from
// context (set by WithPermissionProfile), or "" if none is set. The
// gate then asks PermissionProfiles.Active("") which falls back to
// the configured default.
func permissionProfileNameFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(permissionProfileNameKey{}).(string); ok {
		return v
	}
	return ""
}

// PermissionProfileFrom is the exported reader for the active profile name
// in context (empty when none is set). Callers that want an EXPLICIT
// selection (CLI flag / GUI) to win over a task-declared default check this
// before injecting their own via WithPermissionProfile.
func PermissionProfileFrom(ctx context.Context) string {
	return permissionProfileNameFromCtx(ctx)
}

// permissionGateOutcome carries the result of running a tool's
// CheckPermissions through the kernel gate.
//
//   - allow=true means dispatch should proceed; effectiveInput holds
//     the input that was either the original or the policy's
//     ModifiedInput substitution.
//   - allow=false means the kernel must emit an error tool_result
//     carrying errMsg and skip the dispatch — unless awaitRequest is set.
//   - awaitRequest names the inbox request the person answers the Ask
//     through: the loop waits on it as a machine and, approved, dispatches
//     with effectiveInput (an inbox run that waits as a machine).
type permissionGateOutcome struct {
	allow          bool
	effectiveInput json.RawMessage
	errMsg         string
	awaitRequest   string
	// awaitTask names, instead, the permission's human task:
	// the loop waits on its end as a machine; rules are the ones the tool
	// proposed, kept for an « always » answered after a restart.
	awaitTask string
	rules     []string
}

// applyPermissionGate runs the per-tool permission gate for a single
// tool call. It is called from agent_machine.go between H5
// (OnPreToolExec) and the dispatch / tool_call event emission.
//
// The function is split out from stepToolDispatch so it can be unit-
// tested in isolation against synthetic tools and a mock HumanGate
// (see permission_gate_test.go).
//
// Behavior:
//
//   - If the dispatcher has no Tool registered for `name` (e.g. the
//     plugin is an MCP/sidecar wrapper that doesn't expose typed
//     Tools), only the profile's DENY rules apply; otherwise the call
//     is allowed with the original input. A plugin that holds typed
//     Tools exposes them by being built with toolsapi.NewToolPlugin /
//     NewToolGroupPlugin, so the full gate runs.
//
//   - PermissionAllow → allow=true; ModifiedInput substitutes the
//     original input when non-empty.
//
//   - PermissionDeny → allow=false with the policy message (or a
//     fallback if the policy left it blank).
//
//   - PermissionAsk in a run whose user channel is not direct (inbox,
//     none, a parent): no console prompt. decideWithoutConsole decides
//     first (today: never); then an inbox run that waits as a machine,
//     with an InboxService, asks the person through the inbox and awaits
//     the answer (awaitRequest); any other run's call is denied with a
//     reason the agent can act on and a warning on the run.
//
//   - PermissionAsk on a direct channel → resolve HumanGate via UseService, prompt with
//     the policy message; on Confirmed=true allow (honoring
//     ModifiedInput), on Confirmed=false return a clear deny.
//     If HumanGate isn't registered the gate denies — this is
//     fail-closed: a tool that asked must be answered.
//
//   - PermissionPassthrough → reserved for ms04-p3 MCP work; treated
//     as Allow for now with a slog warning so the gap is visible.
func applyPermissionGate(ctx context.Context, dispatcher *Dispatcher, name string, input json.RawMessage) permissionGateOutcome {
	tool := dispatcher.LookupTool(name)

	// Resolve the active profile (best-effort; nil-tolerant for tests
	// and contexts that didn't register PermissionProfiles).
	var activeProfile *permrules.Profile
	var profiles permrules.Profiles
	if p, err := application.UseService[permrules.Profiles](ctx); err == nil && p != nil {
		profiles = p
		activeProfile = p.Active(permissionProfileNameFromCtx(ctx))
	}

	if tool == nil {
		// A plugin that does not expose typed Tools (MCP, sidecar, a
		// hand-built plugin): no CheckPermissions to run, but the
		// profile's DENY rules still hold — a deny names a tool, not
		// the way its plugin was built.
		if activeProfile != nil {
			if blocked, rule := activeProfile.DenyMatches(name, toolsapi.DefaultDimensionExtractor(input)); blocked {
				return permissionGateOutcome{
					allow:  false,
					errMsg: fmt.Sprintf("blocked by permission profile %q (rule: %s)", activeProfile.Name, rule.String()),
				}
			}
		}
		return permissionGateOutcome{allow: true, effectiveInput: input}
	}

	// Step 1: profile DENY rules. Hard block before the tool's own
	// CheckPermissions runs. The classifier needs dimensions, but we
	// don't have the tool's preferred Dimensions yet — we use the
	// default extractor over the raw input. Tools whose deny scope
	// depends on a derived field (e.g. file_read.absolute_path
	// post-resolution) cannot be blocked at this stage; they'll
	// surface via the tool's own CheckPermissions returning Deny.
	preDims := toolsapi.DefaultDimensionExtractor(input)
	if activeProfile != nil {
		if blocked, rule := activeProfile.DenyMatches(name, preDims); blocked {
			return permissionGateOutcome{
				allow:  false,
				errMsg: fmt.Sprintf("blocked by permission profile %q (rule: %s)", activeProfile.Name, rule.String()),
			}
		}
	}

	// Step 2: tool's own CheckPermissions.
	pres := tool.CheckPermissions(ctx, input)
	switch pres.Behavior {
	case toolsapi.PermissionAllow:
		eff := input
		if len(pres.ModifiedInput) > 0 {
			eff = pres.ModifiedInput
		}
		return permissionGateOutcome{allow: true, effectiveInput: eff}

	case toolsapi.PermissionDeny:
		msg := pres.Message
		if msg == "" {
			msg = "permission denied by tool policy"
		}
		return permissionGateOutcome{allow: false, errMsg: msg}

	case toolsapi.PermissionAsk:
		// Step 3: profile ALLOW rules. Soften the gate by silently
		// approving Asks that match a configured allow rule. Use the
		// tool's own Dimensions when populated; fall back to the
		// default extractor otherwise.
		dims := pres.Dimensions
		if dims == nil {
			dims = preDims
		}
		if activeProfile != nil {
			if matched, _ := activeProfile.AllowMatches(name, dims); matched {
				eff := input
				if len(pres.ModifiedInput) > 0 {
					eff = pres.ModifiedInput
				}
				return permissionGateOutcome{allow: true, effectiveInput: eff}
			}
		}

		// Step 4: the person is asked once, as a permission human task,
		// whatever the channel - after what decides without
		// anyone, off a console.
		if !directChannel(ctx) {
			if out, decided := decideWithoutConsole(ctx, name, input, pres); decided {
				return out
			}
		}
		return askAsHumanTask(ctx, profiles, name, input, pres, dims)

	case toolsapi.PermissionPassthrough:
		// Reserved for a tool that decides its own permission (MCP): not
		// built — denied, never taken for an allow.
		return permissionGateOutcome{allow: false, errMsg: fmt.Sprintf("tool %q asks to decide its own permission, which is not supported — denied", name)}
	}

	// Unknown behavior — fail closed.
	return permissionGateOutcome{
		allow:  false,
		errMsg: fmt.Sprintf("unknown permission behavior %v for tool %q", pres.Behavior, name),
	}
}

// directChannel says whether the run's user channel reaches a person at
// the process's console: direct, or none resolved (a programmatic call
// that bypassed the resolver, read as direct like ask-user does).
func directChannel(ctx context.Context) bool {
	switch agentapi.UserCommsInputFromContext(ctx) {
	case "", "direct":
		return true
	}
	return false
}

// decideWithoutConsole is where a tool's Ask in a run nobody answers on a
// console is decided without asking anyone: the spawn's allow list
// and a risk classifier would answer here, before
// the person is asked through the inbox. Nothing decides yet: it always
// returns undecided (false) — and it must never allow by default.
func decideWithoutConsole(_ context.Context, _ string, _ json.RawMessage, _ toolsapi.PermissionResult) (permissionGateOutcome, bool) {
	return permissionGateOutcome{}, false
}
