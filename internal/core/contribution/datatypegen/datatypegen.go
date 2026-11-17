// SPDX-License-Identifier: Apache-2.0

// Package datatypegen renders a system data-type's DSL from the Go type that
// consumers decode it with — one source for a contract, instead of a
// hand-written schema held to a struct by a drift test.
//
// `reflect.TypeAST` walks the type into the parser's own AST (descriptions
// from `jsonschema:"description=…"` tags, optionality from `omitempty`,
// `jsonschema:"required"` to override), a `schemaapi.Schema` wraps it, and
// the document is emitted as YAML with a header naming its origin. The file
// lands under `internal/core/contribution/embed/data-type/` through
// `go generate` (tools/gen-data-type), so every binary ships it exactly as
// today; a test refuses a stale file.
package datatypegen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
	ocapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	objectpaneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	reviewsapi "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
	universeviewapi "github.com/katyoti/katyoti-fwk/internal/extensions/universeview/api"
)

// Source is one generated data-type: which Go type, and the document header
// the DSL carries around it.
type Source struct {
	// Name is the data-type id — the file is `<Name>.yaml`, the URI
	// `system:/data-type/<Name>`.
	Name string
	// Version is the schema's own version, bumped when the type changes shape.
	Version string
	// Root is the DSL root type name.
	Root string
	// Description is the document-level description.
	Description string
	// Origin names the Go type, for the header comment.
	Origin string
	// AST renders the root type; wraps reflect.TypeAST for one T.
	AST func() (*schemaapi.Type, error)
	// TS, when true, also renders the type as a TypeScript interface for the
	// GUI (`<Name>.ts`, see RenderTS) — the frontend imports it, never a
	// hand-written copy.
	TS bool
	// Named are nested types the document keeps under their own name in
	// `types:` instead of inline: the shape a schema already published named
	// keeps its field paths, so its format ledger reads the same fields.
	Named []NamedType
}

// NamedType is a nested type a document names: wherever the root (or another
// named type) holds an object of exactly its fields, the document refers to
// it by name.
type NamedType struct {
	Name        string
	Description string
	AST         func() (*schemaapi.Type, error)
}

// Registry lists every generated system data-type. Adding one here is the
// whole registration: the tool renders it, the test guards it.
var Registry = []Source{
	{
		Name:    "contribution-type",
		Version: "1.9.0",
		Root:    "ContributionType",
		Origin:  "contribapi.KindDeclaration (internal/core/contribution/api/kind.go, kind_source.go)",
		Description: "Defines a contribution KIND — one entry in the open list of artifact\n" +
			"families a sidecar can advertise: the kebab discriminator written into\n" +
			"Contribution.Type, its layers, how its files become them, and the URI\n" +
			"of the data-type its data layer validates against (the body shape is\n" +
			"that data-type's, one per kind).\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[contribapi.KindDeclaration]() },
		Named: []NamedType{
			{Name: "LayerDecl", Description: "What a kind says about one of its layers.",
				AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[contribapi.LayerDecl]() }},
			{Name: "SourceDecl", Description: "How the files of a pack become entries of a kind.",
				AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[contribapi.SourceDecl]() }},
			{Name: "LayerFrom", Description: "What one layer of an entry is taken from.",
				AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[contribapi.LayerFrom]() }},
			{Name: "Companion", Description: "An entry of another kind read from a file beside an entry.",
				AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[contribapi.Companion]() }},
		},
	},
	{
		Name:    "object-panel",
		Version: "2.0.0",
		Root:    "ObjectPanel",
		Origin:  "objectpaneapi.PanelDecl (internal/extensions/objectpane/api/panel.go)",
		Description: "One tab of the object pane: name, tooltip, icon and order; a filter\n" +
			"(types, capabilities, an el when) deciding which objects show it; and\n" +
			"what it shows, as inline content or a source method.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[objectpaneapi.PanelDecl]() },
	},
	{
		Name:    "panel-component",
		Version: "1.0.0",
		Root:    "PanelComponent",
		Origin:  "objectpaneapi.PanelComponentDecl (internal/extensions/objectpane/api/component.go)",
		Description: "A named view an object panel mounts (content kind extension): what it\n" +
			"shows and the data-type of what it receives.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[objectpaneapi.PanelComponentDecl]() },
	},
	{
		Name:    "hook",
		Version: "1.2.0",
		Root:    "Hook",
		Origin:  "cwmapi.Hook (internal/extensions/cwm/api/hooks.go)",
		Description: "One hook at a point, from a pack: the action to run, when, on which\n" +
			"objects, with which input.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[cwmapi.Hook]() },
	},
	{
		Name:    "hook-type",
		Version: "1.0.0",
		Root:    "HookType",
		Origin:  "cwmapi.HookType (internal/extensions/cwm/api/hooks.go)",
		Description: "A point at which declared behaviour runs: when it fires and what an\n" +
			"answer to it is.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[cwmapi.HookType]() },
	},
	{
		Name:    "event-type",
		Version: "1.0.0",
		Root:    "EventType",
		Origin:  "cwmapi.EventType (internal/extensions/cwm/api/event_types.go)",
		Description: "A kind of event a module emits: its name, its payload, whether other\n" +
			"modules may subscribe to it and the hook point an asynchronous\n" +
			"subscription to it is declared at.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[cwmapi.EventType]() },
	},
	{
		Name:    "object-event",
		Version: "1.4.0",
		Root:    "ObjectEvent",
		Origin:  "cwmapi.ObjectEventPayload (internal/extensions/cwm/api/event_payloads.go)",
		Description: "The payload of an object write's event: the event's id, time, revision\n" +
			"and cause, and the object written (id, module, type, parent, slug)\n" +
			"with the action.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[cwmapi.ObjectEventPayload]() },
	},
	{
		Name:    "object-transitioned-event",
		Version: "1.5.0",
		Root:    "ObjectTransitionedEvent",
		Origin:  "cwmapi.ObjectTransitionedEventPayload (internal/extensions/cwm/api/event_payloads.go)",
		Description: "The payload of a transition's event: the event's id, time, revision\n" +
			"and cause, the object moved, the workflow (or capability) whose state\n" +
			"moved, from, to, the transition, who made it, the note, whether it\n" +
			"was forced and what the guard's functions answered then.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[cwmapi.ObjectTransitionedEventPayload]() },
	},
	{
		Name:    "object-transition-refused-event",
		Version: "1.4.0",
		Root:    "ObjectTransitionRefusedEvent",
		Origin:  "cwmapi.ObjectTransitionRefusedEventPayload (internal/extensions/cwm/api/event_payloads.go)",
		Description: "The payload of a move its guard refused: the event's id, time, the\n" +
			"object's current revision and cause, the object, the move asked (from,\n" +
			"to, the transition, who asked), the guard, why it did not hold and what\n" +
			"each function it called answered.\n",
		AST: func() (*schemaapi.Type, error) {
			return ktwreflect.TypeAST[cwmapi.ObjectTransitionRefusedEventPayload]()
		},
	},
	{
		Name:    "objects-caught-up-event",
		Version: "1.1.0",
		Root:    "ObjectsCaughtUpEvent",
		Origin:  "cwmapi.ObjectsCaughtUpEventPayload (internal/extensions/cwm/api/event_payloads.go)",
		Description: "The payload of a catch-up's one grouped event: the ids of the objects\n" +
			"added, modified and removed outside the CWM verbs, the cause (commits\n" +
			"between two HEADs or the working tree) and who made the change - an\n" +
			"outside actor, the commit's author with its sha - how many moved, and\n" +
			"whether their per-object events are held until CatchUpRelease.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[cwmapi.ObjectsCaughtUpEventPayload]() },
	},
	{
		Name:    "agent-context",
		Version: "1.0.0",
		Root:    "AgentFraming",
		Origin:  "ocapi.AgentFraming (internal/extensions/objectcontext/api/context_section.go)",
		Description: "The framing an extension gives a chat started on the objects its filter\n" +
			"admits: system sections, tools, skills and named files.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[ocapi.AgentFraming]() },
	},
	{
		Name:    "workflow",
		Version: "1.0.0",
		Root:    "Workflow",
		Origin:  "cwmapi.Workflow (internal/extensions/cwm/api/workflow.go)",
		Description: "A named state machine an object's lifecycle moves through: its states,\n" +
			"the initial one, and the transitions allowed between them, each with an\n" +
			"optional gate over the object and the actor.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[cwmapi.Workflow]() },
	},
	{
		Name:    "gate",
		Version: "1.0.0",
		Root:    "Gate",
		Origin:  "gatesapi.Gate (internal/extensions/gates/api/types.go)",
		Description: "A named definition of done: its requirements, each checked on its own\n" +
			"(an el predicate, an action answering accept, or a person's confirmation)\n" +
			"and resolved as its resolve says when unmet.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[gatesapi.Gate]() },
	},
	{
		Name:    "gate-requirement",
		Version: "1.0.0",
		Root:    "GateRequirement",
		Origin:  "gatesapi.GateRequirement (internal/extensions/gates/api/types.go)",
		Description: "A requirement a type or a pack adds to a named gate: the gate, the objects\n" +
			"it applies to (by types and capabilities), how it is checked and how it\n" +
			"is resolved.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[gatesapi.GateRequirement]() },
	},
	{
		Name:    "review-type",
		Version: "1.1.0",
		Root:    "ReviewType",
		Origin:  "reviewsapi.ReviewTypeDef (internal/extensions/reviews/api/review_type.go)",
		Description: "One review type a module or an object can be reviewed by: its title and\n" +
			"description, the dimension it is grouped under, the objects it applies\n" +
			"to, when it runs and what performs it (a skill or a check).\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[reviewsapi.ReviewTypeDef]() },
	},
	{
		Name:    "universe-tag",
		Version: "3.0.0",
		Root:    "UniverseTag",
		Origin:  "universeviewapi.UniverseTag (internal/extensions/universeview/api/api.go)",
		Description: "A universe tag: the GUI elements a universe carrying it shows - one\n" +
			"list of element ids (the place in each id) - each with the lowest display\n" +
			"level that sees it; and the task hashtags listed first.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[universeviewapi.UniverseTag]() },
		Named: []NamedType{
			{Name: "TagItem", Description: "One element a tag allows and the lowest level that sees it.",
				AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[universeviewapi.TagItem]() }},
		},
	},
	{
		Name:    "policy",
		Version: "1.3.0",
		Root:    "Policy",
		Origin:  "cwmapi.Policy (internal/extensions/cwm/api/policy.go)",
		Description: "A rule put on objects by shape: what it says, the types and capabilities\n" +
			"it applies to, the checks that measure it and what an agent working on\n" +
			"such an object is told. It holds no field of the object.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[cwmapi.Policy]() },
	},
	{
		Name:    "render-def",
		Version: "2.0.0",
		Root:    "RenderDef",
		Origin:  "render.Def (internal/core/render/def.go)",
		Description: "One thing to display: which component and what it shows. The same\n" +
			"definition serves an object panel, a method's result, an LLM display\n" +
			"and a terminal artifact.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[render.Def]() },
		TS:  true,
	},
	{
		Name:    "document",
		Version: "1.0.0",
		Root:    "Document",
		Origin:  "render.Document (internal/core/render/document.go)",
		Description: "A document to display: its media type, which picks the display engine,\n" +
			"and its content inline or the source it is read from.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[render.Document]() },
		TS:  true,
	},
	{
		Name:    "display-engine",
		Version: "1.0.0",
		Root:    "DisplayEngine",
		Origin:  "render.EngineDecl (internal/core/render/engine.go)",
		Description: "A display engine a sidecar contributes: the media types it shows\n" +
			"and its capabilities; its widget page is the contribution's widget layer.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[render.EngineDecl]() },
		TS:  true,
	},
	{
		Name:    "panel-type",
		Version: "1.0.0",
		Root:    "PanelType",
		Origin:  "panel.TypeDecl (internal/core/panel/type.go)",
		Description: "A panel type: what a display panel of that type shows - its page tree\n" +
			"named by URI or inline - the type of its data, the inputs a person may\n" +
			"send it, how long an ended one is kept and what stays on its object.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[panel.TypeDecl]() },
	},
	{
		Name:    "page-template",
		Version: "1.0.0",
		Root:    "PageTemplate",
		Origin:  "render.PageTemplate (internal/core/render/template.go)",
		Description: "A page template: a page tree in the page DSL named by its URI, which an\n" +
			"answer names by URI and hash instead of carrying it.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[render.PageTemplate]() },
	},
	{
		Name:    "guide-recipe",
		Version: "1.7.0",
		Root:    "Recipe",
		Origin:  "guideapi.Recipe (internal/extensions/guide/api/recipe.go)",
		Description: "A guide: a Help me's ordered steps the GUI plays over its own controls\n" +
			"(ending in a checked result when a step names a check; with no check a\n" +
			"tour) - a Show me's video - or an Explain me's markdown body. Its file\n" +
			"name is its identity.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[guideapi.Recipe]() },
		TS:  true,
	},
}

// Names lists the registry in order.
func Names() []string {
	out := make([]string, 0, len(Registry))
	for _, s := range Registry {
		out = append(out, s.Name)
	}
	return out
}

// Header is what a rendered document's comment says about where it comes
// from: the tool that wrote it, how to regenerate it, the URI it is read at.
type Header struct {
	Tool       string
	Regenerate string
	URI        string
}

// Render returns the YAML document for one registered data-type, byte-stable
// across runs: the AST is marshalled to canonical JSON, decoded to a generic
// tree and emitted by yaml.v3, which sorts map keys.
func Render(name string) ([]byte, error) {
	var src *Source
	for i := range Registry {
		if Registry[i].Name == name {
			src = &Registry[i]
		}
	}
	if src == nil {
		return nil, fmt.Errorf("datatypegen: no registered source named %q (have %v)", name, Names())
	}
	return RenderAs(*src, Header{
		Tool:       "tools/gen-data-type",
		Regenerate: "`go generate ./internal/core/contribution/`",
		URI:        "system:/data-type/" + src.Name,
	})
}

// RenderAs renders a data-type that is not a system one — a field-group an
// extension's pack declares and its service decodes with a Go type — with the
// header that says where it lives. The extension holds the file current with
// its own test; the document is rendered exactly as Render renders.
func RenderAs(src Source, h Header) ([]byte, error) {
	name := src.Name
	root, err := src.AST()
	if err != nil {
		return nil, fmt.Errorf("datatypegen %s: %w", name, err)
	}
	doc := schemaapi.NewSchema(src.Name)
	doc.Version = src.Version
	doc.Root = src.Root
	doc.Description = src.Description
	doc.Types = map[string]*schemaapi.Type{src.Root: root}
	if err := hoistNamed(doc.Types, src.Named); err != nil {
		return nil, fmt.Errorf("datatypegen %s: %w", name, err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("datatypegen %s: marshal: %w", name, err)
	}
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("datatypegen %s: reread: %w", name, err)
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# GENERATED by %s from %s — DO NOT EDIT.\n", h.Tool, src.Origin)
	fmt.Fprintf(&buf, "# The Go type is the one source of this contract: change the struct (and its\n")
	fmt.Fprintf(&buf, "# jsonschema description tags), then %s.\n", h.Regenerate)
	fmt.Fprintf(&buf, "# A test refuses a stale file. URI: %s\n", h.URI)
	doc2 := orderedDoc(tree)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc2); err != nil {
		return nil, fmt.Errorf("datatypegen %s: yaml: %w", name, err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// hoistNamed puts each named type in types under its name, and replaces every
// object of exactly its fields — in the root and in the named types — by a
// reference to it, keeping the field's own optionality and description.
// Matched top-down on the walked shapes: a named type holding another is
// matched whole, then its own fields are hoisted.
func hoistNamed(types map[string]*schemaapi.Type, named []NamedType) error {
	if len(named) == 0 {
		return nil
	}
	bySig := map[string]string{}
	for _, n := range named {
		t, err := n.AST()
		if err != nil {
			return fmt.Errorf("named type %s: %w", n.Name, err)
		}
		t.Description = n.Description
		sig, err := fieldsSig(t)
		if err != nil {
			return err
		}
		bySig[sig] = n.Name
		types[n.Name] = t
	}
	match := func(t *schemaapi.Type) string {
		if t == nil || t.Kind != schemaapi.KindObject || len(t.Fields) == 0 {
			return ""
		}
		sig, err := fieldsSig(t)
		if err != nil {
			return ""
		}
		return bySig[sig]
	}
	var walk func(t *schemaapi.Type)
	walkRef := func(r *schemaapi.TypeRef) {
		if r == nil || r.Inline == nil {
			return
		}
		if n := match(r.Inline); n != "" {
			*r = *schemaapi.RefName(n)
			return
		}
		walk(r.Inline)
	}
	walk = func(t *schemaapi.Type) {
		for k, f := range t.Fields {
			if n := match(f); n != "" {
				t.Fields[k] = &schemaapi.Type{Kind: schemaapi.TypeKind(n), Optional: f.Optional,
					Description: f.Description, Label: f.Label}
				continue
			}
			walk(f)
		}
		walkRef(t.Items)
		walkRef(t.Catchall)
	}
	// Every type's fields and references, never the type itself: the
	// signatures were taken before any of them changed.
	for _, t := range types {
		walk(t)
	}
	return nil
}

// fieldsSig is an object's field set as canonical JSON — what a named type is
// recognised by.
func fieldsSig(t *schemaapi.Type) (string, error) {
	b, err := json.Marshal(t.Fields)
	return string(b), err
}

// orderedDoc puts the document header keys first, in the order a reader
// expects, then the rest alphabetically. yaml.v3 emits a mapping Node in the
// order its Content was built.
func orderedDoc(tree map[string]any) yaml.Node {
	first := []string{"$schema", "name", "version", "root", "description"}
	seen := map[string]bool{}
	keys := make([]string, 0, len(tree))
	for _, k := range first {
		if _, ok := tree[k]; ok {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	rest := make([]string, 0, len(tree))
	for k := range tree {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	keys = append(keys, rest...)
	doc := yaml.Node{Kind: yaml.MappingNode}
	for _, k := range keys {
		var kn, vn yaml.Node
		kn.SetString(k)
		if err := vn.Encode(tree[k]); err != nil {
			continue
		}
		doc.Content = append(doc.Content, &kn, &vn)
	}
	return doc
}
