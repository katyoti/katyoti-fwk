// SPDX-License-Identifier: Apache-2.0

package api

import "context"

// Inspector shows ONE contribution as the model holds it: the entry, the kind
// that governs it, its layers, its producer and its verdict. It is the CLI half
// of "humans can inspect the full model" (the GUI Resources pane is the other),
// for ANY kind: every question about the kind is answered by the kind
// declarations the source carries (contribapi.Kinds), never by code that knows
// a kind by name.
//
// Not a ComposeService method: the verb renders YAML by default, which the
// generated flat CLI cannot, so `ktw-compose inspect` is a hand-written command
// over this one body (compose/local).
type Inspector interface {
	Inspect(ctx context.Context, in InspectInput) (InspectOutput, error)
}

// InspectInput names one entry and where to read it: a compiled sidecar
// (through its own `sc resources manifest`, in a throwaway workspace — the same
// read the lint verbs use) or the universe this binary belongs to (the
// aggregate, at universe reach).
type InspectInput struct {
	URI      string `json:"uri"`
	Exe      string `json:"exe,omitempty"`
	Universe bool   `json:"universe,omitempty"`
}

// InspectOutput is the one document the verb prints.
type InspectOutput struct {
	URI string `json:"uri" yaml:"uri"`
	// Source says what answered: `exe:<path>` or `universe`.
	Source     string          `json:"source" yaml:"source"`
	Provider   InspectProvider `json:"provider" yaml:"provider"`
	Kind       InspectKind     `json:"kind" yaml:"kind"`
	Meta       map[string]any  `json:"meta,omitempty" yaml:"meta,omitempty"`
	Layers     []InspectLayer  `json:"layers" yaml:"layers"`
	Provenance *InspectProv    `json:"provenance,omitempty" yaml:"provenance,omitempty"`
	Verdict    InspectVerdict  `json:"verdict" yaml:"verdict"`
}

// InspectProvider is the producer of the entry and its identity rows, as the
// manifest recorded them (Manifest.Providers, matched on name and kind).
type InspectProvider struct {
	Name     string            `json:"name" yaml:"name"`
	Identity []InspectIdentity `json:"identity,omitempty" yaml:"identity,omitempty"`
	// Note explains an empty identity (the meta-model has no manifest of its
	// own; a view that cannot name a provider's manifest says so).
	Note string `json:"note,omitempty" yaml:"note,omitempty"`
}

// InspectIdentity is one ProviderInfo row.
//
// Digest is an immutable provider's enumeration epoch; a mutable one has none.
type InspectIdentity struct {
	Type    string `json:"type" yaml:"type"`
	Digest  string `json:"digest,omitempty" yaml:"digest,omitempty"`
	Mutable bool   `json:"mutable" yaml:"mutable"`
}

// InspectKind is what the kind registry answers about the entry's kind.
type InspectKind struct {
	Name         string             `json:"name" yaml:"name"`
	Declared     bool               `json:"declared" yaml:"declared"`
	DeclaredBy   string             `json:"declaredBy,omitempty" yaml:"declaredBy,omitempty"`
	Schema       string             `json:"schema,omitempty" yaml:"schema,omitempty"`
	DefaultLayer string             `json:"defaultLayer,omitempty" yaml:"defaultLayer,omitempty"`
	Layers       []InspectKindLayer `json:"layers,omitempty" yaml:"layers,omitempty"`
	Budget       int                `json:"budget,omitempty" yaml:"budget,omitempty"`
	Validate     string             `json:"validate,omitempty" yaml:"validate,omitempty"`
	Execution    string             `json:"execution" yaml:"execution"`
	Findings     []string           `json:"findings,omitempty" yaml:"findings,omitempty"`
}

// InspectKindLayer is one layer the kind declares.
type InspectKindLayer struct {
	Name      string `json:"name" yaml:"name"`
	MediaType string `json:"mediaType,omitempty" yaml:"mediaType,omitempty"`
	Required  bool   `json:"required" yaml:"required"`
	Default   bool   `json:"default" yaml:"default"`
	Resident  bool   `json:"resident" yaml:"resident"`
}

// InspectLayer is one layer of the entry: declared by the kind, carried by the
// entry, or both — a gap on either side is visible as such.
type InspectLayer struct {
	Name      string `json:"name" yaml:"name"`
	Declared  bool   `json:"declared" yaml:"declared"`
	Carried   bool   `json:"carried" yaml:"carried"`
	Hash      string `json:"hash,omitempty" yaml:"hash,omitempty"`
	Size      int64  `json:"size,omitempty" yaml:"size,omitempty"`
	Band      string `json:"band,omitempty" yaml:"band,omitempty"`
	MediaType string `json:"mediaType,omitempty" yaml:"mediaType,omitempty"`
	// Inline reports whether the layer's bytes travel in the index.
	Inline bool `json:"inline" yaml:"inline"`
}

// InspectProv is the provenance assertion, when the entry carries one, with the
// identity digest it is over.
type InspectProv struct {
	Signed         bool   `json:"signed" yaml:"signed"`
	IdentityDigest string `json:"identityDigest" yaml:"identityDigest"`
	KeyID          string `json:"keyId,omitempty" yaml:"keyId,omitempty"`
	Publisher      string `json:"publisher,omitempty" yaml:"publisher,omitempty"`
	Signature      string `json:"signature,omitempty" yaml:"signature,omitempty"`
}

// InspectVerdict is the validation verdict as recorded — read, never
// re-derived. State is valid, invalid or unchecked.
type InspectVerdict struct {
	State  string   `json:"state" yaml:"state"`
	Valid  *bool    `json:"valid,omitempty" yaml:"valid,omitempty"`
	Errors []string `json:"errors,omitempty" yaml:"errors,omitempty"`
	Note   string   `json:"note,omitempty" yaml:"note,omitempty"`
}
