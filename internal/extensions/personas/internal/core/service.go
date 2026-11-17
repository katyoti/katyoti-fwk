// SPDX-License-Identifier: Apache-2.0

// Package core implements the persona library (Resolve / List
// / Spawn). Personas are NOT a new task type — they're TASK.md
// files of type `agent-loop` plus persona-specific frontmatter,
// shipped by method-pack sidecars through the manifest catalog
// (`mbapi.ResourceTypePersona`).
//
// The library reads the catalog at every call (the catalog is
// already cached at the application layer; this service doesn't
// re-cache). Workspace + universe directory tiers can override
// catalog entries by name — deferred to v2; v1 reads catalog only.
//
// Spawn integration with the agent-loop TaskService is scaffolded
// — the lookup + validation + spawn_id allocation work today; the
// final TaskService.Run call is wired through `application.UseService`
// at call time so the catalog and the TaskService remain
// independently resolvable.
package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/google/uuid"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/personas/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// Service implements api.PersonaService.
type Service struct{}

var _ api.PersonaService = (*Service)(nil)

// New returns a ready-to-use Service. No state — Resolve/List
// fetch the catalog at every call; Spawn fetches the TaskService
// at every call (both via `application.UseService`).
func New() *Service { return &Service{} }

// Resolve walks the catalog for an entry of type "persona" whose
// ID matches name. Returns the first match. v1 doesn't yet
// implement the 3-tier dir override layer (filed as a v2 TODO).
func (s *Service) Resolve(ctx stdctx.Context, in api.ResolveInput) (api.ResolveOutput, error) {
	if in.Name == "" {
		return api.ResolveOutput{},
			fmt.Errorf("%w: persona name required", api.ErrInvalidInput)
	}
	body, err := s.resolveBody(ctx, in.Name)
	if err != nil {
		return api.ResolveOutput{}, err
	}
	return api.ResolveOutput{Persona: toPersonaInfo(body)}, nil
}

// resolveBody is the internal counterpart of Resolve that returns
// the full PersonaBody (including Body + Config) without going
// through the slim public PersonaInfo. Used by Spawn so it can
// build a TaskRunRequest from the raw definition.
func (s *Service) resolveBody(ctx stdctx.Context, name string) (mbapi.PersonaBody, error) {
	return s.resolveBodyFor(ctx, name, nil)
}

// resolveBodyFor resolves a persona for the work `sc` states: a specialised
// layer merges only when it matches (every layer when sc is nil).
func (s *Service) resolveBodyFor(ctx stdctx.Context, name string, sc *api.SpawnContext) (mbapi.PersonaBody, error) {
	bodies, err := loadFromCatalogFor(ctx, sc)
	if err != nil {
		return mbapi.PersonaBody{}, err
	}
	for _, b := range bodies {
		if b.Name == name {
			return b, nil
		}
	}
	return mbapi.PersonaBody{}, fmt.Errorf("%w: persona %q", api.ErrNotFound, name)
}

// List returns the catalog's personas filtered by Category /
// Type / Tier. Empty filters = no narrowing.
func (s *Service) List(ctx stdctx.Context, in api.ListInput) (api.ListOutput, error) {
	bodies, err := loadFromCatalog(ctx)
	if err != nil {
		return api.ListOutput{}, err
	}
	var out []api.Info
	for _, b := range bodies {
		if in.Category != "" && b.Category != in.Category {
			continue
		}
		if in.Type != "" && !allowsType(b.Types, in.Type) {
			continue
		}
		if in.Tier != "" && string(b.Tier) != in.Tier {
			continue
		}
		out = append(out, toPersonaInfo(b))
	}
	return api.ListOutput{Personas: out}, nil
}

// Spawn looks up the persona, validates the bind mode, allocates a spawn_id, builds a TaskRunRequest
// from the persona's body + config + persona-injected inputs,
// and submits it to the TaskService.
//
// Returns `Status: "rejected"` on validation failure (caller
// inspects `Reason`). On submission success returns
// `Status: "running"` + the task UUID as the spawn ID.
func (s *Service) Spawn(ctx stdctx.Context, in api.SpawnInput) (api.SpawnOutput, error) {
	if in.Name == "" {
		return api.SpawnOutput{},
			fmt.Errorf("%w: persona name required", api.ErrInvalidInput)
	}

	// Resolve to the full PersonaBody so we can build the
	// TaskRunRequest from its body + config.
	persona, err := s.resolveBodyFor(ctx, in.Name, in.Context)
	if err != nil {
		return api.SpawnOutput{}, err
	}

	// Bind-mode validation.
	switch persona.BindMode {
	case "object", "":
		if in.Target == "" {
			return api.SpawnOutput{
				Status: "rejected",
				Reason: "persona requires a target (bind_mode=object)",
			}, nil
		}
	case "global":
		// no target required.
	case "custom":
		// Routed to object until custom binding is built.
		if in.Target == "" {
			return api.SpawnOutput{
				Status: "rejected",
				Reason: "persona bind_mode=custom not yet implemented; provide a target",
			}, nil
		}
	}

	// Types is not checked against the target here: the scheduler spawns
	// personas on a request, a type no persona lists. Whether the target
	// is the request or the object it concerns is decided apart.

	// Allocate the spawn ID. UUID v7 (time-sortable, same shape
	// as everywhere else in CWM).
	spawnUUID, err := uuid.NewV7()
	if err != nil {
		return api.SpawnOutput{}, fmt.Errorf("alloc spawn_id: %w", err)
	}
	spawnID := spawnUUID.String()

	// Build a TaskRunRequest. The persona's frontmatter + body
	// become an inline TaskDef of type agent-loop; the runtime
	// input carries target + gate so the agent loop can
	// read them via plugin context.
	taskDef := map[string]any{
		"type":        "agent-loop",
		"name":        persona.Name,
		"description": persona.Description,
		"body":        persona.Body, // markdown after TASK.md frontmatter
	}
	// The persona's config block (plugins, profile, turns) is the task's.
	if len(persona.Config) > 0 {
		var cfg map[string]any
		if err := json.Unmarshal(persona.Config, &cfg); err == nil {
			taskDef["config"] = cfg
		}
	}

	// Inputs: caller's `inputs` map + framework-injected fields
	// (target, gate, spawn_id, parent_spawn_id).
	input := map[string]any{}
	for k, v := range in.Inputs {
		input[k] = v
	}
	input["_target"] = in.Target
	input["_gate"] = stringOrDefault(in.Gate, persona.DefaultGate)
	input["_spawn_id"] = spawnID
	input["_parent_spawn_id"] = in.ParentSpawnID

	// Submit via TaskService — resolved lazily through ctx.
	ts, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil || ts == nil {
		// Without a TaskService we can't actually spawn. Return
		// a queued status so the caller knows the persona was
		// validated + spawn_id allocated, but the run hasn't
		// started.
		return api.SpawnOutput{
			SpawnID: spawnID,
			Status:  "queued",
			Reason:  "TaskService not available in context; spawn_id allocated but run not started",
		}, nil
	}

	resp, err := ts.Run(ctx, taskapi.TaskRunRequest{
		TaskDef:     taskDef,
		Input:       input,
		ParentRunID: in.ParentSpawnID,
		SessionID:   in.SessionID,
		Group:       in.Group,
		Priority:    in.Priority,
		Origin:      in.Origin,
		Target:      in.Target,
	})
	if err != nil {
		return api.SpawnOutput{
			SpawnID: spawnID,
			Status:  "rejected",
			Reason:  fmt.Sprintf("TaskService.Run: %v", err),
		}, nil
	}

	return api.SpawnOutput{
		SpawnID: spawnID,
		TaskID:  resp.RunID,
		Status:  "running",
	}, nil
}

// loadFromCatalog reads every "persona" resource entry across all
// cached manifests, decodes the bodies, and tags each with its
// tier (always "built-in" for catalog-shipped personas in v1).
func loadFromCatalog(ctx stdctx.Context) ([]mbapi.PersonaBody, error) {
	return loadFromCatalogFor(ctx, nil)
}

// loadFromCatalogFor is loadFromCatalog for the work sc states.
func loadFromCatalogFor(ctx stdctx.Context, sc *api.SpawnContext) ([]mbapi.PersonaBody, error) {
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil || cat == nil {
		// No catalog = no built-in personas. Returning empty is
		// the right behaviour (tests run without a catalog).
		return nil, nil
	}
	// Two-plane index: decode every persona body lazily via
	// Catalog.GetBody (content-store hit / one bounded per-hash fetch).
	attributed, err := mbapi.QueryTyped[mbapi.PersonaBody](ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: mbapi.ResourceTypePersona, Catalog: cat})
	if err != nil {
		return nil, fmt.Errorf("personas: load catalog: %w", err)
	}
	bodies := make([]mbapi.PersonaBody, 0, len(attributed))
	for _, a := range attributed {
		b := a.Body
		b.Tier = "built-in"
		bodies = append(bodies, b)
	}
	return mergeAugmentationsFor(bodies, sc), nil
}

// mergeAugmentations folds every augmentation (PersonaBody with a
// non-empty Augments naming a baseline) INTO its baseline, returning
// one merged PersonaBody per baseline name plus any baselines that
// had no augmentations. Augmentations whose baseline is absent are
// DROPPED — a half-persona never surfaces.
//
// Merge (v1):
//   - Body: baseline, then each augmentation under a `\n\n---\n\n`
//     separator (augmentations are authored as addenda).
//   - Skills + Types: union, baseline order first.
//   - Config: shallow key-merge, augmentation wins per key; the
//     `plugins` sub-list unions.
//   - Description/Icon*/BindMode/DefaultGate: baseline wins; an
//     augmentation fills only when the baseline left it empty.
//
// Determinism: augmentations apply sorted by (Name) — catalog order
// is not stable across map-backed sources.
func mergeAugmentations(bodies []mbapi.PersonaBody) []mbapi.PersonaBody {
	return mergeAugmentationsFor(bodies, nil)
}

// mergeAugmentationsFor is mergeAugmentations for the work sc states: an
// augmentation whose applies_to does not match sc is left out. With no
// context (nil), every augmentation merges — the work is unknown.
func mergeAugmentationsFor(bodies []mbapi.PersonaBody, sc *api.SpawnContext) []mbapi.PersonaBody {
	baseIdx := map[string]int{} // name → index in out
	var out []mbapi.PersonaBody
	var augs []mbapi.PersonaBody

	for _, b := range bodies {
		if b.Augments != "" {
			augs = append(augs, b)
			continue
		}
		baseIdx[b.Name] = len(out)
		out = append(out, b)
	}

	sort.SliceStable(augs, func(i, j int) bool { return augs[i].Name < augs[j].Name })
	for _, a := range augs {
		idx, ok := baseIdx[a.Augments]
		if !ok {
			// Orphan augmentation — no baseline of that name. Drop it.
			continue
		}
		if !appliesTo(a.AppliesTo, sc) {
			continue
		}
		out[idx] = applyAugmentation(out[idx], a)
	}
	return out
}

// applyAugmentation merges one augmentation into a baseline per the
// v1 semantics documented on mergeAugmentations.
func applyAugmentation(base, aug mbapi.PersonaBody) mbapi.PersonaBody {
	if aug.Body != "" {
		if base.Body != "" {
			base.Body += "\n\n---\n\n"
		}
		base.Body += aug.Body
	}
	base.Skills = unionStrings(base.Skills, aug.Skills)
	base.Types = unionStrings(base.Types, aug.Types)
	base.Config = mergeConfig(base.Config, aug.Config)

	if base.Description == "" {
		base.Description = aug.Description
	}
	if base.Icon == "" {
		base.Icon = aug.Icon
	}
	if base.IconName == "" {
		base.IconName = aug.IconName
	}
	if base.BindMode == "" {
		base.BindMode = aug.BindMode
	}
	if base.DefaultGate == "" {
		base.DefaultGate = aug.DefaultGate
	}
	return base
}

// unionStrings appends b's entries to a, skipping duplicates,
// preserving a's order then b's. Returns nil when both are empty.
func unionStrings(a, b []string) []string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range append(append([]string{}, a...), b...) {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// mergeConfig shallow-merges two JSON config blobs: augmentation
// keys win, except `plugins` (a list) which unions. Returns the
// baseline blob untouched when the augmentation carries no config.
func mergeConfig(baseRaw, augRaw json.RawMessage) json.RawMessage {
	if len(augRaw) == 0 {
		return baseRaw
	}
	if len(baseRaw) == 0 {
		return augRaw
	}
	var baseM, augM map[string]any
	if err := json.Unmarshal(baseRaw, &baseM); err != nil {
		return baseRaw
	}
	if err := json.Unmarshal(augRaw, &augM); err != nil {
		return baseRaw
	}
	for k, v := range augM {
		if k == "plugins" {
			baseM[k] = unionAny(baseM[k], v)
			continue
		}
		baseM[k] = v
	}
	merged, err := json.Marshal(baseM)
	if err != nil {
		return baseRaw
	}
	return merged
}

// unionAny unions two values that are expected to be string lists
// (the `plugins` config key). Non-list values fall back to the
// augmentation value.
func unionAny(base, aug any) any {
	toStrs := func(v any) ([]string, bool) {
		raw, ok := v.([]any)
		if !ok {
			return nil, false
		}
		out := make([]string, 0, len(raw))
		for _, e := range raw {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	bs, bok := toStrs(base)
	as, aok := toStrs(aug)
	if !bok || !aok {
		return aug
	}
	merged := unionStrings(bs, as)
	anyOut := make([]any, len(merged))
	for i, s := range merged {
		anyOut[i] = s
	}
	return anyOut
}

// toPersonaInfo converts the wire-shape PersonaBody into the
// api-level Info Resolve and List return. The agent-loop
// prompt body is carried in a separate field so Spawn can inject
// it into the TaskRunRequest without exposing it on the public
// list/resolve surface (it's large + only relevant at run time).
func toPersonaInfo(b mbapi.PersonaBody) api.Info {
	return api.Info{
		Name:        b.Name,
		Category:    b.Category,
		Tier:        api.Tier(b.Tier),
		Description: b.Description,
		Icon:        b.Icon,
		Types:       b.Types,
		BindMode:    b.BindMode,
		DefaultGate: b.DefaultGate,
		Skills:      b.Skills,

		// Path carries the persona name as a lightweight
		// resolution hint. The full Body + Config aren't
		// exposed on Info — Spawn re-reads them via
		// resolveBody when it needs to build a TaskRunRequest.
		Path: b.Name,
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func stringOrDefault(s, dflt string) string {
	if s != "" {
		return s
	}
	return dflt
}

// appliesTo says whether a layer's scope matches the work: no scope, or no
// context, always does; otherwise each list the scope sets must match — a
// file the work touches matches one of its globs, the work's module is one
// of its modules.
func appliesTo(scope *mbapi.PersonaScope, sc *api.SpawnContext) bool {
	if scope == nil || sc == nil {
		return true
	}
	if len(scope.Files) > 0 && !anyFileMatches(scope.Files, sc.Files) {
		return false
	}
	if len(scope.Modules) > 0 && !contains(scope.Modules, sc.Module) {
		return false
	}
	return true
}

func anyFileMatches(globs, files []string) bool {
	for _, f := range files {
		f = strings.TrimPrefix(filepath.ToSlash(f), "./")
		for _, g := range globs {
			if ok, _ := doublestar.Match(g, f); ok {
				return true
			}
		}
	}
	return false
}

// allowsType reports whether a persona whose allow-list is types may target
// an object of type typ: an empty list or `*` allows any.
func allowsType(types []string, typ string) bool {
	return len(types) == 0 || contains(types, "*") || contains(types, typ)
}
