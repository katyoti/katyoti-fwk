// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// RefreshHook is a consumer-side reaction to a ContributionsChangedEvent.
//
// The contribution-change subscriber refreshes the in-process catalog index
// first, then fans out to every contributed RefreshHook whose Kinds intersect
// the event's KindsChanged. This makes the catalog refresh extensible: a
// consumer (SkillManager, the CWM type store, …) contributes a hook instead of
// being hard-wired into the subscriber. SkillManager.Reload is one such hook —
// "one subscriber among others".
//
// Hooks are contributed to ContributionRefreshHooksPoint (an EM list point) and
// resolved via application.UseExtensions[RefreshHook](ctx).
type RefreshHook struct {
	// Name identifies the hook in logs (e.g. "skill-manager", "cwm-types").
	Name string

	// Kinds gates when the hook fires. A hook fires when its Kinds intersect
	// the event's KindsChanged, OR when either side is the wildcard "*". An
	// empty Kinds slice is treated as "*" (fire on every change).
	Kinds []string

	// Refresh performs the consumer-side re-resolution. Invoked best-effort:
	// the subscriber logs an error and continues to the next hook.
	Refresh func(ctx context.Context, ev contribapi.ContributionsChangedEvent) error
}

// Matches reports whether this hook should fire for the given changed kinds.
// An empty hook.Kinds, a "*" in hook.Kinds, an empty changed slice, or a "*"
// in changed all mean "fire". Otherwise it's a set intersection.
func (h RefreshHook) Matches(changed []string) bool {
	if len(h.Kinds) == 0 || len(changed) == 0 {
		return true
	}
	want := make(map[string]struct{}, len(h.Kinds))
	for _, k := range h.Kinds {
		if k == "*" {
			return true
		}
		want[k] = struct{}{}
	}
	for _, c := range changed {
		if c == "*" {
			return true
		}
		if _, ok := want[c]; ok {
			return true
		}
	}
	return false
}
