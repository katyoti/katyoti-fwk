// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"sort"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// Manifest is the cacheable, content-addressed descriptor for one binary
// (modules/contributions/concepts.md).
//
// Three sections:
//   - SlotTypes: typed contracts (KTSP wire surface — the CALLABLE lane)
//   - Slots: slot instances (each with a Name + Type link — CALLABLE)
//   - Contributions: the READABLE-lane envelope (skills, tasks, references,
//     data-types, configs, and any open-set kind).
//
// Consumers decode polymorphic contribution bodies via `GetContributions[T]`.
type Manifest struct {
	Provider      string                    `json:"provider"`
	Version       string                    `json:"version"`
	Description   string                    `json:"description,omitempty"`
	Hash          string                    `json:"hash,omitempty"`
	Transports    []Transport               `json:"transports,omitempty"`
	SlotTypes     []SlotType                `json:"slotTypes,omitempty"`
	Slots         []Slot                    `json:"slots,omitempty"`
	Contributions []contribapi.Contribution `json:"contributions,omitempty"`

	// Drops are the files a provider met and could not publish.
	//
	// Not an error — the descriptor is complete as far as it goes, and runtime
	// deliberately carries on: one malformed skill must not cost a universe the
	// other two hundred. But the fact has to REACH someone. Packaging is not
	// watching any log, and without this it sees a pack that is simply short of
	// the content it was built to carry.
	Drops []contribapi.Drop `json:"drops,omitempty"`

	// Overrides are the layers an override tree served instead of the
	// provider's embedded content: the attached project's
	// (`project:<token>`) or the universe's, per entry and layer. Like Drops,
	// outside the manifest's hash: where a layer came from is said, its
	// bytes are what the layer's hash already names.
	Overrides []contribapi.Override `json:"overrides,omitempty"`

	// OverrideEntries are the override entries this manifest's providers
	// hold over an entry another member serves: the builder applies those
	// whose entry it serves itself, and carries these for the aggregator to
	// apply (the catalog, which holds every member). Sorted by the entry they
	// override, then their own URI. Part of the hash: they change what the
	// universe serves.
	OverrideEntries []contribapi.OverrideEntry `json:"overrideEntries,omitempty"`

	// ForeignKinds are the kinds this manifest publishes entries of and
	// declares nowhere — neither under its own provider nor in the system
	// set it embeds. Sorted.
	//
	// Not an error at the producer: a kind is declared ONCE in a universe,
	// and the member that declares it may be another one (a files-only pack
	// declaring `recipe`, a Go producer publishing recipes). Only the
	// aggregator holds every declaration, so only it can refuse an entry
	// whose kind no member declares — and it refuses THAT ENTRY, with a
	// verdict, never a member's whole manifest. The census, which can name
	// the declared universe, stays strict: a foreign kind it cannot resolve
	// fails the build. The builder writes this; nothing else does.
	ForeignKinds []string `json:"foreignKinds,omitempty"`

	// Providers records the identity of every producer behind this manifest:
	// whether its source can change while the process runs, and the digest of
	// that source when it has one. Concepts §4 — the epoch every cache keys on.
	// Derived by the builder, never authored; sorted by (Name, Type).
	Providers []ProviderInfo `json:"providers,omitempty"`

	// ReadRoots are the folders this binary reads contributions from at run
	// time — content its executable does not carry, so a rebuild-free edit
	// there must still reach the aggregator (concepts §10, "the content root
	// is declared"). Each is named against an anchor the aggregator resolves
	// in ITS universe, never as a path: the record of a binary is shared by
	// every universe on the host, and an absolute path would be one
	// universe's. Declared by the binary (DeclareReadRoot), whether the folder
	// exists or not, so a folder created later is noticed. Sorted; not part
	// of Hash — it names where content comes from, not what it is.
	ReadRoots []ReadRoot `json:"readRoots,omitempty"`
}

// ReadRoot is one folder a binary reads contributions from: Path, relative to
// the folder its Anchor names.
type ReadRoot struct {
	Anchor string `json:"anchor"`
	Path   string `json:"path"`
}

// The anchors a ReadRoot is resolved against. A root under the universe folder
// makes the binary's record a function of the universe as well as of the
// binary; one under the global home is the same for every universe.
const (
	ReadRootUniverse = "universe" // the universe folder (application.Env().UniverseDir())
	ReadRootGlobal   = "global"   // the global home (application.Env().GlobalHomeDir())
)

// ProviderInfo is one producer's identity, as the builder recorded it.
type ProviderInfo struct {
	// Name is the provider token; Type the kind it produces.
	Name string `json:"name"`
	Type string `json:"type"`
	// Digest is an immutable provider's enumeration epoch
	// (contribapi.DigestOf): the version of the source it serves. Empty for a
	// mutable provider, and only for one — the census refuses an immutable
	// row without it.
	Digest string `json:"digest,omitempty"`
	// Mutable reports whether the source can change while the process runs.
	Mutable bool `json:"mutable,omitempty"`
}

// Transport advertises one connection mode the binary supports for NATS.
// Value-only (no methods or impls); descriptor consumers use it to know
// what they can negotiate.
type Transport struct {
	// Kind is "in-process", "tcp", "ws". Per (modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md) deploy modes.
	Kind string `json:"kind"`
	// DefaultPort, when nonzero, is the binary's preferred bind port.
	DefaultPort int `json:"defaultPort,omitempty"`
}

// SlotType is one row in the descriptor's `slot_types` section: the typed
// contract for a slot kind (e.g., the "skill" type's methods).
type SlotType struct {
	Type    string       `json:"type"`
	Version string       `json:"version,omitempty"`
	Methods []SlotMethod `json:"methods,omitempty"`
}

// SlotMethod is one method in a slot type's contract. Schemas are JSON
// Schema documents (json.RawMessage to avoid coupling).
type SlotMethod struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"inputSchema,omitempty"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
	Tags         []string        `json:"tags,omitempty"` // [cli, mcp, internal, ...]
	// TimeoutS is the per-method request timeout in seconds callers apply
	// when dispatching this method over the bus (0 = transport default).
	// See meta.MethodDef.TimeoutS.
	TimeoutS int `json:"timeoutS,omitempty"`
}

// UnmarshalJSON accepts `tags` in both wire forms: the manifest's list
// (["cli", "read-only"]) and the KTSP descriptor's map ({"read-only": "true",
// "cli.name": "x"}), which a manifest mapping flattens the same way
// (manifest/local tagsToSlice: a "" or "true" value is the bare key, any
// other value is key=value). A reader of either document gets one shape.
func (m *SlotMethod) UnmarshalJSON(b []byte) error {
	type plain SlotMethod
	var aux struct {
		plain
		Tags json.RawMessage `json:"tags,omitempty"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	*m = SlotMethod(aux.plain)
	m.Tags = nil
	if len(aux.Tags) == 0 || string(aux.Tags) == "null" {
		return nil
	}
	if err := json.Unmarshal(aux.Tags, &m.Tags); err == nil {
		return nil
	}
	var byKey map[string]string
	if err := json.Unmarshal(aux.Tags, &byKey); err != nil {
		return fmt.Errorf("slot method %s: tags are neither a list nor a map: %w", m.Name, err)
	}
	for k, v := range byKey {
		if v == "" || v == "true" {
			m.Tags = append(m.Tags, k)
		} else {
			m.Tags = append(m.Tags, k+"="+v)
		}
	}
	sort.Strings(m.Tags)
	return nil
}

// IsReadOnly reports whether the method declares that it changes nothing
// (the `read-only` tag, from meta.MethodDef.ReadOnly).
func (m SlotMethod) IsReadOnly() bool {
	for _, t := range m.Tags {
		if t == "read-only" {
			return true
		}
	}
	return false
}

// IsLong reports whether the method declares that it runs long (the `long`
// tag, from meta.MethodDef.LongRunning): the action surface runs it as a method run
// with its display panel.
func (m SlotMethod) IsLong() bool {
	for _, t := range m.Tags {
		if t == "long" {
			return true
		}
	}
	return false
}

// Slot is one slot instance the binary exposes. Multiple slots may share a
// SlotType (the named-services pattern).
type Slot struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// GetContributions[T] decodes the Body of every contribapi.Contribution with a
// matching Type into typed values.
//
// Example:
//
//	skills, _ := api.GetContributions[SkillBody](manifest, "skill")
//	for _, s := range skills { fmt.Println(s.Name) }
//
// Returns the first decode error encountered (no partial results) so
// schema drift surfaces immediately. Returns an empty slice + nil
// error when no entries match.
func GetContributions[T any](m *Manifest, t string) ([]T, error) {
	if m == nil {
		return nil, nil
	}
	var out []T
	for _, c := range m.Contributions {
		if c.Type != t {
			continue
		}
		var v T
		if len(c.Data()) > 0 {
			if err := json.Unmarshal(c.Data(), &v); err != nil {
				return nil, fmt.Errorf("manifest: decode contribution %s: %w", c.ResolvedURI(), err)
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// Attributed pairs a decoded contribution Body with its effective
// provider: the Contribution's own Provider field when set, else the
// manifest's. A manifest may carry contributions for layer providers
// other than itself (e.g. ktw-agents's manifest holds the workspace
// "universe" layer's tasks/skills) — consumers that build URIs or
// attribute entries MUST use this effective provider, never the
// manifest key alone, or the same logical entry surfaces under two
// URIs and dedupe breaks.
type Attributed[T any] struct {
	Provider string
	Body     T

	// URI is the entry's address, so a consumer holding a typed view can say
	// WHICH contribution it is looking at without reconstructing it from
	// fields the body may no longer carry.
	URI string

	// Meta is the picker-level view: name, description, icon, hashtags,
	// category, visibility.
	//
	// It is here because picker fields have ONE home and this is it. A body
	// that repeated them was a second copy that could disagree with the first,
	// and disagreeing copies of a name are how the same thing appears twice in
	// a list under two spellings. Resolved, never nil — a consumer reads
	// fields, not a pointer.
	Meta contribapi.ContributionMeta

	// Valid and ValidationErrors carry the verdict recorded when this
	// contribution entered the universe view — the aggregator holds the whole
	// graph and can check what no single binary can see.
	//
	// A consumer READS this. It never re-derives one: deriving a verdict
	// needs the DSL parser, which is why no sidecar ships it, and it would be
	// answering a question already answered.
	//
	// Valid nil means no verdict was recorded — the entry was not checked, or
	// was checked under a severity that records nothing. It does NOT mean
	// valid. A contribution that fails is marked and KEPT, so a consumer
	// decides for itself whether to use something marked invalid; something
	// visibly wrong is more useful than something silently absent.
	Valid            *bool
	ValidationErrors []string
}

// GetAttributedContributions[T] is GetContributions[T] plus effective-
// provider attribution per entry. Same decode-error semantics.
func GetAttributedContributions[T any](m *Manifest, t string) ([]Attributed[T], error) {
	if m == nil {
		return nil, nil
	}
	var out []Attributed[T]
	for _, c := range m.Contributions {
		if c.Type != t {
			continue
		}
		var v T
		if len(c.Data()) > 0 {
			if err := json.Unmarshal(c.Data(), &v); err != nil {
				return nil, fmt.Errorf("manifest: decode contribution %s: %w", c.ResolvedURI(), err)
			}
		}
		provider := c.Provider
		if provider == "" {
			provider = m.Provider
		}
		out = append(out, Attributed[T]{
			Provider: provider, Body: v,
			URI: c.ResolvedURI(), Meta: c.ResolvedMetadata(),
			Valid: c.Valid, ValidationErrors: c.ValidationErrors,
		})
	}
	return out, nil
}
