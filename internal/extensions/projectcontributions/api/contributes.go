// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"path"
	"sort"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// CapContributes is the capability that publishes an object as a
// contribution entry of a declared kind (decisions and
// OD-1354): named `contributes`, never `contributions` - that is
// the project's folder-and-token capability.
const CapContributes = "contributes"

// Contributes is the `contributes` capability's field-group, and the Go type
// its data-type `cap-contributes` is generated from: the kind the object is
// published as, its id, where each of its layers comes from and the entry it
// overrides.
type Contributes struct {
	Kind      string            `json:"kind" yaml:"kind" jsonschema:"title=Kind,description=The kind the object is published as - any declared kind (deck-theme; persona; task)"`
	ID        string            `json:"id,omitempty" yaml:"id,omitempty" jsonschema:"title=Id,description=The entry's id under the kind; empty means the object's slug and else its id"`
	Layers    map[string]string `json:"layers" yaml:"layers" jsonschema:"title=Layers,description=Where each layer comes from by layer name: body; fields; fields.<key>; capabilities.<capability>; capabilities.<capability>.<key>; files/<path relative to the object's module>"`
	Overrides string            `json:"overrides,omitempty" yaml:"overrides,omitempty" jsonschema:"title=Overrides,description=The URI of the entry this one overrides (provider:/kind/id); it then carries only the layers it replaces and inherits the others"`
}

// ContributesOf reads an object's `contributes` field-group; ok is false when
// the object carries none.
func ContributesOf(o cwmapi.Object) (Contributes, bool, error) {
	group, ok := o.Capabilities[CapContributes]
	if !ok {
		return Contributes{}, false, nil
	}
	raw, err := json.Marshal(group)
	if err != nil {
		return Contributes{}, true, err
	}
	var c Contributes
	if err := json.Unmarshal(raw, &c); err != nil {
		return Contributes{}, true, fmt.Errorf("object %s: capability %s: %w", o.ID, CapContributes, err)
	}
	return c, true, nil
}

// ObjectFiles reads a file of an object's module: CWMService has it; a host
// without CWM in process passes its own.
type ObjectFiles interface {
	ObjectFileRead(ctx context.Context, in cwmapi.ObjectFileReadInput) (cwmapi.ObjectFileReadOutput, error)
}

// ObjectEntry is one object published as a contribution entry: the entry, the
// object it comes from and, for an override, the URI it overrides.
// It travels in a push (EntriesPush) as JSON.
type ObjectEntry struct {
	Entry     contribapi.Contribution `json:"entry"`
	Object    string                  `json:"object"`
	Overrides string                  `json:"overrides,omitempty"`
}

// URI is the entry's own URI (<token>:/<kind>/<id>).
func (e ObjectEntry) URI() string { return e.Entry.ResolvedURI() }

// Layer sources a `contributes` field-group names.
const (
	SourceBody         = "body"
	SourceFields       = "fields"
	SourceCapabilities = "capabilities"
	SourceFiles        = "files/"
)

// EntryOfObject projects an object carrying `contributes` into the entry it
// publishes under the project's provider token: its URI, its layers read from
// the object (body, fields, a capability's field-group, a file of its module)
// and the URI it overrides. An object without the capability is an error: the
// caller lists the objects that carry it.
func EntryOfObject(ctx context.Context, o cwmapi.Object, token string, files ObjectFiles) (ObjectEntry, error) {
	c, ok, err := ContributesOf(o)
	if err != nil {
		return ObjectEntry{}, err
	}
	if !ok {
		return ObjectEntry{}, fmt.Errorf("object %s carries no %s capability", o.ID, CapContributes)
	}
	id := c.ID
	if id == "" {
		id = o.Slug
	}
	if id == "" {
		id = o.ID
	}
	uri, err := contribapi.NewResourceURI(token, c.Kind, id)
	if err != nil {
		return ObjectEntry{}, fmt.Errorf("object %s: %w", o.ID, err)
	}
	if c.Overrides != "" {
		over, err := contribapi.ParseResourceURI(c.Overrides)
		if err != nil {
			return ObjectEntry{}, fmt.Errorf("object %s: overrides: %w", o.ID, err)
		}
		if over.Type != c.Kind {
			return ObjectEntry{}, fmt.Errorf("object %s: overrides %s, an entry of kind %s, not %s", o.ID, c.Overrides, over.Type, c.Kind)
		}
		if over.Provider == token {
			return ObjectEntry{}, fmt.Errorf("object %s: overrides %s, an entry of its own project", o.ID, c.Overrides)
		}
	}
	if len(c.Layers) == 0 {
		return ObjectEntry{}, fmt.Errorf("object %s: %s declares no layer", o.ID, CapContributes)
	}
	entry := contribapi.Contribution{Provider: uri.Provider, Type: uri.Type, ID: uri.ID, URI: uri.String()}
	names := make([]string, 0, len(c.Layers))
	for name := range c.Layers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		b, media, err := layerOf(ctx, o, c.Layers[name], files)
		if err != nil {
			return ObjectEntry{}, fmt.Errorf("object %s: layer %s: %w", o.ID, name, err)
		}
		if len(b) == 0 {
			return ObjectEntry{}, fmt.Errorf("object %s: layer %s: %s is empty", o.ID, name, c.Layers[name])
		}
		entry.SetLayer(name, b, media)
	}
	return ObjectEntry{Entry: entry, Object: o.ID, Overrides: c.Overrides}, nil
}

// layerOf reads one layer's bytes from where its source names.
func layerOf(ctx context.Context, o cwmapi.Object, src string, files ObjectFiles) ([]byte, string, error) {
	switch {
	case src == SourceBody:
		return []byte(o.Body), "text/markdown", nil
	case src == SourceFields || strings.HasPrefix(src, SourceFields+"."):
		v, err := pick(o.Fields, strings.TrimPrefix(strings.TrimPrefix(src, SourceFields), "."))
		if err != nil {
			return nil, "", fmt.Errorf("%s: %w", src, err)
		}
		return jsonOf(v)
	case strings.HasPrefix(src, SourceCapabilities+"."):
		capName, key, _ := strings.Cut(strings.TrimPrefix(src, SourceCapabilities+"."), ".")
		if capName == CapContributes {
			return nil, "", fmt.Errorf("%s: a layer never comes from the %s capability itself", src, CapContributes)
		}
		group, ok := o.Capabilities[capName]
		if !ok {
			return nil, "", fmt.Errorf("%s: the object carries no capability %s", src, capName)
		}
		v, err := pick(group, key)
		if err != nil {
			return nil, "", fmt.Errorf("%s: %w", src, err)
		}
		return jsonOf(v)
	case strings.HasPrefix(src, SourceFiles):
		rel := strings.TrimPrefix(src, SourceFiles)
		if !safeModulePath(rel) {
			return nil, "", fmt.Errorf("%s: a layer file is one file inside the object's module (relative; no .. segment; no folder)", src)
		}
		if files == nil {
			return nil, "", fmt.Errorf("%s: no reader of the object's module files", src)
		}
		f, err := files.ObjectFileRead(ctx, cwmapi.ObjectFileReadInput{ID: o.ID, Path: rel})
		if err != nil {
			return nil, "", fmt.Errorf("%s: %w", src, err)
		}
		media := mime.TypeByExtension(path.Ext(rel))
		if media == "" {
			media = "application/octet-stream"
		}
		return f.Content, media, nil
	}
	return nil, "", fmt.Errorf("unknown source %q (body; fields[.<key>]; capabilities.<capability>[.<key>]; files/<path>)", src)
}

// pick reads key of group (the whole group when key is empty).
func pick(group map[string]any, key string) (any, error) {
	if key == "" {
		if len(group) == 0 {
			return nil, fmt.Errorf("empty")
		}
		return group, nil
	}
	v, ok := group[key]
	if !ok {
		return nil, fmt.Errorf("no field %s", key)
	}
	return v, nil
}

func jsonOf(v any) ([]byte, string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, "", err
	}
	return b, "application/json", nil
}

// safeModulePath: one file named inside the module - relative, with no `..`
// or empty segment, no drive or backslash and no trailing slash - so a
// declaration (editable on the object) cannot read outside its module.
func safeModulePath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") || strings.ContainsAny(p, `\:`) {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." || seg == "" {
			return false
		}
	}
	return true
}

// AsOverride is the override entry e publishes (contribapi.OverrideEntry),
// from the project published under its token; trusted says the universe
// trusts that project. Only meaningful when e overrides an entry.
func (e ObjectEntry) AsOverride(trusted bool) contribapi.OverrideEntry {
	return contribapi.OverrideEntry{
		Overrides: e.Overrides,
		Entry:     e.Entry,
		Origin:    contribapi.ProjectOrigin(e.Entry.Provider),
		Object:    e.Object,
		Trusted:   trusted,
	}
}

// Clash is an entry a project serves twice: from a file of its contributions
// folder and from an object, or from two objects. Both are refused.
type Clash struct {
	URI     string
	Sources []string
}

// Finding is the clash as a person reads it, naming both sources.
func (c Clash) Finding() string {
	return fmt.Sprintf("%s is served twice by the project (%s): neither is served", c.URI, strings.Join(c.Sources, " and "))
}

// Clashes names the entries served twice: an object entry whose URI a folder
// entry or another object entry also has. Sorted by URI.
func Clashes(folder []contribapi.Contribution, objects []ObjectEntry) []Clash {
	sources := map[string][]string{}
	for _, f := range folder {
		sources[f.ResolvedURI()] = append(sources[f.ResolvedURI()], "file "+f.ResolvedURI())
	}
	for _, o := range objects {
		sources[o.URI()] = append(sources[o.URI()], "object "+o.Object)
	}
	var out []Clash
	for uri, s := range sources {
		if len(s) > 1 {
			sort.Strings(s)
			out = append(out, Clash{URI: uri, Sources: s})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URI < out[j].URI })
	return out
}
