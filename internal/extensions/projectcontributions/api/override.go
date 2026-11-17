// SPDX-License-Identifier: Apache-2.0

package api

import (
	"mime"
	"path"
	"regexp"
	"sort"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// An override object is an object of a module carrying `contributes` with
// `overrides`: the entry it overrides and the layers it replaces, each a file
// of its module. Its shape is built here once, for
// every writer - the migration that retires a project's overwrites trees, and
// the authoring verbs that make one.

// ConvertedOverride is one override object: the entry it overrides, and each
// layer it replaces as a file of the object's module.
type ConvertedOverride struct {
	// Overrides is the overridden entry's URI (provider:/kind/id).
	Overrides string
	// Kind and ID are the entry's (the object publishes kind; id its own).
	Kind, ID string
	// Layers are the `contributes` layers: layer name to `files/<path>`.
	Layers map[string]string
	// Files are the layer files to write in the object's module, by path
	// relative to the module.
	Files map[string][]byte
	// From are the files the entry read, relative to the tree they came from.
	From []string
}

// OverrideObjectType is the type of an override object: a document, its
// `contributes` capability saying what it overrides (any object may carry it).
const OverrideObjectType = "document"

var notIDChar = regexp.MustCompile(`[^a-z0-9]+`)

// OverrideObjectID is the override object's id in its module: the provider,
// the kind and the entry's id, in kebab case.
func OverrideObjectID(provider, kind, id string) string {
	s := strings.Trim(notIDChar.ReplaceAllString(strings.ToLower(provider+"-"+kind+"-"+id), "-"), "-")
	return "override-" + s
}

// OverrideObject is the override object of o, an entry of provider: its id,
// type, name, description, title and `contributes` group.
func OverrideObject(provider string, o ConvertedOverride) cwmapi.Object {
	layers := map[string]any{}
	for name, src := range o.Layers {
		layers[name] = src
	}
	return cwmapi.Object{ID: OverrideObjectID(provider, o.Kind, o.ID), Type: OverrideObjectType, Name: o.Overrides,
		Description: "Overrides " + o.Overrides + " for this project: the layers it carries replace the entry's, the others are inherited.",
		Fields:      map[string]any{"title": o.Overrides},
		Capabilities: map[string]map[string]any{CapContributes: {
			"kind": o.Kind, "id": o.ID, "overrides": o.Overrides, "layers": layers}}}
}

// OverrideLayerFile is where an override object keeps one layer, relative to
// its module: <filesRoot>/<kind>/<id>/<layer><ext>, the extension that of the
// layer's media type so the layer is read back with it.
func OverrideLayerFile(filesRoot, kind, id, layer, mediaType string) string {
	return path.Join(filesRoot, kind, id, layer+extensionOf(mediaType))
}

// extensionOf is the file extension of a layer's media type.
func extensionOf(media string) string {
	media, _, _ = strings.Cut(media, ";")
	switch strings.TrimSpace(media) {
	case "text/markdown":
		return ".md"
	case "application/json":
		return ".json"
	case "application/yaml", "text/yaml", "application/x-yaml":
		return ".yaml"
	case "text/plain":
		return ".txt"
	}
	if exts, err := mime.ExtensionsByType(media); err == nil && len(exts) > 0 {
		sort.Strings(exts)
		return exts[0]
	}
	return ""
}
