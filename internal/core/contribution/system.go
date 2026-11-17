// SPDX-License-Identifier: Apache-2.0

// Package contribution carries the system meta-model every binary embeds —
// the kind declarations and well-known data-types under embed/ — and the
// loader that publishes them as `system:/…` contributions
// (SystemContributionsExtension). The typed contract lives in api/.
//
// A `Kind[T]` builder used to live here too: a generic provider over an EM
// point. It had no registration in any binary, so it went (d1, 2026-09-21);
// providers are written against contribapi.ContributionProvider directly.
package contribution

import (
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// Well-known contribution-type names — the kebab discriminators that
// appear in `Contribution.Type` for the open list of system kinds.
// Use these constants instead of literal strings so a typo at one call
// site fails the compiler instead of producing a broken Contribution.
const (
	// TypeContributionType is the meta-meta discriminator: a
	// contribution-type entry IS itself a contribution of type
	// "contribution-type". The bootstrap chain bottoms out at
	// `system:/contribution-type/contribution-type` which is
	// self-referential.
	TypeContributionType = "contribution-type"

	// TypeDataType is the discriminator for canonical data-type
	// contributions. The DSL is the source layer; the media type says
	// whether it is YAML or JSON.
	// Mirrors contribapi.ResourceTypeDataType — re-exported here so
	// system-loader code stays self-contained.
	TypeDataType = contribapi.ResourceTypeDataType

	// TypeService is the discriminator for service-interface
	// contributions. Body holds the meta.ServiceDef in JSON.
	TypeService = "service"

	// TypeSkill, TypeTask, TypeReference — well-known concrete kinds
	// retained as constants for the same compile-time-safety reason
	// (sidecars producing these types use the constant rather than a
	// literal). These don't necessarily have system-scope entries; the
	// constants just document the kebab token.
	TypeSkill     = "skill"
	TypeTask      = "task"
	TypeReference = "reference"
)

// SystemContributionTypeURI returns the canonical URI of the
// contribution-type entry that defines a given Type's schema. Use it
// when setting `Contribution.Schema` on items whose schema lives in
// the embedded system layer (the common case).
//
// Example:
//
//	SystemContributionTypeURI("data-type")
//	  → "system:/contribution-type/data-type"
//
// Note this is the URI of the contribution-type ENTRY, not the
// data-type's schema URI. For body-shape lookups use the
// data-type URI directly: `system:/data-type/<Type>`.
func SystemContributionTypeURI(typ string) string {
	return contribapi.SystemProvider + ":/" + TypeContributionType + "/" + typ
}

// SystemDataTypeURI returns the canonical URI of a system-scope
// data-type entry. Use it for the `Schema` field on Contributions
// whose body is described by a well-known system data-type.
//
// Example:
//
//	SystemDataTypeURI("contribution")
//	  → "system:/data-type/contribution"
func SystemDataTypeURI(name string) string {
	return contribapi.SystemProvider + ":/" + TypeDataType + "/" + name
}

// SystemServiceURI returns the canonical URI of a system-scope
// service-interface entry.
//
// Example:
//
//	SystemServiceURI("SkillService")
//	  → "system:/service/SkillService"
func SystemServiceURI(name string) string {
	return contribapi.SystemProvider + ":/" + TypeService + "/" + name
}
