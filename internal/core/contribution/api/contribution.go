// SPDX-License-Identifier: Apache-2.0

// Contribution is the envelope for one readable resource in the universe.
//
// Model: modules/contributions/concepts.md. Everything addressable is a
// contribution — identity + meta + layers — and its kind (a contribution too)
// declares the contract every entry obeys: schema, layers, budget, severity,
// execution class. See kind.go and kinds.go for the declaration and the
// registry that reads it.
//
// Three lanes read an entry, and each answers one question:
//
//   - the INDEX: Query — envelopes, body-free above the inline band;
//   - the BODY lane: GetBody — the `data` layer, and nothing else;
//   - the CONTENT lane: Open(uri, layer) — any named layer, at any band, at
//     either reach. The kind names the default layer.
//
// There is no preference baked into this type: Data() and Source() answer
// their own layer only. Two accessors with opposite fallbacks used to live
// here, and the second is how YAML reached a JSON decoder.
//
// Key invariants:
//
//   - Two-lane model: Contributions are READABLE; KTSP slot methods are
//     CALLABLE. There is no callable Contribution kind.
//   - Outgoing references are DERIVED at ingest and held by the aggregator as
//     an index, not carried on the envelope.
//   - A kind IS a contribution, so it is versioned by its own hash. There is
//     no separate schema-version integer.
//   - Metadata is split from the layers. Picker-level info (name, description,
//     hashtags, ...) lives in ContributionMeta — cheap to list without
//     materialising any layer, and budgeted by the kind.

package contribapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

// Contribution is the universal readable-resource envelope.
//
// Field population responsibility:
//
//	Provider   producer
//	Type       producer
//	ID         producer
//	URI        producer MAY set; catalog re-derives + asserts equality at ingest
//	Metadata   producer (catalog may Summarize from the data layer if absent)
//	Layers     producer (`data` is the structured content, typed by the kind)
type Contribution struct {
	// === Identity ===

	// Provider is the binary that owns this Contribution (e.g., "ktw-docs").
	// Set by the producer; immutable across the Contribution's lifetime.
	Provider string `json:"provider"`

	// Type is the open-set kind discriminator (e.g., "skill", "task",
	// "reference", "config", "data-type", "model-card"). Lowercase +
	// hyphen, â‰¤63 chars.
	Type string `json:"type"`

	// ID is the item path within (Provider, Type). Forms the URI as
	// `<provider>:/<type>/<id>`. May contain forward slashes for
	// folder-structured kinds (e.g., "diagrams/architecture.md" for a
	// reference whose provider is "ktw-docs").
	ID string `json:"id"`

	// URI is the canonical identifier `<provider>:/<type>/<id>`.
	// Derived from Provider/Type/ID; producers MAY emit it for
	// cross-language consumers that don't want to compose it themselves.
	// The catalog re-derives and asserts equality at ingest — any
	// mismatch produces a lint warning.
	URI string `json:"uri,omitempty"`

	// === Picker-level structured metadata (separated from Body) ===

	// Metadata carries the picker-level fields a UI needs to render the
	// Contribution without loading the (potentially large) Body. Split
	// out so an index listing (`ResourceService.List`, `mbapi.Query`) is cheap. Producers populate; the catalog
	// may call Strategy.Summarize(Body) to derive Metadata if absent.
	//
	// Pointer-to-struct so canonical-JSON-stable hashing distinguishes
	// "absent metadata" from "empty metadata". Consumers should call
	// `ResolvedMetadata()` to get a non-nil value with zero defaults.
	Metadata *ContributionMeta `json:"metadata,omitempty"`

	// === Layers ===

	// Layers are the contribution's named, content-addressed content.
	//
	// A layer is bytes plus a media type, addressed by hash. The conventional
	// names are `data` (the structured body, validated against the kind's
	// schema) and `source` (the raw readable bytes a consumer renders —
	// markdown, DSL text, a file). A kind declares which layers it has.
	//
	// Layers exist so that content and index are separable. Before them the
	// readable body had to be inlined into the index — one kind carried ~7 KB of
	// markdown per entry, which is most of why a single pack's manifest reached
	// 287 KB — and content larger than a message payload could not be
	// represented at all.
	//
	// A layer's TRANSPORT follows from its size, not from a declaration: small
	// layers travel inline, larger ones are fetched on demand, and beyond that a
	// locator is carried instead of the bytes. Consumers call Open and never
	// branch on which band applies.
	Layers map[string]LayerRef `json:"layers,omitempty"`

	// === Graph ===

	// Provenance is an OPTIONAL assertion of origin, over this contribution's
	// identity rather than its content — see provenance.go. Produced where
	// authorship becomes distribution (packaging, sealing); absent for content
	// an author serves from their own folder, which is not a distribution
	// event and needs no signature.
	//
	// Nil is normal. An unsigned contribution is valid and usable.
	Provenance *Provenance `json:"provenance,omitempty"`

	// === Validation status (populated by the catalog at ingest) ===

	// Valid carries the schema-validation result, populated at reindex
	// alongside Edges. nil means "not validated" (validation off, or no
	// declared Schema under a non-strict severity). *true means the Body
	// passed its data-type schema. *false means it FAILED under an
	// error/on severity — the contribution stays in the catalog, visibly
	// marked, never dropped (kept-but-flagged).
	//
	// Pointer-to-bool so canonical JSON distinguishes "not validated"
	// from "validated and passed".
	Valid *bool `json:"valid,omitempty"`

	// ValidationErrors holds the human-readable schema diagnostics when
	// validation found a problem (under any non-off severity — set even
	// for warn, where Valid stays unset). Plain strings (not Diagnostic
	// structs) so this package takes no schema/api dependency.
	ValidationErrors []string `json:"validationErrors,omitempty"`
}

// ResolvedURI returns the canonical URI `<provider>:/<type>/<id>` even
// when the URI field is unset. Use this when consuming Contribution
// values from the wire — the URI field is optional, but the identity
// is always derivable.
func (c Contribution) ResolvedURI() string {
	if c.URI != "" {
		return c.URI
	}
	return c.Provider + ":/" + c.Type + "/" + c.ID
}

// TypedURI returns the structured ResourceURI for this Contribution.
// Use this (over ResolvedURI) when you need to pass the URI to
// ParseURI / catalog filters — anywhere a structured value beats a
// string. The fields are taken verbatim; no charset re-validation.
func (c Contribution) TypedURI() ResourceURI {
	return ResourceURI{Provider: c.Provider, Type: c.Type, ID: c.ID}
}

// ResolvedMetadata returns Metadata if set, or a zero-value
// ContributionMeta otherwise. Convenience for consumers that don't
// want to nil-check.
func (c Contribution) ResolvedMetadata() ContributionMeta {
	var m ContributionMeta
	if c.Metadata != nil {
		m = *c.Metadata
	}
	// Every contribution carries a non-empty human label. When a producer
	// didn't set one, derive it from the contribution's identity so the
	// field is never empty — consumers (pickers, the GUI Resources view)
	// always have something to render. Producers SHOULD set Metadata.Name
	// for a richer label; this is the floor, not a replacement.
	if m.Name == "" {
		m.Name = c.defaultName()
	}
	return m
}

// defaultName derives a label from the contribution's identity: the last
// '/'-segment of the ID (a reference's path → its filename; a contribution-type
// → its type token), falling back to Type then Provider when the ID is empty.
func (c Contribution) defaultName() string {
	if c.ID != "" {
		if i := strings.LastIndexByte(c.ID, '/'); i >= 0 && i < len(c.ID)-1 {
			return c.ID[i+1:]
		}
		return c.ID
	}
	if c.Type != "" {
		return c.Type
	}
	return c.Provider
}

// ContributionMeta is the typed picker-level view of a Contribution.
// Designed to be small (< 1 KB typical) so an index listing
// (`ResourceService.List`, `mbapi.Query`) returns cheaply over the wire.
type ContributionMeta struct {
	// Name is the human-readable label. SHOULD match a single line.
	Name string `json:"name"`

	// Description is a brief explanation. Markdown-format; SHOULD fit
	// in â‰¤ 1024 chars (consumers may truncate).
	Description string `json:"description,omitempty"`

	// Icon is the optional dual rendering hint (Tabler icon for GUI,
	// emoji for terminal). Pointer-to-struct so an empty Icon block
	// truly omits from canonical JSON.
	Icon *ContributionIcon `json:"icon,omitempty"`

	// Hashtags are tag-style filter facets. Lowercase.
	Hashtags []string `json:"hashtags,omitempty"`

	// Category is the primary grouping (e.g., "documents", "policy",
	// "agent-tools").
	Category string `json:"category,omitempty"`

	// MediaType is the MIME type of the contribution's READABLE content
	// — what ResourceService.Read returns for this URI (e.g.
	// "text/markdown" for a skill/task/persona, "application/yaml" for a
	// data-type, "application/json" for a structured descriptor,
	// "application/pdf" for a binary reference). Lets index consumers
	// (GUI Resources view) pick a renderer without fetching the body.
	MediaType string `json:"mediaType,omitempty"`

	// Advertised controls picker visibility. Default true; explicit
	// false hides this Contribution from `<binary> contributions list`
	// + the agent prompt builder's `<skills_system>` block + GUI
	// pickers. Still reachable by URI for consumers that know it.
	//
	// Pointer-to-bool to encode three states (default-true / explicit-true
	// / explicit-false) — matches the existing skill-frontmatter pattern.
	Advertised *bool `json:"advertised,omitempty"`

	// Annotations are free-form key/value tags. Use sparingly — prefer
	// first-class fields. Useful for vendor-specific markup that
	// doesn't deserve a canonical field.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// IsAdvertised resolves the pointer-to-bool default of Advertised.
// nil or *true â†’ true; *false â†’ false.
func (m ContributionMeta) IsAdvertised() bool {
	if m.Advertised == nil {
		return true
	}
	return *m.Advertised
}

// ContributionIcon carries the dual rendering hint. Both fields are
// optional; consumers fall back as appropriate.
type ContributionIcon struct {
	// Tabler is the Tabler-icon name (no leading "tabler-"). Used by
	// ktw-display + other GUI consumers.
	Tabler string `json:"tabler,omitempty"`

	// Emoji is the terminal-friendly emoji rendering. Used by CLI
	// consumers + the agent's prompt block.
	Emoji string `json:"emoji,omitempty"`
}

// NewIcon builds a *ContributionIcon from a Tabler-icon name and an
// emoji, returning nil when both are empty so the Icon block truly
// omits from canonical JSON. Providers use this when promoting body
// frontmatter (IconName → Tabler, Icon/Emoji → Emoji) into the index
// ContributionMeta.
func NewIcon(tabler, emoji string) *ContributionIcon {
	if tabler == "" && emoji == "" {
		return nil
	}
	return &ContributionIcon{Tabler: tabler, Emoji: emoji}
}

// LayerRef describes one layer without carrying it: the hash is the layer's
// identity, the size decides how it travels, and the media type tells a consumer
// how to render it.
//
// Hash rather than bytes is what lets an index stay bounded by the NUMBER of
// contributions rather than their size, and what lets two providers shipping
// identical content deduplicate.
type LayerRef struct {
	// Hash is the content address: "sha256:" + hex(sha256(content)).
	Hash string `json:"hash" yaml:"hash"`

	// Size in bytes. Decides the transport band, and lets a consumer choose
	// whether to fetch before it does.
	Size int64 `json:"size" yaml:"size"`

	// MediaType is how to render this layer's content. The ONE place a media
	// type is declared — previously it was derived independently in three.
	//
	// The yaml tags on this struct are not decoration: a bulk layer's
	// descriptor is RECORDED in a pack (see layer_manifest.go), so these
	// field names are a wire format an installed pack is read back through.
	MediaType string `json:"mediaType,omitempty" yaml:"mediaType,omitempty"`

	// Inline carries the content itself, for the inline band only.
	//
	// The band table says content at this size is "carried in the index", and
	// this is where it is carried. Without it the inline band was a row in a
	// document with nothing behind it: `BandOf` named it, nothing acted on it,
	// and the envelope's `Body` field played the part under a different name —
	// which is why a contribution had two ways to hold content that differed
	// only in which one a consumer remembered to look at.
	//
	// Populated ONLY when Size is within the inline band. A larger layer
	// carries its hash and nothing else, so an index stays bounded by the
	// NUMBER of contributions rather than by their content.
	Inline []byte `json:"inline,omitempty" yaml:"inline,omitempty"`
}

// Conventional layer names. A kind may declare others; these two carry the
// meanings every generic consumer understands.
const (
	// LayerData is the structured body, validated against the kind's schema.
	LayerData = "data"
	// LayerSource is the raw readable content a consumer renders.
	LayerSource = "source"
	// LayerInstructions is the markdown body of a frontmatter kind (a skill),
	// after the fence: what a reader opens. Declared by the kind that carries
	// it; the file as written stays `source`, the frontmatter `data`.
	LayerInstructions = "instructions"
	// LayerWidget is a display engine's widget page (one self-contained HTML
	// page), which the GUI loads in a sandboxed iframe. Declared by the
	// `display-engine` kind; read by the GUI's DisplayEngineWidget.
	LayerWidget = "widget"
	// LayerTranslations holds an entry's texts in other languages, the source
	// language staying in the entry itself: the `<name>.i18n.yaml` beside the
	// entry (language code → texts, see translations.go). Declared by every
	// kind that declares `translatable` texts; read by the hub's translation
	// bundle, the GUI's guide player and the content lint.
	LayerTranslations = "translations"
)

// ContributionEdge is one outgoing reference from a Contribution.
// Populated by the catalog's edge extractor at ingest.
type ContributionEdge struct {
	// Relation is the edge label, declared by the data-type's edge
	// annotation (e.g., "requires", "extends", "provides", "validates",
	// "documented-by"). Open set; lowercase + hyphen.
	Relation string `json:"relation"`

	// Target is the URI of the referenced Contribution.
	Target string `json:"target"`

	// Source is the dotted body-field path that produced this edge
	// (e.g., "requires[2]", "spec.installs.engine"). Useful for
	// debugging dangling-reference lint findings; not load-bearing for
	// graph traversal.
	Source string `json:"source,omitempty"`
}

// LintFinding is one finding from a cross-contribution check (packaging,
// the census, the aggregator's edge extraction). Severity is
// "error" / "warning" / "info"; Code is a stable identifier (e.g.,
// "DANGLING_URI"); Message is human-readable; Path locates the
// problematic field (dotted body-field path or edge index).
type LintFinding struct {
	Severity string `json:"severity"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

// ContributionsChangedEvent is the payload published on
// `ktw.<tenant>.universe.<u>.event.contributions-changed` (see
// subjects.ContributionsChangedSubject) whenever the catalog's loaded
// Contributions set changes — initial readiness, refresh after a
// sidecar produces a new manifest cache file, or single-provider
// reload.
//
// Subscribers (agent prompt builder, GUI catalog viewer, MCP bridge)
// use this to refresh their materialised view without polling.
// Subscribers are expected to apply their own debounce (50-100 ms
// typical) — the publisher emits one event per change without
// coalescing.
type ContributionsChangedEvent struct {
	// Provider is the binary whose Contributions changed. Empty when
	// the change is universe-wide (e.g. MarkReady firing across every
	// loaded manifest at boot).
	Provider string `json:"provider,omitempty"`

	// KindsChanged lists the Contribution types touched by this change
	// (e.g. ["skill", "task"]). Special value ["*"] means "any kind may
	// have changed; subscribers must refresh broadly." Lets subscribers
	// filter selectively (e.g. the agent prompt builder only cares about
	// "skill" changes).
	KindsChanged []string `json:"kindsChanged,omitempty"`

	// ChangedAt is the wall-clock time the change was observed. UTC.
	ChangedAt time.Time `json:"changedAt"`
}

// ContributionsEventPublisher is the minimal publisher surface the
// catalog needs to emit ContributionsChangedEvent. Kept as a tiny
// interface so the catalog package doesn't take a hard transport
// dependency (NATS, in-process EventBus, no-op stub — all satisfy).
type ContributionsEventPublisher interface {
	// PublishContributionsChanged publishes the event. Implementations
	// are responsible for any payload serialisation (JSON) and transport
	// (NATS, in-process). Errors are best-effort — the catalog logs and
	// continues; a broken bus must NOT stop catalog operations.
	PublishContributionsChanged(ev ContributionsChangedEvent) error
}

// HashContent returns a layer's content address: "sha256:" + hex(sha256(b)).
//
// One function so every producer addresses content identically — two providers
// shipping the same bytes must produce the same hash, or deduplication and
// cache-hit detection both break.
func HashContent(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// === Size bands ===
//
// A layer's transport follows from its SIZE, not from a declaration. Producers
// say what layers exist; the band says how each one travels. A consumer calls
// one operation and never branches on which band applies.
//
// The boundaries are chosen from what each transport can actually carry, not
// from taste: the control plane's message payload limit is 1 MiB, and a body
// travelling it is base64-framed inside a JSON envelope, which inflates it by
// about a third. BandControl leaves room for that with margin.
const (
	// BandInline is the ceiling for content carried IN the index. Above it,
	// inlining would make the index grow with content size rather than with
	// the number of contributions, which is the thing the layer model exists
	// to prevent.
	BandInline = 4 << 10 // 4 KiB

	// BandControl is the ceiling for content fetched on demand over the
	// control plane, one hash at a time. Above it, content moves out of band.
	BandControl = 256 << 10 // 256 KiB
)

// Band names how a layer of this size travels.
type Band string

const (
	// BandNameInline — carried in the index itself.
	BandNameInline Band = "inline"
	// BandNameControl — fetched on demand over the control plane, cached by hash.
	BandNameControl Band = "control"
	// BandNameBulk — a locator is carried instead of the bytes; the content is
	// fetched out of band from the object store, range-capable and resumable.
	BandNameBulk Band = "bulk"
)

// BandOf returns the band a layer of this size belongs to.
//
// Deriving it means two components never disagree about how a given layer
// travels: the size is in the index, so producer and consumer compute the same
// answer without coordinating.
func BandOf(size int64) Band {
	switch {
	case size <= BandInline:
		return BandNameInline
	case size <= BandControl:
		return BandNameControl
	default:
		return BandNameBulk
	}
}

// IsBulk reports whether this band travels out of band. The one question a
// caller asks of a size before deciding whether the content can be held.
func (b Band) IsBulk() bool { return b == BandNameBulk }

// Band returns the band this layer belongs to.
func (l LayerRef) Band() Band { return BandOf(l.Size) }

// IsBulk reports whether this layer must be fetched out of band. The one check
// a caller needs before deciding whether content can be held in memory.
func (l LayerRef) IsBulk() bool { return l.Band() == BandNameBulk }

// === The data layer ===
//
// A contribution's structured content is its `data` layer. Small enough to be
// inline — which it nearly always is, since a body a consumer decodes is a
// description rather than a document — it is resident in the index and costs
// nothing to list. That residency is the whole reason listing a kind fetches
// nothing per entry, and it is what these accessors preserve.

// Data returns this contribution's structured content, or nil.
//
// Resident content only. A `data` layer past the inline band is fetched like
// any other layer, through Open — returning nil here rather than silently
// reaching for the network is what keeps "listing costs nothing" true.
func (c Contribution) Data() json.RawMessage {
	l, ok := c.Layers[LayerData]
	if !ok {
		return nil
	}
	return l.Inline
}

// DataHash returns the content address of the structured content, or "".
//
// Present whether or not the content is resident: a layer past the inline band
// carries its hash and no bytes, and the hash is what a consumer uses to look
// it up, dedupe it, or decide it already has it.
func (c Contribution) DataHash() string { return c.Layers[LayerData].Hash }

// DropInline reduces this contribution to what the index carries.
//
// **This is where the band table is applied.** A layer in the inline band KEEPS
// its bytes — "carried in the index" is what that band means, and dropping them
// would leave the row a name with no content anywhere. Everything larger keeps
// its descriptor and loses its bytes, so listing a universe moves nothing that
// grows with the size of what is in it.
//
// Dropping the LAYER instead would be a different and much worse thing: the
// entry would stop saying the content exists, and a consumer would read
// "nothing here" where the truth is "not carried right now".
//
// # Copy-on-write, and why it has to be
//
// Every caller reaches this the same way — range a cached slice by value, drop
// on the copy, hand the copy out. `Layers` is a MAP, so a struct copy shares
// it, and an in-place write would reach back into the cache the copy existed to
// protect. All four call sites believed they were safe; the hub's own comment
// said "mutating in place would corrupt the cache, so we copy each entry
// before nilling Body", and the copy did not do that.
//
// What it cost: listing the index dropped the bytes out of the LIVE catalog, so
// every later GetBody for a contribution above the inline band returned empty —
// a read of the index destroying the content plane, on a path with no
// in-process guard over it.
//
// Rebinding a fresh map instead of writing into the shared one makes that
// impossible to reintroduce, rather than leaving it to each call site to
// remember. The allocation happens only when something is actually dropped, so
// the common index-plane case where everything is inline stays free.
func (c *Contribution) DropInline() {
	var dropped map[string]LayerRef
	for name, l := range c.Layers {
		if l.Inline == nil || BandOf(l.Size) == BandNameInline {
			continue
		}
		if dropped == nil {
			dropped = make(map[string]LayerRef, len(c.Layers))
			for k, v := range c.Layers {
				dropped[k] = v
			}
		}
		l.Inline = nil
		dropped[name] = l
	}
	if dropped != nil {
		c.Layers = dropped
	}
}

// InlineData builds the layer map for a contribution whose only layer is its
// structured content. A TEST-FACING helper: production producers call SetData;
// fixtures across the repo build entries in one expression with this.
//
// The composite-literal form of SetData, for producers that build a
// contribution in one expression. Same rule about residency: the band decides,
// not the caller.
func InlineData(b json.RawMessage) map[string]LayerRef {
	var c Contribution
	c.SetData(b)
	return c.Layers
}

// DataDescriptor builds the layer map for a contribution whose structured
// content is NOT carried (a TEST-FACING helper for index-plane fixtures) — the index plane, where an entry says what its
// content is and where to get it, without being it.
//
// The counterpart to InlineData, and the reason DropInline keeps the
// descriptor: a consumer holding one of these can resolve the content by hash
// through the content lane, which is the whole two-plane model in one value.
func DataDescriptor(hash string, size int64) map[string]LayerRef {
	if hash == "" {
		return nil
	}
	return map[string]LayerRef{LayerData: {Hash: hash, Size: size, MediaType: "application/json"}}
}

// SetData declares this contribution's structured content.
//
// The producer has the bytes — it just built them — so it carries them, and
// the band decides nothing here. Residency is a question about the INDEX, not
// about the producer: see DropInline, which is where the band table is
// actually applied.
//
// The one exception is the bulk band, which is not an index question but a
// memory one: a structured body no process can hold is not something to hold.
func (c *Contribution) SetData(b json.RawMessage) {
	if len(b) == 0 {
		delete(c.Layers, LayerData)
		return
	}
	if c.Layers == nil {
		c.Layers = map[string]LayerRef{}
	}
	ref := LayerRef{
		Hash:      HashContent(b),
		Size:      int64(len(b)),
		MediaType: "application/json",
	}
	if !BandOf(ref.Size).IsBulk() {
		ref.Inline = b
	}
	c.Layers[LayerData] = ref
}

// IsMetaKind reports whether a kind is part of the meta-model itself.
//
// The bootstrap base case: `contribution-type` and `data-type` cannot name
// their own schema without circularity — a contribution-type describing
// contribution-type — so schema resolution terminates here, at the system
// data-type of the same name, rather than following a declaration that cannot
// exist.
func IsMetaKind(kind string) bool {
	return kind == ResourceTypeContributionType || kind == ResourceTypeDataType
}

// === The source layer ===

// Source returns this contribution's readable content, when the index carries
// it, else nil.
//
// Resident content only, like Data. Most kinds do NOT carry their source in the
// index — a skill's markdown is what a reader renders, never what a picker
// shows — so nil here is the normal answer and means "fetch it with Open",
// not "there is none". SourceRef says whether there is one.
func (c Contribution) Source() []byte { return c.Layers[LayerSource].Inline }

// SourceRef returns the source layer's descriptor, and whether it exists.
//
// The question to ask before Source: a nil Source with a descriptor means the
// content is elsewhere, and a nil Source with no descriptor means there is
// none. Collapsing the two is how "not carried right now" gets read as
// "nothing here".
func (c Contribution) SourceRef() (LayerRef, bool) {
	l, ok := c.Layers[LayerSource]
	return l, ok
}

// SetSource declares this contribution's readable content, carried in the
// index.
//
// For a producer whose content has no file to point at — a schema derived from
// a Go type, a task's inline I/O declaration. A file-backed producer describes
// its source from the file instead, and decides residency there.
//
// `mediaType` is the ONE statement of what the bytes are. A producer that also
// recorded the format inside the content would be answering the same question
// twice, in two places that can disagree.
func (c *Contribution) SetSource(b []byte, mediaType string) { c.SetLayer(LayerSource, b, mediaType) }

// SetLayer declares one named layer with its bytes — addressed by hash,
// carried inline up to the bulk band, exactly as SetSource does for `source`.
// For the layers a kind declares beyond `data` and `source` (a skill's
// `instructions`). Empty bytes remove the layer.
func (c *Contribution) SetLayer(name string, b []byte, mediaType string) {
	if len(b) == 0 {
		delete(c.Layers, name)
		return
	}
	if c.Layers == nil {
		c.Layers = map[string]LayerRef{}
	}
	ref := LayerRef{Hash: HashContent(b), Size: int64(len(b)), MediaType: mediaType}
	if !BandOf(ref.Size).IsBulk() {
		ref.Inline = b
	}
	c.Layers[name] = ref
}
