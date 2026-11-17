// SPDX-License-Identifier: Apache-2.0

package api

// Content templates (modules/doc-engine/, th21): WHAT a deliverable must say,
// whatever its format — a deck, a document, later a Confluence page. A template
// is an ordered list of sections, each with what it must establish (intent),
// where its facts come from (sources, relative to the object the deliverable
// is about), guidance, and optional hints per format. It never holds the text:
// the agent writing a format writes it. A partial template is a section group
// (« options and recommendation », « risks ») that a full template includes.
//
// Templates are `content-template` contributions (a sidecar ships them) and
// `content-template` objects of the project (a team's own), the project's
// first — as deck themes are.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

const (
	// ContributionTypeContentTemplate is the kind a sidecar publishes a template as.
	ContributionTypeContentTemplate = "content-template"
	// ObjectTypeContentTemplate is a template stored in the project; its
	// declaration is its CapabilityContentTemplate capability.
	ObjectTypeContentTemplate = "content-template"
	CapabilityContentTemplate = "content-template"
	// OptionSourceContentTemplates is the `options_from` source of TemplateOptions.
	OptionSourceContentTemplates = "content-templates"
	// ProviderProject is the provider a project template is attributed to.
	ProviderProject = "project"
)

// ContentTemplate is a template's declaration.
type ContentTemplate struct {
	Name        string            `json:"name"                  jsonschema:"description=The template's name in kebab-case - how a task or another template names it"`
	Label       string            `json:"label,omitempty"       jsonschema:"description=The name shown in a picker"`
	Description string            `json:"description"           jsonschema:"description=What the deliverable is for - one or two sentences"`
	Purpose     string            `json:"purpose,omitempty"     jsonschema:"description=The moment it serves such as an architecture board or a steering committee"`
	Target      string            `json:"target,omitempty"      jsonschema:"description=The object type it is about such as dossier - empty for any"`
	Partial     bool              `json:"partial,omitempty"     jsonschema:"description=A section group other templates include - never offered on its own"`
	Sections    []TemplateSection `json:"sections"              jsonschema:"description=The sections in reading order"`
}

// TemplateSection is one part of a deliverable — or, with Include, the
// sections of a partial template at this place.
type TemplateSection struct {
	ID       string           `json:"id"                 jsonschema:"description=A handle unique in the template in kebab-case"`
	Include  string           `json:"include,omitempty"  jsonschema:"description=A partial template whose sections go here - then no other field but id"`
	Title    string           `json:"title,omitempty"    jsonschema:"description=The section's title as a reader sees it"`
	Intent   string           `json:"intent,omitempty"   jsonschema:"description=What the section must establish for its reader"`
	Sources  []TemplateSource `json:"sources,omitempty"  jsonschema:"description=Where its facts come from - never invented"`
	Guidance string           `json:"guidance,omitempty" jsonschema:"description=How to write it - density and pitfalls and what to leave out"`
	Required bool             `json:"required,omitempty" jsonschema:"description=Kept in a short version - a missing fact is listed rather than the section dropped"`
	Repeat   string           `json:"repeat,omitempty"   jsonschema:"description=One section per child object of this type such as dossier-solution"`
	Weight   int              `json:"weight,omitempty"   jsonschema:"description=Relative importance from 1 to 5 - how much of the speaking time or pages it takes"`
	Deck     *DeckHint        `json:"deck,omitempty"     jsonschema:"description=How the section shows in a deck when the default is not enough"`
	Doc      *DocHint         `json:"doc,omitempty"      jsonschema:"description=How the section shows in a document when the default is not enough"`
}

// TemplateSource names where a section's facts come from, relative to the
// object the deliverable is about.
type TemplateSource struct {
	From  string `json:"from"            jsonschema:"description=What kind of source,enum=field|children|named-file|files|links|object|form"`
	Value string `json:"value,omitempty" jsonschema:"description=Which one - a field name or an object type or a file name or a glob or a link kind or a form field"`
	Note  string `json:"note,omitempty"  jsonschema:"description=What to take from it"`
}

// DeckHint is a section's look in a deck.
type DeckHint struct {
	Layout string `json:"layout,omitempty" jsonschema:"description=The layout its main slide takes such as comparison or timeline or key-figures"`
	Group  string `json:"group,omitempty"  jsonschema:"description=A slide group shown for it"`
}

// DocHint is a section's look in a document.
type DocHint struct {
	Level int `json:"level,omitempty" jsonschema:"description=Its heading level from 1 to 4"`
}

var templateName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// Expand is t with every include replaced by the sections of the partial
// template it names (in turn expanded); lookup finds a template by name. An
// included section's id is prefixed by the include's id, so ids stay unique.
func Expand(t ContentTemplate, lookup func(name string) (ContentTemplate, bool)) (ContentTemplate, error) {
	parts, err := Parts(t, lookup)
	if err != nil {
		return ContentTemplate{}, err
	}
	t.Sections = make([]TemplateSection, len(parts))
	for i, p := range parts {
		t.Sections[i] = p.Section
	}
	return t, nil
}

// TemplatePart is one section of an expanded template with where it comes
// from: Groups names the partial templates it was included through, outermost
// first — empty for a section the template writes itself.
type TemplatePart struct {
	Section TemplateSection `json:"section"`
	Groups  []string        `json:"groups,omitempty"`
}

// Parts is t's sections in reading order with every include expanded (as
// Expand does), each marked with the groups it came through.
func Parts(t ContentTemplate, lookup func(name string) (ContentTemplate, bool)) ([]TemplatePart, error) {
	var walk func(t ContentTemplate, prefix string, groups, seen []string) ([]TemplatePart, error)
	walk = func(t ContentTemplate, prefix string, groups, seen []string) ([]TemplatePart, error) {
		for _, s := range seen {
			if s == t.Name {
				return nil, fmt.Errorf("template %q includes itself (%v)", t.Name, append(seen, t.Name))
			}
		}
		seen = append(seen, t.Name)
		var out []TemplatePart
		for _, s := range t.Sections {
			if s.Include == "" {
				s.ID = prefix + s.ID
				out = append(out, TemplatePart{Section: s, Groups: groups})
				continue
			}
			inc, ok := lookup(s.Include)
			if !ok {
				return nil, fmt.Errorf("section %q includes %q, which is no template in reach", s.ID, s.Include)
			}
			if !inc.Partial {
				return nil, fmt.Errorf("section %q includes %q, which is not a partial template", s.ID, s.Include)
			}
			sub, err := walk(inc, prefix+s.ID+".", append(append([]string(nil), groups...), inc.Name), seen)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
		}
		return out, nil
	}
	return walk(t, "", nil, nil)
}

// Check names what is wrong with a template, alone and with its includes
// resolved through lookup (nil: includes are not followed). Empty when sound.
func Check(t ContentTemplate, lookup func(name string) (ContentTemplate, bool)) []string {
	var out []string
	if !templateName.MatchString(t.Name) {
		out = append(out, fmt.Sprintf("name %q: kebab-case, at most 63 characters", t.Name))
	}
	if t.Description == "" {
		out = append(out, "a template says what its deliverable is for (description)")
	}
	if len(t.Sections) == 0 {
		out = append(out, "a template has at least one section")
	}
	ids := map[string]bool{}
	for i, s := range t.Sections {
		at := fmt.Sprintf("section %d (%s)", i+1, s.ID)
		if !templateName.MatchString(s.ID) {
			out = append(out, at+": its id is kebab-case")
		}
		if ids[s.ID] {
			out = append(out, at+": its id is used twice")
		}
		ids[s.ID] = true
		if s.Include != "" {
			if s.Title != "" || s.Intent != "" || len(s.Sources) > 0 || s.Guidance != "" || s.Repeat != "" || s.Deck != nil || s.Doc != nil {
				out = append(out, at+": an include carries nothing but its id")
			}
			continue
		}
		if s.Title == "" || s.Intent == "" {
			out = append(out, at+": a section has a title and an intent")
		}
		if s.Weight < 0 || s.Weight > 5 {
			out = append(out, at+": weight is from 1 to 5")
		}
		if s.Doc != nil && (s.Doc.Level < 1 || s.Doc.Level > 4) {
			out = append(out, at+": doc.level is from 1 to 4")
		}
		for _, src := range s.Sources {
			switch src.From {
			case "field", "children", "named-file", "files", "links", "object", "form":
			default:
				out = append(out, fmt.Sprintf("%s: source from %q is none of field, children, named-file, files, links, object, form", at, src.From))
			}
		}
	}
	if lookup != nil && len(out) == 0 {
		if _, err := Expand(t, lookup); err != nil {
			out = append(out, err.Error())
		}
	}
	return out
}

// --- the templates in reach ----------------------------------------------------

// TemplateObjects is what the resolver reads project templates through.
// CWMService has it; a host without it in process (the GUI) passes its own
// with WithTemplateObjects.
type TemplateObjects interface {
	ObjectList(ctx context.Context, in cwmapi.ObjectListInput) (cwmapi.ObjectListOutput, error)
}

type templateObjectsKey struct{}

// WithTemplateObjects makes the resolver read project templates through src.
func WithTemplateObjects(ctx context.Context, src TemplateObjects) context.Context {
	return context.WithValue(ctx, templateObjectsKey{}, src)
}

func templateObjects(ctx context.Context) TemplateObjects {
	if src, ok := ctx.Value(templateObjectsKey{}).(TemplateObjects); ok && src != nil {
		return src
	}
	if application.AppFromContext(ctx) == nil {
		return nil
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil || cwm == nil {
		return nil
	}
	return cwm
}

// TemplateOfObject reads a content-template object's declaration.
func TemplateOfObject(o cwmapi.Object) (ContentTemplate, error) {
	raw, ok := o.Capabilities[CapabilityContentTemplate]
	if !ok {
		return ContentTemplate{}, fmt.Errorf("%s has no %s capability", o.ID, CapabilityContentTemplate)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return ContentTemplate{}, err
	}
	var t ContentTemplate
	if err := json.Unmarshal(b, &t); err != nil {
		return ContentTemplate{}, fmt.Errorf("%s: %w", o.ID, err)
	}
	return t, nil
}

// Templates is every template in reach — the project's first, hiding a
// contribution of the same name — by name.
func Templates(ctx context.Context, q mbapi.Q) ([]mbapi.Attributed[ContentTemplate], error) {
	q.Kind = ContributionTypeContentTemplate
	entries, err := mbapi.QueryTyped[ContentTemplate](ctx, q)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []mbapi.Attributed[ContentTemplate]
	if src := templateObjects(ctx); src != nil {
		if objs, err := src.ObjectList(ctx, cwmapi.ObjectListInput{Type: ObjectTypeContentTemplate}); err == nil {
			for _, o := range objs.Objects {
				t, err := TemplateOfObject(o)
				if err != nil || !templateName.MatchString(t.Name) || seen[t.Name] {
					continue
				}
				seen[t.Name] = true
				out = append(out, mbapi.Attributed[ContentTemplate]{Provider: ProviderProject, Body: t})
			}
		}
	}
	for _, e := range entries {
		if !seen[e.Body.Name] {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Body.Name < out[j].Body.Name })
	return out, nil
}

// TemplateOptions is the `content-templates` option source: every full
// template in reach (partial ones are only included) as `name`, `label`,
// `description`, `purpose`, `target` and `provider`. A form parameter
// `target` keeps the templates about that object type (and those about any).
func TemplateOptions(q mbapi.Q) schemaapi.OptionSource {
	return schemaapi.OptionSourceFunc(func(ctx context.Context, params map[string]any) ([]map[string]any, error) {
		all, err := Templates(ctx, q)
		if err != nil {
			return nil, err
		}
		want, _ := params["target"].(string)
		out := make([]map[string]any, 0, len(all))
		for _, t := range all {
			b := t.Body
			if b.Partial || (want != "" && b.Target != "" && b.Target != want) {
				continue
			}
			label := b.Label
			if label == "" {
				label = b.Name
			}
			out = append(out, map[string]any{"name": b.Name, "label": label, "description": b.Description, "purpose": b.Purpose, "target": b.Target, "provider": t.Provider})
		}
		return out, nil
	})
}
