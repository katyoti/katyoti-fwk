// SPDX-License-Identifier: Apache-2.0

package contribapi

// kind.go — what a kind declares, as the code reads it.
//
// A kind is a contribution of type `contribution-type` whose data layer is this
// declaration. It is the ONE place everything governing a kind is stated: the
// schema its entries validate against, the layers they carry, the budget their
// metadata may occupy, the severity a failing entry is reported at, and whether
// its content is read or run. See modules/contributions/contribution-types.md.
//
// Decoded here rather than re-declared per contribution — the kind says it once,
// so nothing has to be repeated on every entry and nothing can disagree. The
// shipped meta-schema `system:/data-type/contribution-type` and this struct are
// held field for field by kind_decl_drift_test.go: a field added to one without
// the other is either undeclared shape or a promise nothing keeps.
//
// Until 2026-09-21 the declaration carried only name, description, schema,
// validate and execution — and of those, `validate` and `execution` had no
// reader. Residency, the default layer and the read preference were decided by
// Go code (a provider field set at one call site, a hard-coded `source`, two
// envelope methods with opposite orders), and severity by a hard-coded map that
// duplicated the kind files. Everything a consumer needs now comes from here.

// KindDeclaration is the data layer of a `contribution-type` contribution.
type KindDeclaration struct {
	// Name is the kebab discriminator written into every entry's Type.
	Name string `json:"name" jsonschema:"description=The kebab-case discriminator written into Contribution.Type"`

	// Description is the human summary surfaced in listings.
	Description string `json:"description,omitempty" jsonschema:"description=The human summary surfaced in catalog listings"`

	// Schema is the URI of the data-type the `data` layer validates against.
	// Declared, never inferred from the name: a guessed schema cannot be
	// validated when the guess is wrong, and the failure is silent.
	Schema string `json:"schema,omitempty" jsonschema:"description=URI of the data-type every contribution of this kind validates against. Declared - never inferred from the kind name: a guessed schema cannot be validated when the guess is wrong and the failure is silent."`

	// Layers declares which layers an entry of this kind carries, by name.
	// Conventional names are `data` (the structured body) and `source` (the
	// raw readable bytes). A kind that declares none is a descriptor kind:
	// its metadata is the whole resource.
	Layers map[string]LayerDecl `json:"layers,omitempty" jsonschema:"description=The layers every entry of this kind carries by name. Conventional names are data (the structured body validated against schema) and source (the raw readable bytes). A kind declaring none is a descriptor kind: its metadata is the whole resource."`

	// Meta bounds the resident metadata of every entry of this kind.
	Meta *MetaDecl `json:"meta,omitempty" jsonschema:"description=Bounds on the resident metadata of every entry of this kind"`

	// Validate is the severity a failing entry is reported at: off, warn or
	// error. Central config may override it; it never invents a default.
	Validate string `json:"validate,omitempty" jsonschema:"description=Validation severity for this kind - off or warn or error. Absent resolves to the documented default."`

	// WarnReason says why a kind is kept at `warn` rather than `error`: every
	// kind validates its entries as an error, and one that does not states
	// why, so a warn is a recorded choice and never a leftover. Check reports
	// a `warn` without a reason, and a reason on a kind that is not at `warn`.
	WarnReason string `json:"warn_reason,omitempty" jsonschema:"description=Why this kind is kept at validate warn rather than error. Required with warn and refused with any other severity: a warn is a recorded choice and never a leftover."`

	// Execution is declarative (content is READ) or executable (content RUNS,
	// or names something to run). Absent resolves to declarative.
	Execution string `json:"execution,omitempty" jsonschema:"description=How this kind content is consumed - declarative (READ) or executable (RUNS or names something to run). Absent resolves to declarative so a kind never becomes executable by accident."`

	// Translatable lists the texts of an entry's data layer a person reads,
	// which a `translations` layer may give in other languages: a field
	// (`label`), a list of strings (`hashtags`), or a field of the items of
	// a list, keyed by one of their fields (`steps[id].say`). Grammar and
	// readers in translations.go. Absent: nothing of the kind is translated.
	Translatable []string `json:"translatable,omitempty" jsonschema:"description=The texts of an entry data layer a person reads which its translations layer may give in other languages: a field (label); a list of strings (hashtags); a field of the items of a list keyed by one of their fields (steps[id].say). A kind declaring any declares the translations layer."`

	// Expressions lists the fields of an entry's data layer that hold an el
	// predicate over one object (`filter.when`): dotted paths. The census
	// compiles each against the object vocabulary (el.ObjectVocabulary), any
	// called name standing for an expression function, so a name the
	// vocabulary does not know is a finding before any screen evaluates it.
	// Absent: nothing of the kind is an expression.
	Expressions []string `json:"expressions,omitempty" jsonschema:"description=The fields of an entry data layer holding an el predicate over one object as dotted paths (filter.when) - the census compiles each against the object vocabulary so an unknown name is a finding"`

	// Source says how the files of a pack become entries of this kind: the
	// folder, the files, the id, the format and what each layer is taken
	// from (kind_source.go). Absent: the kind is produced by code of its own.
	Source *SourceDecl `json:"source,omitempty" jsonschema:"description=How the files of a pack become entries of this kind - the folder; the files; the id; the format and what each layer is taken from. Absent - the kind is produced by code of its own."`
}

// LayerDecl is what a kind says about one of its layers.
type LayerDecl struct {
	// MediaType is how the layer's bytes are rendered, e.g. text/markdown.
	// The producer may refine it per entry (a reference's file type); the
	// kind states the norm.
	MediaType string `json:"mediaType,omitempty" jsonschema:"description=How the layer bytes are rendered (text/markdown). The kind states the norm; a producer may refine it per entry."`

	// Required means every entry of the kind carries this layer. An entry
	// missing a required layer is a finding at ingest.
	Required bool `json:"required,omitempty" jsonschema:"description=Every entry of the kind carries this layer; a missing one is a finding at ingest"`

	// Default marks the layer `Open(uri, "")` resolves to, and the one the
	// read lane renders. Exactly one layer of a kind may be default; a kind
	// with layers and no default is a finding about the kind.
	Default bool `json:"default,omitempty" jsonschema:"description=The layer Open(uri) with no layer resolves to and the read lane renders. Exactly one per kind."`

	// Resident means the layer's bytes are worth carrying in the index when
	// the size band allows. This is the kind answering "does a LISTING need
	// this content?" — a data-type says yes (its DSL is what every consumer
	// registers), a skill says no (its markdown is what a reader renders).
	// The band is a ceiling the declaration cannot raise.
	Resident bool `json:"resident,omitempty" jsonschema:"description=The layer bytes are worth carrying in the index when the size band allows: the kind answering whether a listing needs this content. The band is a ceiling the declaration cannot raise."`
}

// MetaDecl bounds the resident metadata of a kind, and says where it comes
// from.
type MetaDecl struct {
	// Budget is the maximum size, in bytes, of an entry's serialised
	// metadata. Metadata is the one part of a contribution multiplied by the
	// size of the catalog, so it is bounded per kind.
	Budget int `json:"budget,omitempty" jsonschema:"description=Maximum size in bytes of an entry serialised metadata. Metadata is the one part multiplied by the size of the catalog so it is bounded per kind."`
	// From names the layer the framework DERIVES this kind's picker fields
	// from — `data`, the one structured layer, or the `source` of a kind kept
	// as written in YAML (a data-type's DSL). When set, the producer writes
	// the fields once, in that layer, and the manifest builder fills meta from
	// it; a producer that also wrote meta would be a second writer of the
	// same name, which is what let a skill appear twice in a listing (c22).
	// Absent means the producer writes meta itself, as every kind did before
	// 2026-09-22.
	From string `json:"from,omitempty" jsonschema:"description=The layer the framework derives this kind picker fields from (data). When set the producer writes those fields once in that layer and the manifest builder fills meta from it - one writer. Absent - the producer writes meta itself."`
	// Fields lists the meta fields derived, by META field name (see
	// DerivableMetaFields). Each names the data field(s) it is read from by
	// the envelope's own convention: `icon` reads `iconName` and `icon`.
	Fields []string `json:"fields,omitempty" jsonschema:"description=The meta fields derived by meta field name: name; description; icon; hashtags; category; advertised. Each reads its data field(s) by the envelope convention (icon reads iconName and icon)."`
}

// DerivableMetaFields are the picker fields of ContributionMeta a kind may
// declare as derived from its data layer, with the data field(s) each reads.
// The list is the envelope's, not the kind's: a kind chooses among these and
// cannot invent a meta field.
var DerivableMetaFields = map[string][]string{
	"name":        {"name"},
	"description": {"description"},
	"icon":        {"iconName", "icon"},
	"hashtags":    {"hashtags"},
	"category":    {"category"},
	"advertised":  {"advertised"},
}

// MetaFrom returns the layer this kind's meta is derived from and the meta
// fields derived — ("", nil) for a kind whose producer writes meta itself.
func (d KindDeclaration) MetaFrom() (layer string, fields []string) {
	if d.Meta == nil || d.Meta.From == "" {
		return "", nil
	}
	return d.Meta.From, d.Meta.Fields
}

// ExecutionClass returns how this kind's content is consumed.
func (d KindDeclaration) ExecutionClass() Execution {
	return ExecutionOf(d.Execution)
}

// DefaultLayer returns the layer `Open(uri, "")` resolves to for this kind.
//
// The declared default wins. A kind that declares layers but marks none is
// answered by convention — `source` if declared, else `data` — so that a
// half-written declaration still opens SOMETHING while Check reports the
// omission. A kind with no layers has no default: its metadata is the resource.
func (d KindDeclaration) DefaultLayer() string {
	for name, l := range d.Layers {
		if l.Default {
			return name
		}
	}
	if _, ok := d.Layers[LayerSource]; ok {
		return LayerSource
	}
	if _, ok := d.Layers[LayerData]; ok {
		return LayerData
	}
	return ""
}

// IsResident reports whether this kind wants the named layer carried in the
// index. False for an undeclared layer: nothing is resident by accident.
func (d KindDeclaration) IsResident(layer string) bool {
	return d.Layers[layer].Resident
}

// Budget returns the metadata budget in bytes, or 0 when the kind declares
// none — which Check reports, because an unbounded kind is a listing that can
// grow without anyone having decided it may.
func (d KindDeclaration) Budget() int {
	if d.Meta == nil {
		return 0
	}
	return d.Meta.Budget
}

// Severity returns the declared severity, normalised: off, warn or on.
// Empty resolves to warn — visible, never blocking, never silent.
func (d KindDeclaration) Severity() string {
	return normalizeSeverity(d.Validate)
}
