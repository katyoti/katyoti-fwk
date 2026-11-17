// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestremote "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/admin"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// ListTasksDetail returns task summaries discovered by the named active
// universe's ktw-agents core. The core's CompositeTaskLibrary aggregates
// local + sidecar (RemoteTaskLibrary) sources, so any sidecar that
// publishes a TaskLibrary slot is included here.
//
// Replaces the legacy `exec.Command("ktw-agents tasks list")` path. That
// subprocess inherited whatever KTW_WORKSPACE was set in the GUI's environment
// — wrong when multiple universes are active.
//
// Wails-bound: window.go.main.App.ListTasksDetail(universeName).
func (a *App) ListTasksDetail(universeName string) []TaskInfo {
	tasks, err := a.listTasksKTSP(universeName)
	if err != nil {
		// Surface to the frontend via the standard error event so a
		// silent empty list doesn't masquerade as "no tasks".
		a.emitError(fmt.Sprintf("ListTasksDetail %q: %v", universeName, err))
		return nil
	}
	return tasks
}

// ListSkills returns skill summaries from the named active universe's
// core. Same path as tasks — CompositeSkillManager merges the core's
// local skills with each active sidecar's remote SkillManager.
//
// Wails-bound: window.go.main.App.ListSkills(universeName).
func (a *App) ListSkills(universeName string) []SkillInfo {
	skills, err := a.listSkillsKTSP(universeName)
	if err != nil {
		a.emitError(fmt.Sprintf("ListSkills %q: %v", universeName, err))
		return nil
	}
	return skills
}

// SkillInfo mirrors skillsapi.SkillInfo for Wails binding generation.
// Keeping a GUI-side type so the frontend's generated bindings don't
// depend on internal package paths.
type SkillInfo struct {
	Name        string   `json:"name"`
	Provider    string   `json:"provider"`
	Description string   `json:"description"`
	Icon        string   `json:"icon,omitempty"`
	IconName    string   `json:"iconName,omitempty"`
	Hashtags    []string `json:"hashtags,omitempty"`
	Category    string   `json:"category,omitempty"`
}

// â”€â”€ internals â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€

// taskInfoOf is the card a task's catalog body gives: what the task lists
// show and filter on (its audience, kind and types included).
func taskInfoOf(provider, taskURI string, b mbapi.TaskBody) TaskInfo {
	return TaskInfo{
		Name:        b.Name,
		Title:       b.Title,
		Provider:    provider,
		URI:         taskURI,
		Type:        b.Type,
		Description: b.Description,
		Body:        b.Body,
		Icon:        b.Icon,
		IconName:    b.IconName,
		Hashtags:    b.Hashtags,
		Category:    b.Category,
		Tags:        b.Tags,
		Kind:        b.Kind,
		Types:       b.Types,
		Audience:    b.Audience,
	}
}

func (a *App) listTasksKTSP(universeName string) ([]TaskInfo, error) {
	return a.tasksKTSP(universeName, false)
}

// tasksKTSP lists the universe's tasks; withHidden keeps those an explicit
// `advertised: false` hides from the gallery - an action naming one by its
// URI still opens its form.
func (a *App) tasksKTSP(universeName string, withHidden bool) ([]TaskInfo, error) {
	cat, err := a.universeSnapshot(universeName)
	if err != nil {
		return nil, err
	}
	// Enumerating the universe contacts no contributor: the aggregator holds
	// the index and the query surface reads it. That is the point of the
	// catalog — listing without running anything.
	entries, err := mbapi.QueryTyped[mbapi.TaskBody](a.ctx,
		mbapi.Q{Reach: mbapi.ReachUniverse, Kind: mbapi.ResourceTypeTask, Catalog: cat})
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	// A task's contracts are their own contributions (its contract files):
	// the data-type index says which tasks have one, with no content read.
	contracts := map[string]bool{}
	if dts, err := cat.GetContributionsByType(a.ctx, contribapi.ResourceTypeDataType); err == nil {
		for _, c := range dts {
			contracts[c.ResolvedURI()] = true
		}
	}
	out := make([]TaskInfo, 0, len(entries))
	for _, e := range entries {
		b := e.Body
		b.Normalize()
		// Advertised gate: an explicit `advertised: false` hides the task
		// from the gallery (worker/sub-tasks run by top-level tasks). It
		// stays callable by exact name via the run path — only the listing
		// is filtered. Default (nil) and explicit true are shown.
		if !withHidden && b.Advertised != nil && !*b.Advertised {
			continue
		}
		// URI here is the TASK URI (`<provider>:<name>`) that
		// `taskapi.Task.URI()` produces — NOT the resource URI
		// (`<provider>:/task/<id>` from `e.ResolvedURI()`). The frontend passes
		// this string back as `TaskName` in `Terminal.StartTask`, and
		// `TaskService.resolveTask` keys its lookup map by the task
		// URI form. Resource URI form is for catalog filtering, not
		// for task runtime addressing.
		taskURI := b.Name
		if e.Provider != "" {
			taskURI = e.Provider + ":" + b.Name
		}
		// Derive the schema-resource URIs that `ContributeTaskResources`
		// auto-contributed for any task with a non-empty input/output
		// schema block. Empty when the task didn't ship a schema —
		// signals the frontend to fall back to the text-prompt run
		// surface.
		//
		// Empty now means exactly that, which it did not always: a task
		// whose schema was authored in the wrong language published a
		// URI that was PRESENT and 404'd, so the frontend took the
		// present-URI branch and rendered nothing — a task with a
		// schema degraded worse than one without. Every authored schema
		// is the DSL now and the census fails the build on one that
		// does not register, so a non-empty URI here resolves.
		var inputSchemaURI, outputSchemaURI string
		if u := mbapi.TaskSchemaURI(e.Provider, b.Name, mbapi.SchemaRoleInput); len(b.InputSchema) > 0 || contracts[u] {
			inputSchemaURI = u
		}
		if u := mbapi.TaskSchemaURI(e.Provider, b.Name, mbapi.SchemaRoleOutput); len(b.OutputSchema) > 0 || contracts[u] {
			outputSchemaURI = u
		}
		info := taskInfoOf(e.Provider, taskURI, b)
		info.InputSchemaURI, info.OutputSchemaURI = inputSchemaURI, outputSchemaURI
		out = append(out, info)
	}
	return out, nil
}

func (a *App) listSkillsKTSP(universeName string) ([]SkillInfo, error) {
	cat, err := a.universeSnapshot(universeName)
	if err != nil {
		return nil, err
	}
	// Enumerating contacts no contributor: the aggregator holds the index.
	entries, err := mbapi.Query(a.ctx,
		mbapi.Q{Reach: mbapi.ReachUniverse, Kind: mbapi.ResourceTypeSkill, Catalog: cat})
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	out := make([]SkillInfo, 0, len(entries))
	for _, e := range entries {
		// Picker fields come from meta, which the index carries — so this
		// listing needs no body at all and works on a body-free page.
		meta := e.ResolvedMetadata()
		var icon, iconName string
		if meta.Icon != nil {
			icon, iconName = meta.Icon.Emoji, meta.Icon.Tabler
		}
		out = append(out, SkillInfo{
			Name:        meta.Name,
			Provider:    e.Provider,
			Description: meta.Description,
			Icon:        icon,
			IconName:    iconName,
			Hashtags:    meta.Hashtags,
			Category:    meta.Category,
		})
	}
	return out, nil
}

// universeSender resolves the named universe to its active core's KTSP
// Sender. Activate is idempotent — fast-path returns the existing core
// without spawning a new one. Errors when the universe isn't registered
// or its core failed to spawn.
func (a *App) universeSender(universeName string) (ktsp.Sender, error) {
	a.mu.Lock()
	master := a.master
	a.mu.Unlock()
	if master == nil {
		return nil, fmt.Errorf("orchestrator master not initialized")
	}
	if universeName == "" {
		return nil, fmt.Errorf("universeName is required")
	}
	// The CONTROL-PLANE universe is infrastructure, not a user selection: the
	// management services live there, so reaching it must work before the user has
	// activated anything — that is how the GUI bootstraps at startup. This used to
	// be implicit in orchSvc()/marketplaceSvc(), which activated "admin"
	// unconditionally; when those per-method bindings were deleted in favour of the
	// generic dispatcher, the behaviour had to move here or the universe list fails
	// to load on launch.
	//
	// Every OTHER universe keeps the explicit-activation guard: silently spawning a
	// core because some panel read something would be a surprising side effect.
	if universeName != admin.UniverseName && !master.IsActive(universeName) {
		return nil, fmt.Errorf("universe %q is not active — activate it first", universeName)
	}
	core, err := master.Activate(a.ctx, universeName)
	if err != nil {
		return nil, fmt.Errorf("activate %q: %w", universeName, err)
	}
	// Every call carries the person using the GUI as its cause
	// (app_person_cause.go): the writes made for it say who made them.
	return a.withPersonCause(core.Sender()), nil
}

// universeCatalog returns a reader over one universe's catalog.
//
// The GUI drives several universes at once, so the reader is per universe and
// holds nothing but that universe's Sender — 13 first-party packages, against
// the 55 an in-process catalog costs. That weight is the reason every browser
// here used to hand-build the aggregator's address instead: importing the real
// catalog to read a list would have roughly doubled this binary.
//
// With this, every browser asks through the one query surface and gets what the
// surface provides — the decode cache, the typed view with meta, the recorded
// verdict — rather than re-deriving each of them from the wire.
func (a *App) universeCatalog(universeName string) (*manifestremote.Catalog, error) {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return nil, err
	}
	return manifestremote.New(sender), nil
}

// universeSnapshot is universeCatalog for a listing that reads MANY BODIES.
//
// A bare Catalog holds nothing by design, so locating an entry to fetch its
// body re-reads the whole paginated index over the bus — and a listing of N
// bodies paid N index fetches plus N body fetches. Measured on the task
// gallery: 63 tasks, ~127 round trips, four seconds, every time the panel
// drew. A snapshot reads the index once and answers locating from memory.
//
// Deliberately NOT the default: a caller that reads one thing should not hold
// the world, and a snapshot is a moment rather than a live view.
func (a *App) universeSnapshot(universeName string) (*manifestremote.Snapshot, error) {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return nil, err
	}
	return manifestremote.TakeSnapshot(a.ctx, sender)
}
