// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// source_layers.go — one file of a kind, read into its layers as the kind's
// `source:` block says (contribapi.SourceDecl). The one reader of every
// format: one frontmatter splitter, one YAML-to-JSON conversion. Nothing here
// knows what a kind means; what it reads is the kind's declaration.

// SplitFrontmatter separates a file into its YAML frontmatter and its body,
// both trimmed. An absent or unterminated `---` delimiter is an error: guessing
// where the header ended would silently turn part of it into body.
func SplitFrontmatter(content string) (frontmatter, body string, err error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return "", "", fmt.Errorf("missing frontmatter delimiter (---)")
	}
	rest := strings.TrimLeft(content[3:], "\r\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", fmt.Errorf("missing closing frontmatter delimiter (---)")
	}
	return strings.TrimSpace(rest[:idx]), strings.TrimSpace(rest[idx+4:]), nil
}

// readLayer is one layer a file yields: its bytes and the name its media type
// is detected from.
type readLayer struct {
	raw  []byte
	name string
}

// readSource reads one file of a kind with a `source:` block into its data
// layer and its other layers (siblings excepted: the provider reads them
// beside the file). rel names the file, for media types and errors.
func readSource(s *contribapi.SourceDecl, rel string, raw []byte) (json.RawMessage, map[string]readLayer, error) {
	var header, body string
	var tree json.RawMessage
	switch s.Format {
	case contribapi.SourceFrontmatter:
		var err error
		if header, body, err = SplitFrontmatter(string(raw)); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", rel, err)
		}
	case contribapi.SourceStructured, contribapi.SourceDSL:
		var err error
		if tree, err = yamlDocToJSON(rel, raw); err != nil {
			return nil, nil, err
		}
	}

	var data json.RawMessage
	layers := map[string]readLayer{}
	names := make([]string, 0, len(s.Layers))
	for l := range s.Layers {
		names = append(names, l)
	}
	sort.Strings(names)
	for _, l := range names {
		var got readLayer
		switch s.Layers[l].From {
		case contribapi.FromFile:
			got = readLayer{raw: raw, name: rel}
		case contribapi.FromBody:
			got = readLayer{raw: []byte(body), name: rel}
		case contribapi.FromFrontmatter:
			j, err := yamlDocToJSON(rel, []byte(header))
			if err != nil {
				return nil, nil, err
			}
			got = readLayer{raw: j, name: rel + ".json"}
		case contribapi.FromTree:
			got = readLayer{raw: tree, name: rel + ".json"}
		case contribapi.FromPath:
			j, err := json.Marshal(map[string]string{"path": rel})
			if err != nil {
				return nil, nil, err
			}
			got = readLayer{raw: j, name: rel + ".json"}
		default:
			continue // a sibling: read beside the file by the provider
		}
		if l == contribapi.LayerData {
			data = got.raw
			continue
		}
		layers[l] = got
	}

	// Folds: a layer's text also written into the data layer, under a field.
	for _, l := range names {
		fold := s.Layers[l].Fold
		got, ok := layers[l]
		if fold == "" || !ok {
			continue
		}
		folded, err := foldInto(data, fold, string(got.raw))
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", rel, err)
		}
		data = folded
	}
	return data, layers, nil
}

// sourceID is the id a file under the kind's folder publishes as, by the
// kind's `source.id`: basename (the file's name without its extension — a
// dsl file without its `.dsl.<ext>`), dir (the folder holding it), path
// (its path without its extension) or file (its path as it is).
func sourceID(s *contribapi.SourceDecl, rel string) string {
	stripped := rel
	if s.Format == contribapi.SourceDSL {
		if i := strings.Index(path.Base(rel), ".dsl."); i > 0 {
			stripped = path.Join(path.Dir(rel), path.Base(rel)[:i])
		}
	}
	if stripped == rel {
		if ext := path.Ext(rel); ext != "" && ext != path.Base(rel) {
			stripped = strings.TrimSuffix(rel, ext)
		}
	}
	switch s.ID {
	case contribapi.SourceIDDir:
		if d := path.Dir(rel); d != "." {
			return d
		}
		return ""
	case contribapi.SourceIDPath:
		return stripped
	case contribapi.SourceIDFile:
		return rel
	default:
		return path.Base(stripped)
	}
}

// sourceMatches says whether a file is an entry of the kind: its name matches
// one of the kind's globs (any file when it names none).
func sourceMatches(s *contribapi.SourceDecl, rel string) bool {
	if len(s.Match) == 0 {
		return true
	}
	base := path.Base(rel)
	for _, g := range s.Match {
		if ok, _ := path.Match(g, base); ok {
			return true
		}
	}
	return false
}

// sourceSkips says whether a file sits under a folder the kind's source skips
// (a skill's own references, scripts, assets): part of an entry, not one.
func sourceSkips(s *contribapi.SourceDecl, rel string) bool {
	if len(s.Skip) == 0 {
		return false
	}
	dirs := strings.Split(path.Dir(rel), "/")
	for _, d := range dirs {
		for _, skip := range s.Skip {
			if d == skip {
				return true
			}
		}
	}
	return false
}

// dataName is the `name` a data layer gives, "" when it gives none.
func dataName(data json.RawMessage) string {
	var head struct {
		Name string `json:"name"`
	}
	if len(data) == 0 || json.Unmarshal(data, &head) != nil {
		return ""
	}
	return strings.TrimSpace(head.Name)
}

// dataMatches says whether a data layer has every field value where names
// (compared as text); true when where names none.
func dataMatches(data json.RawMessage, where map[string]string) bool {
	if len(where) == 0 {
		return true
	}
	var fields map[string]any
	if len(data) == 0 || json.Unmarshal(data, &fields) != nil {
		return false
	}
	for k, want := range where {
		got, ok := fields[k]
		if !ok || fmt.Sprint(got) != want {
			return false
		}
	}
	return true
}

// foldInto writes text into the data layer under field.
func foldInto(data json.RawMessage, field, text string) (json.RawMessage, error) {
	var fields map[string]any
	if len(data) > 0 {
		if err := json.Unmarshal(data, &fields); err != nil {
			return nil, fmt.Errorf("fold %q: the data layer is not an object: %w", field, err)
		}
	}
	if fields == nil {
		fields = map[string]any{}
	}
	fields[field] = text
	b, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("fold %q: %w", field, err)
	}
	return b, nil
}

// byName says whether an entry's id is read from its data (id: name, or
// name-or-file).
func byName(s *contribapi.SourceDecl) bool {
	return s.ID == contribapi.SourceIDName || s.ID == contribapi.SourceIDNameFile
}
