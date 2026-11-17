// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A RELATION IS AN EDGE. A field never holds an object id: a relation is an
// edge of a declared kind, so CWM validates it, refuses it dangling, shows it
// in Links and rewrites it on a rename. A field that only MENTIONS an object
// — prose, a record of what happened — stays text, and says so here, with the
// reason. Every shipped data-type field whose name or description marks an
// object reference is either gone or listed below.
var textOnPurpose = map[string]string{
	// An inline criterion of an ancestor gate: a record inside an object,
	// not an object.
	"gate.overrides": "names an inline criterion, not an object",
	// Who answered a question: a person's handle or an agent's id - an actor,
	// never an object.
	"answer.by_id": "names the actor who answered - a person's handle or an agent's id - not an object",
	// Where a published document came from: a document tree is the payload
	// handed to a publisher, never stored as an object, so its fields join
	// nothing.
	"document-tree.source": "a publish payload's provenance - a file path or an object id named - not an object's relation",
	// A pack published after the open-source build adds its own entries from a
	// development-only file (relations_are_edges_private_test.go).
}

// notObjectIDs end in _id and name no object in any data-type: a spawn's own
// id, a runtime task's id. A field name that is an id elsewhere only in one
// data-type goes in textOnPurpose, keyed by that field.
var notObjectIDs = map[string]bool{"spawn_id": true, "task_id": true}

// objectRef marks a field that holds object ids: its description says so, or
// its name is an id of a thing.
var objectRef = regexp.MustCompile(`(?i)\b((object|decision|criteri(on|a)|use-case|item|goal|acceptance-criterion|target)[ -]ids?|id of the object|ids of the (objects?|criteria|items?)|OD-NNN|UUIDs? of items|module-qualified|body ids?)\b`)

func TestNoFieldHoldsAnObjectID(t *testing.T) {
	root := repoRootForTypes(t)
	var files []string
	for _, pattern := range []string{"app/*/embed", "internal/extensions/*/contrib/embed"} {
		dirs, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		for _, d := range dirs {
			_ = filepath.WalkDir(d, func(p string, e os.DirEntry, err error) error {
				if err == nil && !e.IsDir() && strings.Contains(filepath.ToSlash(p), "/data-types/") && strings.HasSuffix(p, ".yaml") {
					files = append(files, p)
				}
				return nil
			})
		}
	}
	if len(files) < 40 {
		t.Fatalf("only %d data-types found — the scan is not working", len(files))
	}
	seen := map[string]bool{}
	var bad []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Name  string    `yaml:"name"`
			Types yaml.Node `yaml:"types"`
		}
		if yaml.Unmarshal(b, &doc) != nil || doc.Name == "" {
			continue
		}
		for _, fld := range dataTypeFields(&doc.Types) {
			key := doc.Name + "." + fld.name
			ref := objectRef.MatchString(fld.description) ||
				strings.HasSuffix(fld.name, "_id") && !notObjectIDs[fld.name]
			if !ref {
				continue
			}
			seen[key] = true
			if _, ok := textOnPurpose[key]; !ok {
				rel, _ := filepath.Rel(root, f)
				bad = append(bad, key+" ("+filepath.ToSlash(rel)+"): "+fld.description)
			}
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("a field holds an object id — make it an edge of a declared kind, or list it as text on purpose with the reason:\n  %s", strings.Join(bad, "\n  "))
	}
	for key := range textOnPurpose {
		if !seen[key] {
			t.Errorf("textOnPurpose lists %s, which no data-type declares as a reference any more — remove it", key)
		}
	}
}

type dataTypeField struct{ name, description string }

// dataTypeFields walks a data-type's `types:` and returns every field with its
// description, at any depth.
func dataTypeFields(n *yaml.Node) []dataTypeField {
	var out []dataTypeField
	if n == nil || n.Kind != yaml.MappingNode {
		for _, c := range n.Content {
			out = append(out, dataTypeFields(c)...)
		}
		return out
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Value == "fields" && v.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(v.Content); j += 2 {
				name, def := v.Content[j].Value, v.Content[j+1]
				desc := ""
				if def.Kind == yaml.MappingNode {
					for m := 0; m+1 < len(def.Content); m += 2 {
						if def.Content[m].Value == "description" || def.Content[m].Value == "label" {
							desc += " " + def.Content[m+1].Value
						}
					}
				}
				out = append(out, dataTypeField{name: name, description: strings.TrimSpace(desc)})
			}
		}
		out = append(out, dataTypeFields(v)...)
	}
	return out
}
