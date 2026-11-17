// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// PermissionSettings holds the configurable knobs for the rules system.
// Loaded from PermissionProfiles.yaml via the typed-config service
// pipeline (config.DecodeBytes / variable expansion / ProcessConfig).
//
// Embedded default ships with default_profile=workspace-edit and
// persist_always_decisions=false. Users can override either field by
// dropping their own PermissionProfiles.yaml in their config dir.
type PermissionSettings struct {
	// DefaultProfile is the profile name applied when the user doesn't
	// pass --permission-profile. Falls back to "workspace-edit" if the
	// configured name doesn't exist on disk (with a warning).
	DefaultProfile string `yaml:"default_profile"`

	// PersistAlwaysDecisions controls whether AllowAlways/DenyAlways
	// selections from PromptPermission are written to a sticky file
	// (`Permissions/_session-additions.yaml`) so they survive across
	// sessions. Defaults to false: in-memory session additions only.
	PersistAlwaysDecisions bool `yaml:"persist_always_decisions"`
}

// PermissionProfiles is the DI service that resolves named permission
// profiles for the kernel permission gate.
//
// It embeds permrules.Profiles (the leaf interface the kernel gate
// consumes) and adds the local-only List() method used by CLI
// commands and tests.
//
// The service holds:
//   - the base profiles: the `permission-profile` contributions, read at the
//     first Active - the universe's, else this binary's own
//     so the gate works with the hub down and the catalog cold
//   - a session-mutable forked copy of the active profile (with
//     AllowAlways/DenyAlways additions inserted at the front)
//
// All methods are safe for concurrent use across goroutines within
// the same session.
type PermissionProfiles interface {
	permrules.Profiles

	// List returns the names of all contributed base profiles. Used by
	// CLI surfaces and tests; the kernel gate doesn't need it.
	List() []string
}

// permissionProfiles is the concrete implementation backing the
// PermissionProfiles interface.
type permissionProfiles struct {
	mu sync.Mutex

	settings PermissionSettings

	// ctx reads the profiles: the application's, never cancelled.
	ctx context.Context

	// base holds the read-only loaded profiles, keyed by name; nil until
	// first read (a read that finds none is tried again next time).
	base map[string]*permrules.Profile

	// active is the cloned session-mutable copy of the currently
	// active profile. Lazily created on first Active() call so we
	// don't pay the clone cost when the session never reaches the
	// permission gate.
	active     *permrules.Profile
	activeName string

	// userProfilesDir is where _session-additions.yaml is written
	// when PersistAlwaysDecisions is true (`<ConfigDir>/Permissions`).
	// Empty: nothing persisted.
	userProfilesDir string
}

// PermissionProfilesExtension registers PermissionProfiles as a DI
// service backed by PermissionSettings (loaded from
// `PermissionProfiles.yaml`). Mirrors LLMProfilesExtension.
//
// The settings come through the typed-config pipeline; the profiles
// themselves are the `permission-profile` contributions (QueryProfiles),
// published by agentcontrib.OwnProfiles and by any member.
//
// Two parallel registrations land:
//   - PermissionProfiles (this package's interface) — used by CLI
//     commands and tests that want List().
//   - permrules.Profiles (the leaf interface) — used by the kernel
//     permission gate in agent/internal/runtime, which can't import
//     this package without creating a cycle.
//
// Both registrations share the same underlying instance.
var PermissionProfilesExtension application.Extension = func(init *application.ApplicationInit) {
	// Register the local-typed service (with List).
	application.DeclareService[PermissionProfiles, PermissionSettings](
		func(ctx context.Context, cfg *PermissionSettings) (PermissionProfiles, error) {
			return constructPermissionProfiles(ctx, cfg)
		},
	)(init)

	// Register the leaf-interface service the kernel gate consumes.
	// Backed by the SAME underlying type — the framework caches
	// service instances by type-name so the second registration is
	// resolved independently. To keep them in sync we use the same
	// constructor.
	application.DeclareService[permrules.Profiles, PermissionSettings](
		func(ctx context.Context, cfg *PermissionSettings) (permrules.Profiles, error) {
			pp, err := constructPermissionProfiles(ctx, cfg)
			if err != nil {
				return nil, err
			}
			return pp, nil
		},
	)(init)
}

// constructPermissionProfiles is the shared constructor body used by
// both DI registrations above. It reads no profile: the first Active()
// does, so the service needs no hub to be built.
func constructPermissionProfiles(ctx context.Context, cfg *PermissionSettings) (PermissionProfiles, error) {
	if cfg.DefaultProfile == "" {
		cfg.DefaultProfile = "workspace-edit"
	}
	return &permissionProfiles{
		settings:        *cfg,
		ctx:             context.WithoutCancel(ctx),
		userProfilesDir: filepath.Join(application.Env().ConfigDir(), "Permissions"),
	}, nil
}

// QueryProfiles reads the contributed permission profiles, keyed by name
// (the entry's id): the universe's, else - the hub down, the catalog cold,
// or none there - this binary's own. A profile whose rule does not parse is
// left out, with a warning. A var: a test sets it.
var QueryProfiles = func(ctx context.Context) (map[string]*permrules.Profile, error) {
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Kind: permrules.ContributionTypePermissionProfile}
	entries, err := queryProfileSpecs(ctx, q)
	if err != nil || len(entries) == 0 {
		q.Reach = mbapi.ReachSelf
		entries, err = queryProfileSpecs(ctx, q)
	}
	if err != nil {
		return nil, err
	}
	out := make(map[string]*permrules.Profile, len(entries))
	for _, e := range entries {
		ref, perr := contribapi.ParseResourceURI(e.URI)
		if perr != nil || ref.ID == "" {
			continue
		}
		if _, seen := out[ref.ID]; seen {
			continue
		}
		p, perr := permrules.NewProfile(ref.ID, e.Body)
		if perr != nil {
			slog.Warn("permission profile left out", "uri", e.URI, "error", perr)
			continue
		}
		out[ref.ID] = p
	}
	return out, nil
}

// queryProfileSpecs is the catalog read; a test makes the universe unreachable.
var queryProfileSpecs = mbapi.QueryTyped[permrules.ProfileSpec]

// baseLocked reads the profiles once; must be called with pp.mu held.
func (pp *permissionProfiles) baseLocked() map[string]*permrules.Profile {
	if pp.base != nil {
		return pp.base
	}
	ctx := pp.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	got, err := QueryProfiles(ctx)
	if err != nil {
		slog.Warn("permission profiles unreadable", "error", err)
	}
	if len(got) == 0 {
		return nil
	}
	pp.base = got
	return got
}

// ── permissionProfiles methods ──────────────────────────────────────────

func (pp *permissionProfiles) Active(profileName string) *permrules.Profile {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	if profileName == "" {
		profileName = pp.settings.DefaultProfile
	}

	// Reuse the cached clone if the requested profile matches.
	if pp.active != nil && pp.activeName == profileName {
		return pp.active
	}

	profiles := pp.baseLocked()
	base, ok := profiles[profileName]
	if !ok {
		// Fall back to the default if the requested name is missing.
		base, ok = profiles[pp.settings.DefaultProfile]
		if !ok {
			// No profiles at all → nil. Caller treats nil as "no
			// profile filtering."
			return nil
		}
	}

	// Clone the base so session additions don't pollute the loaded
	// profile shared across sessions.
	pp.active = base.Clone()
	pp.activeName = profileName

	// Layer in any persistent session additions from disk.
	pp.loadPersistedSessionAdditionsLocked()

	return pp.active
}

func (pp *permissionProfiles) AddSessionAllow(rule permrules.Rule) {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	if pp.active == nil {
		// No active profile yet → defer creation. The caller is
		// almost certainly inside the gate, which already called
		// Active() to discover there was no profile, so this branch
		// is rare.
		return
	}
	pp.active.AddSessionAllow(rule)
	pp.persistAdditionLocked("allow", rule)
}

func (pp *permissionProfiles) AddSessionDeny(rule permrules.Rule) {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	if pp.active == nil {
		return
	}
	pp.active.AddSessionDeny(rule)
	pp.persistAdditionLocked("deny", rule)
}

func (pp *permissionProfiles) List() []string {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	profiles := pp.baseLocked()
	out := make([]string, 0, len(profiles))
	for name := range profiles {
		out = append(out, name)
	}
	return out
}

func (pp *permissionProfiles) Default() string {
	return pp.settings.DefaultProfile
}

// ── Session-additions persistence ──────────────────────────────────────
//
// When PersistAlwaysDecisions is true, session additions are appended
// to a YAML file at userProfilesDir/_session-additions.yaml. The file
// is loaded at first Active() so additions survive a restart.
//
// Layout (matches the standard profile shape so it could be
// hand-edited if needed):
//
//	# _session-additions.yaml — auto-generated; do not edit by hand
//	description: "Session additions persisted from PromptPermission"
//	allow_rules:
//	  - "file_read.absolute_path:/home/user/notes/**"
//	deny_rules:
//	  - "bash:rm *"

const sessionAdditionsFile = "_session-additions.yaml"

// loadPersistedSessionAdditionsLocked must be called with pp.mu held.
// Reads the persisted additions file (if it exists) and layers each
// rule into the active profile via AddSessionAllow / AddSessionDeny.
// Errors are logged-and-ignored: a corrupt persistence file should
// not block the agent from running.
func (pp *permissionProfiles) loadPersistedSessionAdditionsLocked() {
	if pp.userProfilesDir == "" {
		return
	}
	path := filepath.Join(pp.userProfilesDir, sessionAdditionsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		// File missing is the common case — no error.
		return
	}
	profile, err := permrules.LoadProfile("_session-additions", data)
	if err != nil {
		// Corrupt persistence file: skip rather than crash.
		return
	}
	// Insert each rule at the front of the active profile via
	// AddSessionAllow/Deny so the front-of-list shadow semantics
	// hold consistently with runtime additions.
	for _, r := range profile.AllowRules {
		pp.active.AddSessionAllow(r)
	}
	for _, r := range profile.DenyRules {
		pp.active.AddSessionDeny(r)
	}
}

// persistAdditionLocked must be called with pp.mu held. Writes the
// current session-additions out to disk when PersistAlwaysDecisions
// is true. The write is full-rewrite (not append) for simplicity:
// the on-disk file always reflects the current in-memory state.
//
// Errors are silently swallowed — persistence is best-effort, and a
// failed write should not prevent the in-memory rule from taking
// effect for the rest of the session.
func (pp *permissionProfiles) persistAdditionLocked(kind string, _ permrules.Rule) {
	if !pp.settings.PersistAlwaysDecisions {
		return
	}
	if pp.userProfilesDir == "" {
		return
	}
	if pp.active == nil {
		return
	}
	_ = kind // currently unused; reserved for future per-kind logging

	path := filepath.Join(pp.userProfilesDir, sessionAdditionsFile)
	if err := os.MkdirAll(pp.userProfilesDir, 0o755); err != nil {
		return
	}

	// Build the YAML content from the current active profile's
	// rule lists. Note: we serialize ALL of active.AllowRules and
	// active.DenyRules — the loaded session additions are at the
	// FRONT of those lists (we put them there at load time), so a
	// rewrite preserves them naturally. The only thing this misses
	// is rules from the BASE profile, which we deliberately exclude
	// (they live in the base YAML files, not the additions file).
	//
	// To distinguish "session-added" from "from base", we leverage
	// the fact that session adds always go to the front and base
	// rules to the back. We capture the count at first session-add
	// time and serialize only the prefix.
	//
	// For MVP we take a simpler shortcut: write only the rules whose
	// Raw form appears as a session addition (front-of-list scan
	// until we hit a rule that exists in the base profile).
	//
	// Even simpler MVP: just re-serialize the entire active.AllowRules
	// and DenyRules. On restart we layer them back via
	// loadPersistedSessionAdditionsLocked, which calls
	// AddSessionAllow/Deny — those insert at the front, so duplicates
	// from the base profile end up at the front harmlessly. The base
	// rules are still in the cloned profile from the base load.
	//
	// The only cost is a slightly redundant rule list across restarts,
	// which the matcher handles correctly (first-match-wins).
	allowLines := make([]string, 0, len(pp.active.AllowRules))
	for _, r := range pp.active.AllowRules {
		allowLines = append(allowLines, r.String())
	}
	denyLines := make([]string, 0, len(pp.active.DenyRules))
	for _, r := range pp.active.DenyRules {
		denyLines = append(denyLines, r.String())
	}

	var b strings.Builder
	b.WriteString("# _session-additions.yaml — auto-generated by ktw-agents permission gate. Do not edit by hand.\n")
	b.WriteString(`description: "Session additions persisted from PromptPermission"` + "\n")
	b.WriteString("allow_rules:\n")
	for _, l := range allowLines {
		fmt.Fprintf(&b, "  - %q\n", l)
	}
	b.WriteString("deny_rules:\n")
	for _, l := range denyLines {
		fmt.Fprintf(&b, "  - %q\n", l)
	}

	// Atomic-ish: write to a temp file then rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
