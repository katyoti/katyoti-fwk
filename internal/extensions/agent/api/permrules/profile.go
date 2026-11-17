// SPDX-License-Identifier: Apache-2.0

package permrules

// Profile is a named bundle of permission rules. The kernel
// permission gate (applyPermissionGate) consults the active profile
// before falling through to the tool's own CheckPermissions and
// before prompting the user.
//
// A profile holds two independent rule lists:
//
//   - DenyRules — checked FIRST. A match means the tool call is
//     blocked outright; the tool's own CheckPermissions is not
//     consulted. Used by hardening profiles like "plan-mode" to
//     block writes regardless of what the tool would have allowed.
//
//   - AllowRules — checked AFTER the tool's CheckPermissions, but
//     ONLY when the tool returned PermissionAsk. A match silences
//     the user prompt and treats the call as allowed. Used by
//     softening profiles like "workspace-edit" to pre-approve
//     workspace-internal asks.
//
// Profiles can both soften (pre-approve asks via AllowRules) AND
// harden (block tools via DenyRules). The two lists are independent,
// not a single ordered list. Within each list, rules are evaluated
// in order — the first matching rule wins. Session-added rules are
// inserted at the FRONT (via AddSessionAllow / AddSessionDeny) so
// they shadow base rules from the loaded profile YAML.
//
// Important security invariant: profiles cannot un-Allow a tool's
// PermissionAllow result, and they cannot un-Deny a tool's
// PermissionDeny result. The tool's intrinsic decision is the floor
// for security; profiles can only restrict (via DenyRules at the
// front of the gate) or relax (via AllowRules at the back of the
// gate, intercepting Asks before they reach the user).
type Profile struct {
	// Name is the profile identifier (e.g. "workspace-edit"). Used
	// in error messages and the --permission-profile CLI flag.
	Name string

	// Description is a human-readable summary, surfaced in
	// `ktw-agents run --help` and similar discovery surfaces.
	Description string

	// AllowRules pre-approve tool calls that returned PermissionAsk.
	// Order matters: first match wins. Session additions are inserted
	// at index 0 so they shadow base rules.
	AllowRules []Rule

	// DenyRules block tool calls before the tool's CheckPermissions
	// runs. Order matters: first match wins.
	DenyRules []Rule
}

// AllowMatches reports whether any AllowRule matches the given tool
// call, and returns the matching rule for diagnostics. Returns
// (false, zero Rule) on no match.
func (p *Profile) AllowMatches(toolName string, dimensions map[string]string) (bool, Rule) {
	if p == nil {
		return false, Rule{}
	}
	for _, r := range p.AllowRules {
		if r.Match(toolName, dimensions) {
			return true, r
		}
	}
	return false, Rule{}
}

// DenyMatches reports whether any DenyRule matches the given tool
// call, and returns the matching rule for diagnostics.
func (p *Profile) DenyMatches(toolName string, dimensions map[string]string) (bool, Rule) {
	if p == nil {
		return false, Rule{}
	}
	for _, r := range p.DenyRules {
		if r.Match(toolName, dimensions) {
			return true, r
		}
	}
	return false, Rule{}
}

// AddSessionAllow inserts a rule at the FRONT of AllowRules so it
// shadows any base rule that would otherwise match. Used by the
// permission gate when the user picks "Always allow" in a
// PromptPermission response.
//
// The rule is added in-place; callers serialize concurrent access
// at the PermissionProfiles service level.
func (p *Profile) AddSessionAllow(rule Rule) {
	p.AllowRules = append([]Rule{rule}, p.AllowRules...)
}

// AddSessionDeny inserts a rule at the FRONT of DenyRules so it
// takes precedence over any base rule. Used by the permission gate
// when the user picks "Always deny" in a PromptPermission response.
func (p *Profile) AddSessionDeny(rule Rule) {
	p.DenyRules = append([]Rule{rule}, p.DenyRules...)
}

// Clone returns a deep copy of the profile. Used when forking the
// loaded base profile into a session-mutable copy so that runtime
// AddSession* calls don't pollute the base profile shared by other
// sessions.
func (p *Profile) Clone() *Profile {
	if p == nil {
		return nil
	}
	clone := &Profile{
		Name:        p.Name,
		Description: p.Description,
		AllowRules:  make([]Rule, len(p.AllowRules)),
		DenyRules:   make([]Rule, len(p.DenyRules)),
	}
	copy(clone.AllowRules, p.AllowRules)
	copy(clone.DenyRules, p.DenyRules)
	return clone
}

// Profiles is the cross-package interface for accessing the active
// permission profile and recording session-add decisions. Defined here
// (in the leaf permrules package) so that both the runtime package
// (kernel gate) and the agent/local package (concrete impl) can
// implement and consume it without creating an import cycle.
//
// The kernel permission gate resolves this interface from the DI
// container via application.UseService[permrules.Profiles](ctx). The
// concrete implementation lives in agent/local/permissions.go.
type Profiles interface {
	// Active returns the session-mutable profile for the given name,
	// or for the configured default if name is empty. Returns nil if
	// no profile by that name exists; the caller should treat nil as
	// "no profile filtering" (existing tool gate behavior preserved).
	Active(profileName string) *Profile

	// AddSessionAllow inserts an allow rule at the front of the
	// active session profile so it shadows base rules.
	AddSessionAllow(rule Rule)

	// AddSessionDeny inserts a deny rule at the front of the active
	// session profile.
	AddSessionDeny(rule Rule)

	// Default returns the configured default profile name.
	Default() string
}
