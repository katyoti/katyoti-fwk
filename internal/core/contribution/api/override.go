// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"fmt"
	"sort"
	"sync"
)

// Override is one layer of an entry that is not the producer's own: served
// from an override tree laid over the producer's embedded content — the
// attached project's (`project:<token>`) or the universe's (`universe`).
// Carried in the manifest beside Drops, so what a universe serves says where
// it came from.
type Override struct {
	// URI is the overridden entry's.
	URI string `json:"uri"`
	// Layer is the layer the override serves (source, translations, …).
	Layer string `json:"layer"`
	// From is the override tree: `project:<token>` or `universe`.
	From string `json:"from"`
}

// OverrideReporter is implemented by a provider that can say which of its
// layers an override tree served. Optional, like DropReporter.
type OverrideReporter interface {
	// Overrides returns what the most recent Enumerate took from an override.
	Overrides() []Override
}

// OverrideLog accumulates overrides for a provider. Embed it and call
// RecordOverride; reset at the start of each Enumerate.
type OverrideLog struct {
	mu        sync.Mutex
	overrides []Override
}

// RecordOverride notes one overridden layer.
func (l *OverrideLog) RecordOverride(o Override) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.overrides = append(l.overrides, o)
}

// ResetOverrides forgets the previous enumeration's overrides.
func (l *OverrideLog) ResetOverrides() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.overrides = nil
}

// Overrides implements OverrideReporter.
func (l *OverrideLog) Overrides() []Override {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Override(nil), l.overrides...)
}

// OriginUniverse is the universe's override tree, as Override.From names it.
const OriginUniverse = "universe"

// ProjectOrigin is a project's override tree, as Override.From names it:
// project:<its provider token>.
func ProjectOrigin(token string) string { return "project:" + token }

// OverrideEntry is an entry that overrides another provider's entry (decision
// ): it carries only the layers it replaces
// and inherits the others. Its producer (the object-backed provider of a
// project) reports it apart from its entries (OverrideEntryReporter); the
// manifest builder applies it over the entry it names, or carries it in the
// manifest for the aggregator when another member serves that entry.
type OverrideEntry struct {
	// Overrides is the URI of the entry overridden (provider:/kind/id).
	Overrides string `json:"overrides"`
	// Entry is the override's own entry: its URI under its producer's token
	// and the layers it replaces.
	Entry Contribution `json:"entry"`
	// Origin is where it comes from, as Override.From names it
	// (`project:<token>`).
	Origin string `json:"origin"`
	// Object is the object that publishes it, when one does.
	Object string `json:"object,omitempty"`
	// Trusted says the universe trusts its origin: an override of a kind
	// that declares `execution: executable` is applied only then.
	Trusted bool `json:"trusted,omitempty"`
}

// OverrideEntryReporter is implemented by a provider that holds override
// entries. Optional, like OverrideReporter: a provider that implements it
// never serves an override entry from Enumerate.
type OverrideEntryReporter interface {
	OverrideEntries() []OverrideEntry
}

// ApplyOverride serves base with the layers ov replaces: the result keeps
// base's identity and every layer ov does not carry (the unit is the whole
// layer, never a field), and the override record names each replaced layer
// with ov's origin. base is not changed: its metadata is copied, and the
// caller derives the derived fields again from the new layers
// (ApplyDerivedMeta); the validation verdict is the catalog's to give again.
func ApplyOverride(base Contribution, ov OverrideEntry) (Contribution, []Override, error) {
	uri := ov.Entry.ResolvedURI()
	if ov.Overrides == "" {
		return base, nil, fmt.Errorf("%s overrides nothing", uri)
	}
	if ov.Overrides != base.ResolvedURI() {
		return base, nil, fmt.Errorf("%s overrides %s, not %s", uri, ov.Overrides, base.ResolvedURI())
	}
	if ov.Entry.Type != base.Type {
		return base, nil, fmt.Errorf("%s is of kind %s, not %s", uri, ov.Entry.Type, base.Type)
	}
	// An override layer is served from the index, never from a producer: the
	// URI it is opened by is the base entry's, so a lane would reach the base's
	// producer and its bytes. A layer that does not carry its bytes inline (the
	// bulk band, or one stripped on the way) is refused, whole: half an
	// override is never applied.
	for _, name := range sortedLayerNames(ov.Entry.Layers) {
		if l := ov.Entry.Layers[name]; len(l.Inline) == 0 && l.Size > 0 {
			return base, nil, fmt.Errorf("%s layer %q is %d bytes (%s band) and does not carry its bytes: an override layer is served from the index, up to the bulk band", uri, name, l.Size, l.Band())
		}
	}
	out := base
	out.Layers = make(map[string]LayerRef, len(base.Layers)+len(ov.Entry.Layers))
	for name, l := range base.Layers {
		out.Layers[name] = l
	}
	names := sortedLayerNames(ov.Entry.Layers)
	record := make([]Override, 0, len(names))
	for _, name := range names {
		out.Layers[name] = ov.Entry.Layers[name]
		record = append(record, Override{URI: base.ResolvedURI(), Layer: name, From: ov.Origin})
	}
	if len(names) > 0 {
		if base.Metadata != nil {
			meta := *base.Metadata
			out.Metadata = &meta
		}
		out.Valid, out.ValidationErrors = nil, nil
	}
	return out, record, nil
}

// sortedLayerNames are the names of layers, in order.
func sortedLayerNames(layers map[string]LayerRef) []string {
	names := make([]string, 0, len(layers))
	for name := range layers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// WithoutLayers is ov without the layers keep names: what it may still
// replace when another origin holds the last word on those.
func (ov OverrideEntry) WithoutLayers(keep map[string]bool) OverrideEntry {
	if len(keep) == 0 {
		return ov
	}
	layers := make(map[string]LayerRef, len(ov.Entry.Layers))
	for name, l := range ov.Entry.Layers {
		if !keep[name] {
			layers[name] = l
		}
	}
	ov.Entry.Layers = layers
	return ov
}
