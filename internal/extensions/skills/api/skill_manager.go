// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// SkillManager is the in-process accessor for skill metadata.
//
// Post-2026-05-24 (ms-rc1 normalization): the RPC methods
// `GetReference` and `RunScript` were DROPPED — references are
// first-class Contributions of type `reference` addressed via
// `ResourceService.Get(<provider>:/reference/<path>)`, and
// standalone executable scripts live as their own `<provider>:scripts/<name>`
// slot (per script-uri-addressing). Skill-bundled scripts had no live
// consumer; if a future use case re-emerges it'll plug into the
// existing scripts slot rather than a parallel SkillManager surface.
//
// SkillManager today carries ONLY the in-process accessors the agent
// plugin needs to find skill metadata at tool-call time. No published
// service methods — the `skills` KTSP slot publishes no callable verbs.
// Enumeration (L0 metadata) + instruction fetches (L1 body) ride on
// `mbapi.Catalog` (see `api.SkillsFromCatalog` /
// `api.InstructionsFromCatalog`).
type SkillManager interface {
	// GetSkill returns the Skill instance by name (in-process only —
	// the published surface doesn't include this).
	GetSkill(name string) (Skill, error)

	// GetSkillRequires returns the requires field for a named skill.
	// Used by the plugin resolver for transitive dependency resolution.
	// Returns nil if the skill doesn't exist or has no requires.
	GetSkillRequires(name string) []string

	// Reload re-scans every ContributionProvidersPoint provider and
	// rebuilds the in-memory skill / reference maps atomically. Readers
	// see consistent state at all times — the swap is mutex-guarded.
	//
	// Wired to `ContributionsChangedSubject` subscriptions (per
	// [agents-subscribe-contributions-changed]) so newly-added skills
	// surface without a process restart. Errors are returned for the
	// caller to log; the existing maps are kept on failure (atomic
	// "all-or-nothing" rebuild).
	Reload(ctx context.Context) error
}

// SkillManagerDef is the published service definition. After the
// 2026-05-24 RPC drop, the service publishes ZERO methods — it's
// declared only to satisfy the framework's PublishService contract
// and to keep a stable place for in-process clients to resolve
// `application.UseService[SkillManager]`. The skills surface is
// catalog-served + contributions-served end to end.
var SkillManagerDef = meta.ServiceDef{
	Name:    "skills",
	Short:   "Skill registry (in-process accessors only — no RPC)",
	Long:    "In-process accessor for skill metadata (GetSkill / GetSkillRequires). All published surface is catalog-served: enumeration via ResourceService.List(type=skill), instructions via ResourceService.Get, references via ResourceService.Get on type=reference URIs.",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		// Intentionally empty — no published RPC.
	},
}
