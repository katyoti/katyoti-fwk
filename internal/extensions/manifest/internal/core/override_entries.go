// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"log/slog"
	"slices"
	"sort"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// Override entries take effect by origin (decision):
// the binary's entry first, then the project's override entry over it,
// layer by layer; the universe's override tree keeps its last word on a layer
// it already served (it is applied at source, in the producer, under
// allow_overwrites). The builder applies the override entries whose entry it
// serves; the others travel in the manifest (Manifest.OverrideEntries) and the
// catalog applies them over the member that serves the entry.

// sortOverrideEntries orders override entries by the entry they override,
// then their own URI: the order they are applied in, and the manifest's.
func sortOverrideEntries(ovs []contribapi.OverrideEntry) {
	sort.SliceStable(ovs, func(i, j int) bool {
		if ovs[i].Overrides != ovs[j].Overrides {
			return ovs[i].Overrides < ovs[j].Overrides
		}
		return ovs[i].Entry.ResolvedURI() < ovs[j].Entry.ResolvedURI()
	})
}

// refusedOverride says why ov is never applied, or "": an override of a kind
// that declares `execution: executable` from an origin the universe does not
// trust.
func refusedOverride(ov contribapi.OverrideEntry, kinds contribapi.Kinds) string {
	if ov.Trusted {
		return ""
	}
	if kinds.Execution(ov.Entry.Type) == contribapi.ExecutionExecutable {
		return fmt.Sprintf("an override of executable content (%s) from %s, which the universe does not trust (projects.yaml: trusted)", ov.Overrides, ov.Origin)
	}
	return ""
}

// universeHeld are the layers of each entry the universe's override tree
// served, by URI then layer: the project's override never replaces them.
func universeHeld(overrides []contribapi.Override) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, o := range overrides {
		if o.From != contribapi.OriginUniverse {
			continue
		}
		if out[o.URI] == nil {
			out[o.URI] = map[string]bool{}
		}
		out[o.URI][o.Layer] = true
	}
	return out
}

// overrideOne applies ov over base: the layers the universe holds are kept,
// the derived meta is derived again. Nothing is recorded when ov has no layer
// left to replace.
func overrideOne(base contribapi.Contribution, ov contribapi.OverrideEntry, held map[string]bool, kinds contribapi.Kinds) (contribapi.Contribution, []contribapi.Override, error) {
	ov = ov.WithoutLayers(held)
	if len(ov.Entry.Layers) == 0 {
		return base, nil, nil
	}
	out, record, err := contribapi.ApplyOverride(base, ov)
	if err != nil {
		return base, nil, err
	}
	if err := contribapi.ApplyDerivedMeta(&out, kinds); err != nil {
		return base, nil, fmt.Errorf("%s: derive meta: %w", ov.Entry.ResolvedURI(), err)
	}
	return out, record, nil
}

// applyOverrideEntries applies, in place, each override entry whose entry
// contributions holds, and returns what it applied (the override record),
// the entries it carries for the aggregator (their entry is another member's)
// and the ones refused, as drops naming the override and why: one override
// that cannot apply never costs the manifest its other entries.
func applyOverrideEntries(contributions []contribapi.Contribution, ovs []contribapi.OverrideEntry, overrides []contribapi.Override, kinds contribapi.Kinds) (applied []contribapi.Override, carried []contribapi.OverrideEntry, refused []contribapi.Drop) {
	if len(ovs) == 0 {
		return nil, nil, nil
	}
	ovs = append([]contribapi.OverrideEntry(nil), ovs...)
	sortOverrideEntries(ovs)
	at := make(map[string]int, len(contributions))
	for i := range contributions {
		at[contributions[i].ResolvedURI()] = i
	}
	held := universeHeld(overrides)
	for _, ov := range ovs {
		if why := refusedOverride(ov, kinds); why != "" {
			refused = append(refused, contribapi.Drop{Path: ov.Entry.ResolvedURI(), Kind: ov.Entry.Type, Reason: why})
			continue
		}
		i, ok := at[ov.Overrides]
		if !ok {
			carried = append(carried, ov)
			continue
		}
		out, record, err := overrideOne(contributions[i], ov, held[ov.Overrides], kinds)
		if err != nil {
			refused = append(refused, contribapi.Drop{Path: ov.Entry.ResolvedURI(), Kind: ov.Entry.Type, Reason: err.Error()})
			continue
		}
		contributions[i] = out
		applied = append(applied, record...)
	}
	return applied, carried, refused
}

// applyCarriedOverrideEntries applies the override entries the manifests carry
// (Manifest.OverrideEntries: their entry is another member's) over the entry
// the by-URI index places, in member order, each recorded in the overridden
// manifest's Overrides; an override of an entry no member serves, or one
// refused, is a finding. A manifest changed is re-hashed. ordered is the
// membership in declared order (nil for a member without a manifest); kinds
// is asked only when some manifest carries an override entry. Both catalogs
// (CatalogImpl, MemoryCatalog) call it after building their index, so a
// reader on either serves the overridden layer. A record already present is
// not appended again: a catalog that re-indexes the same manifests applies
// an override to the same result and keeps one record of it.
func applyCarriedOverrideEntries(ordered []*mbapi.Manifest, entries map[string]entryLoc, kinds func() contribapi.Kinds) []string {
	var findings []string
	var ks contribapi.Kinds
	haveKinds := false
	changed := map[*mbapi.Manifest]bool{}
	for _, m := range ordered {
		if m == nil || len(m.OverrideEntries) == 0 {
			continue
		}
		if !haveKinds {
			ks, haveKinds = kinds(), true
		}
		for _, ov := range m.OverrideEntries {
			loc, ok := entries[ov.Overrides]
			if !ok {
				findings = append(findings, fmt.Sprintf("%s (from %s) overrides %s, which no member serves", ov.Entry.ResolvedURI(), ov.Origin, ov.Overrides))
				continue
			}
			if why := refusedOverride(ov, ks); why != "" {
				findings = append(findings, ov.Entry.ResolvedURI()+": refused: "+why)
				continue
			}
			out, record, err := overrideOne(loc.m.Contributions[loc.i], ov, universeHeld(loc.m.Overrides)[ov.Overrides], ks)
			if err != nil {
				findings = append(findings, ov.Entry.ResolvedURI()+": "+err.Error())
				continue
			}
			loc.m.Contributions[loc.i] = out
			for _, r := range record {
				if !slices.Contains(loc.m.Overrides, r) {
					loc.m.Overrides = append(loc.m.Overrides, r)
				}
			}
			changed[loc.m] = len(record) > 0 || changed[loc.m]
		}
	}
	for m, ok := range changed {
		if ok {
			ReHash(m)
		}
	}
	for _, f := range findings {
		slog.Warn("catalog: an override entry not applied", "finding", f)
	}
	return findings
}
