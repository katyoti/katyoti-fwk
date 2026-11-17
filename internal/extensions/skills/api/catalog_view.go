// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"fmt"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// ErrSkillNotInCatalog is returned by `InstructionsFromCatalog` when
// the requested skill URI doesn't resolve to a known catalog entry —
// either the provider isn't loaded or the provider's manifest doesn't
// contain a `type: skill` entry with the matching ID.
var ErrSkillNotInCatalog = fmt.Errorf("skill not in catalog")

// SkillInfoFromCatalog projects a catalog `Manifest` + `Contribution` pair
// into the legacy `SkillInfo` shape that today's `SkillManager` consumers
// expect. Lets a consumer that currently does:
//
//	out, _ := mgr.List(ctx, ListInput{})
//	for _, s := range out.Skills { ... }
//
// migrate to:
//
//	skills, _ := api.SkillsFromCatalog(ctx, catalog)
//	for _, s := range skills { ... }
//
// without rewriting the consumer's iteration logic. The conversion is
// pure — no RPC, no allocation beyond the slice header.
//
// Every picker field comes from `meta` and every wiring field from the body,
// which is the split the two are for: meta is what a list shows, the body is
// what the runtime uses. They used to be read from the same place, and the
// duplicate copies of a name could disagree.
//
// `Instructions` is deliberately not projected: it is the skill's `source`
// layer, fetched by whoever actually reads the skill rather than carried into
// every listing.
func SkillInfoFromCatalog(provider string, meta contribapi.ContributionMeta, body mbapi.SkillBody) SkillInfo {
	var icon, iconName string
	if meta.Icon != nil {
		icon, iconName = meta.Icon.Emoji, meta.Icon.Tabler
	}
	return SkillInfo{
		Name:        meta.Name,
		Provider:    provider,
		Description: meta.Description,
		Requires:    body.Requires,
		Icon:        icon,
		IconName:    iconName,
		Hashtags:    meta.Hashtags,
		Category:    meta.Category,
		Provides:    body.Provides,
		// Default true; only an explicit `advertised: false` hides it from
		// bulk listings (an exact name still reaches it).
		Advertised: meta.IsAdvertised(),
	}
}

// InstructionsFromCatalog returns the L1 instructions body for a skill
// addressed by its canonical full URI `<provider>:/skill/<name>`
// (e.g. `ktw-docs:/skill/doc-create`). The catalog body is already
// populated by `sidecarDiscoveryExt` at Bootstrap via each sidecar's
// resource-list probe — no per-call RPC needed.
//
// Returns `ErrSkillNotInCatalog` if the provider isn't loaded OR the
// loaded manifest doesn't contain a matching skill entry.
//
// Updated 2026-05-24: switched from the short `provider:name` form to
// the full URI form to match every other surface (matches
// `<provider>:/<kind>/<id>` resource-URI grammar locked by the
// the command-surface decision; rejects callers passing the legacy short
// form).
func InstructionsFromCatalog(ctx context.Context, cat mbapi.Catalog, skillURI string) (string, error) {
	if cat == nil {
		return "", ErrSkillNotInCatalog
	}
	uri, err := contribapi.ParseResourceURI(skillURI)
	if err != nil {
		return "", fmt.Errorf("InstructionsFromCatalog: malformed skill URI %q (want %q): %w", skillURI, "<provider>:/skill/<name>", err)
	}
	if uri.Type != mbapi.ResourceTypeSkill {
		return "", fmt.Errorf("InstructionsFromCatalog: URI %q is type %q, not skill", skillURI, uri.Type)
	}
	if uri.Provider == "" {
		return "", fmt.Errorf("InstructionsFromCatalog: URI %q has no provider segment", skillURI)
	}
	// One entry's INSTRUCTIONS layer, addressed: the markdown body the
	// producer split from the file, fetched by whoever reads it. Nothing here
	// parses a frontmatter fence — that happened once, producer-side. Listing
	// the kind to find one skill would cost as much as building a whole agent.
	layer, err := mbapi.Open(ctx,
		mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}, skillURI, mbapi.LayerInstructions)
	if err != nil {
		if errors.Is(err, mbapi.ErrNotInView) || errors.Is(err, mbapi.ErrSidecarNotInCatalog) {
			return "", ErrSkillNotInCatalog
		}
		return "", fmt.Errorf("InstructionsFromCatalog: %w", err)
	}
	raw, err := layer.Bytes()
	if err != nil {
		return "", fmt.Errorf("InstructionsFromCatalog: read %s: %w", skillURI, err)
	}
	if len(raw) == 0 {
		return "", ErrSkillNotInCatalog
	}
	return string(raw), nil
}

// SkillsFromCatalog walks every loaded manifest in the catalog,
// extracts `type: skill` resource entries, and returns them as a flat
// `[]SkillInfo`. The order matches the catalog's iteration (provider
// declared order; ID-sorted within a provider).
//
// Unadvertised skills (`SkillBody.Advertised == &false`) are filtered
// out here so the `skills list` surface and the `<skills_system>` block
// stay uncluttered. The catalog itself still carries them — direct
// URI lookups via `InstructionsFromCatalog` (or `requires:` resolution
// from a sibling skill) keep working. This matches the documented
// intent on `SkillMeta.Advertised` (visibility ≠ callability).
//
// Empty catalog (no descriptors loaded) returns an empty slice + nil
// error — consumers should treat that as "no remote skills available
// yet" rather than an error condition.
//
// This is the SOURCE-SIDE replacement for `SkillManager.List(ctx,
// ListInput{})` across all consumers (agent system prompt builder,
// delegate tool, GUI Inspector). Per `bus-split-aggregator-to-catalog`.
func SkillsFromCatalog(ctx context.Context, cat mbapi.Catalog) ([]SkillInfo, error) {
	return skillsFromCatalog(ctx, cat, false)
}

// SkillsFromCatalogAll is like SkillsFromCatalog but INCLUDES unadvertised
// skills, each carrying its real `Advertised` flag. The per-agent skill plugin
// resolves selectors against this full set so an EXACT `skills:` name can reach
// an unadvertised skill, while bulk selectors (`*`, glob) still match only
// advertised ones (the refmatch advertised gate, now fed real flags). The
// global discovery surface (`skills list`, pickers) keeps using
// SkillsFromCatalog (advertised-only).
func SkillsFromCatalogAll(ctx context.Context, cat mbapi.Catalog) ([]SkillInfo, error) {
	return skillsFromCatalog(ctx, cat, true)
}

func skillsFromCatalog(ctx context.Context, cat mbapi.Catalog, includeUnadvertised bool) ([]SkillInfo, error) {
	if cat == nil {
		return nil, nil
	}
	// Two-plane index: pull every `type: skill` body once, resolving each
	// lazily via Catalog.GetBody. (The SkillBody carries the picker
	// fields — Name/Description/Hashtags/Category — alongside the heavy
	// Instructions; the content store keeps repeated lazy fetches cheap.)
	entries, err := mbapi.QueryTyped[mbapi.SkillBody](ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: mbapi.ResourceTypeSkill, Catalog: cat})
	if err != nil {
		return nil, err
	}
	var out []SkillInfo
	// Dedup by skill URI (provider+name): the same skill can appear in more
	// than one manifest/layer (e.g. a methods skill carried by both its own
	// provider's manifest and an aggregating one), which would otherwise list
	// it multiple times in the <skills_system> block. Same provider+name = a
	// true duplicate (a layer OVERRIDE has a different provider token), so
	// first-wins is safe.
	seen := map[string]bool{}
	for _, e := range entries {
		info := SkillInfoFromCatalog(e.Provider, e.Meta, e.Body)
		if !includeUnadvertised && !info.Advertised {
			continue
		}
		if seen[info.URI()] {
			continue
		}
		seen[info.URI()] = true
		out = append(out, info)
	}
	return out, nil
}
