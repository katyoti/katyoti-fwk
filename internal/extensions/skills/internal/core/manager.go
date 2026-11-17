// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"path"
	"sync"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
)

// loaderBackedProvider is the consumer-defined interface SkillManager uses
// to recover the underlying (Loader, Root) tuple from a ContributionProvider.
// EmbedFSResourceProvider — the generic producer every skill and reference
// is published through — satisfies it.
//
// Defining the interface here (consumer-side) avoids forcing every
// ContributionProvider impl to know about SkillManager's needs and lets the
// `manifest/api` package stay neutral re: loader.Loader.
type loaderBackedProvider interface {
	contribapi.ContributionProvider
	SourceLoader() loader.Loader
	SourceRoot() string
}

// referenceSource is the per-provider record SkillManager keeps for
// runtime GetReference. Replaces the api.ReferenceLoader struct (Phase C).
type referenceSource struct {
	loader loader.Loader
	root   string
}

// skillManager implements api.SkillManager.
//
// Maps are guarded by `mu` (RWMutex). Readers (GetSkill /
// GetSkillRequires) hold a read lock; Reload atomically swaps under a
// write lock. Reload is all-or-nothing: a scan failure keeps the
// existing maps so subscribers don't briefly observe an empty registry.
type skillManager struct {
	cfg *SkillManagerConfig
	ctx context.Context // captured at construction for Reload's UseExtensions call

	mu         sync.RWMutex
	skills     map[string]api.Skill
	references map[string]referenceSource // provider â†’ reference loader/root
}

// NewSkillManager is the service factory for SkillManager.
//
// Reads `mbapi.ContributionProvidersPoint` and filters by resource type:
//   - Type() == "skill" â†’ walks the provider's Loader/Root for SKILL.md
//     files and builds the in-memory skill map (runtime ops: RunScript,
//     GetSkill, Reference-within-skill).
//   - Type() == "reference" â†’ indexes the provider's Loader/Root by
//     provider name so GetReference can dispatch.
//
// `cfg.ExtraDirs` are still honored — each becomes an ad-hoc skill source
// (provider name "user") so user-installed skills join the in-memory map
// alongside the embedded ones.
//
// The ctx is captured for later Reload calls (Reload re-walks
// ContributionProvidersPoint, which needs the application context to
// resolve `application.UseExtensions`).
func NewSkillManager(ctx context.Context, cfg *SkillManagerConfig) (api.SkillManager, error) {
	skills, refs, err := scanSkillSources(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &skillManager{
		cfg:        cfg,
		ctx:        ctx,
		skills:     skills,
		references: refs,
	}, nil
}

// scanSkillSources is the read-only helper that walks
// ContributionProvidersPoint + cfg.ExtraDirs and builds fresh skill /
// reference maps. Called by NewSkillManager at construction and by
// Reload to refresh — the swap is the caller's responsibility.
func scanSkillSources(ctx context.Context, cfg *SkillManagerConfig) (map[string]api.Skill, map[string]referenceSource, error) {
	type skillSource struct {
		provider string
		loader   loader.Loader
		root     string
	}
	var skillSrcs []skillSource
	refs := map[string]referenceSource{}

	for _, p := range application.UseExtensions[contribapi.ContributionProvider](ctx) {
		lb, ok := p.(loaderBackedProvider)
		if !ok {
			// Non-loader-backed providers (e.g., static/yaml ones) can't host
			// scripts or references — skip them for SkillManager's purposes.
			// They still feed the catalog via Enumerate.
			continue
		}
		switch p.Type() {
		case mbapi.ResourceTypeSkill:
			skillSrcs = append(skillSrcs, skillSource{
				provider: p.Provider(),
				loader:   lb.SourceLoader(),
				root:     lb.SourceRoot(),
			})
		case mbapi.ResourceTypeReference:
			refs[p.Provider()] = referenceSource{
				loader: lb.SourceLoader(),
				root:   lb.SourceRoot(),
			}
		}
	}

	for _, dir := range cfg.ExtraDirs {
		skillSrcs = append(skillSrcs, skillSource{
			provider: "user",
			loader:   loader.NewFSLoader(dir),
			root:     ".",
		})
	}

	skills := make(map[string]api.Skill)
	for _, src := range skillSrcs {
		// Discover skill .md files at multiple levels:
		//   - <root>/*.md           (flat single-file skills)
		//   - <root>/*/*.md         (folder-based or grouped skills)
		//   - <root>/*/*/*.md       (nested groups)
		var entries []string
		for _, pattern := range []string{
			path.Join(src.root, "*.md"),
			path.Join(src.root, "*", "*.md"),
			path.Join(src.root, "*", "*", "*.md"),
		} {
			entries = append(entries, src.loader.List(pattern)...)
		}

		for _, entry := range entries {
			dir := path.Dir(entry)
			data, err := src.loader.ReadFile(entry)
			if err != nil {
				continue
			}
			skill, err := NewSkillFromBytes(src.loader, dir, data)
			if err != nil {
				continue
			}
			if ls, ok := skill.(*loaderSkill); ok {
				ls.skill.Provider = src.provider
			}
			info := skill.Title()
			skills[info.URI()] = skill // provider:name
		}
	}

	return skills, refs, nil
}

// List + GetInstructions removed in step 13 of
// bus-split-aggregator-to-catalog — enumeration goes through the
// catalog (`api.SkillsFromCatalog`), L1 instructions through
// `api.InstructionsFromCatalog`.
//
// GetReference + RunScript were dropped 2026-05-24 — references are
// first-class Contributions of type `reference` addressed via
// `ResourceService.Get(<provider>:/reference/<path>)`. Standalone
// scripts use the `<provider>:scripts/<name>` slot. The local
// in-memory skill map is still walked by `GetSkill` for the in-process
// agent plugin accessor.

// GetSkill looks up a skill by provider:name URI.
// Also falls back to name-only lookup for backward compat.
func (m *skillManager) GetSkill(uri string) (api.Skill, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Direct lookup by provider:name
	if skill, ok := m.skills[uri]; ok {
		return skill, nil
	}
	// Backward compat: try name-only (scan all skills)
	for _, skill := range m.skills {
		if skill.Title().Name == uri {
			return skill, nil
		}
	}
	return nil, fmt.Errorf("skill %q not found", uri)
}

func (m *skillManager) GetSkillRequires(uri string) []string {
	skill, err := m.GetSkill(uri)
	if err != nil {
		return nil
	}
	return skill.Title().Requires
}

// Reload re-scans every ContributionProvider + cfg.ExtraDirs and
// atomically swaps the in-memory skill / reference maps. Atomic
// all-or-nothing: a scan error keeps the existing maps so subscribers
// never observe an empty registry. The ctx arg is accepted for
// interface conformance but the underlying scan uses the ctx captured
// at NewSkillManager (which carries the Application — needed for
// `application.UseExtensions`).
func (m *skillManager) Reload(_ context.Context) error {
	skills, refs, err := scanSkillSources(m.ctx, m.cfg)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.skills = skills
	m.references = refs
	m.mu.Unlock()
	return nil
}

func contains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}
