// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"gopkg.in/yaml.v3"
)

// TestEveryShippedExpressionCompilesInTheObjectVocabulary — the build-time half
// of "an expression is refused at declaration" (p7o). Every `when:` and every
// `${...}` placeholder that a sidecar ships in its CWM type packs is compiled
// against the OBJECT vocabulary, the strictest of the three: a node-era name, a
// typo'd variable or leftover CEL syntax (`exists(i, …)`, `has(…)`) fails here,
// naming the file, instead of shipping an action that is disabled on every
// object with a reason nobody reads.
//
// The runtime half is evalPredicate: an expression that got past this anyway
// disables its action with the compile error as the reason.
func TestEveryShippedExpressionCompilesInTheObjectVocabulary(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..")
	files, err := filepath.Glob(filepath.Join(root, "app", "*", "embed", "cwm", "*", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// Extensions ship packs too (internal/extensions/<ext>/contrib/embed/cwm).
	extFiles, _ := filepath.Glob(filepath.Join(root, "internal", "extensions", "*", "contrib", "embed", "cwm", "*", "*.yaml"))
	files = append(files, extFiles...)
	// The functions the shipped capabilities add (verify(), lint(), …) are in
	// both vocabularies, as they are at run time.
	var functions []string
	for _, f := range files {
		if filepath.Base(filepath.Dir(f)) != "capabilities" {
			continue
		}
		raw, _ := os.ReadFile(f)
		var c struct {
			Functions []struct {
				Name string `yaml:"name"`
			} `yaml:"functions"`
		}
		if yaml.Unmarshal(raw, &c) == nil {
			for _, fn := range c.Functions {
				functions = append(functions, fn.Name)
			}
		}
	}
	if len(functions) < 2 {
		t.Fatalf("found %d shipped functions — verify() and lint() at least", len(functions))
	}
	objectExprs := el.NewEnvCache(el.WithFunctions(el.ObjectVocabulary(), functions))
	transitionExprs := el.NewEnvCache(el.WithFunctions(el.TransitionVocabulary(), functions))
	// A hook sees the hook vocabulary: the object's, plus the event it runs at.
	hookExprs := el.NewEnvCache(el.WithFunctions(el.HookVocabulary(), functions))
	var predicates, placeholders, gates int
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			continue // not this test's concern; the type loaders refuse a bad file
		}
		rel, _ := filepath.Rel(root, f)
		// An object-panel whose content is an EXTENSION passes that
		// extension's input verbatim (resolveRenderDef): its templates are the
		// extension's own — method-panel's ${opt.*} is a click-time toggle —
		// so the object vocabulary is not theirs and they are not checked here.
		if filepath.Base(filepath.Dir(f)) == "object-panels" {
			dropExtensionInput(&doc)
			dropPageConditions(&doc)
		}
		// A data type's view extensions draw page actions: their `when` is the
		// page grammar's (`next_step=freeze`, a bare flag - display's
		// actionShown), evaluated on the row by the page, never by CWM
		// . Their placeholders are still checked.
		if filepath.Base(filepath.Dir(f)) == "data-types" {
			dropActionConditions(&doc, false)
		}
		walkScalars(&doc, "", func(key, value string) {
			// A workflow transition's gate sees the transition vocabulary: the
			// object's, plus by / from / to.
			if (key == "gate" && filepath.Base(filepath.Dir(f)) == "workflows") ||
				(key == "check" && filepath.Base(filepath.Dir(f)) == "gates") {
				gates++
				if _, err := transitionExprs.Compile(value); err != nil {
					t.Errorf("%s: gate %q does not compile in the transition vocabulary: %v", rel, value, err)
				}
				return
			}
			if key == "when" {
				predicates++
				if _, err := objectExprs.Compile(value); err != nil {
					t.Errorf("%s: when %q does not compile in the object vocabulary: %v", rel, value, err)
				}
				return
			}
			for _, inner := range placeholdersIn(value) {
				if render.IsHostTemplate(inner) {
					continue // filled by the page host when the action runs, not by CWM
				}
				placeholders++
				exprs, vocabulary := objectExprs, "object"
				if filepath.Base(filepath.Dir(f)) == "hooks" {
					exprs, vocabulary = hookExprs, "hook"
				}
				if _, err := exprs.Compile(inner); err != nil {
					t.Errorf("%s: placeholder ${%s} does not compile in the %s vocabulary: %v", rel, inner, vocabulary, err)
				}
			}
		})
	}
	// FLOORS: a walk that found nothing proves nothing. The packs ship 3
	// predicates, 61 placeholders and 18 gates.
	if predicates < 3 || placeholders < 20 || gates < 1 {
		t.Fatalf("found %d predicates, %d placeholders and %d gates in %d files — the walk is not reaching the packs", predicates, placeholders, gates, len(files))
	}
	t.Logf("%d predicates, %d placeholders across %d type-pack files", predicates, placeholders, len(files))
}

// dropActionConditions removes the `when` keys under a data type's
// `extensions` (the page actions they draw).
func dropActionConditions(n *yaml.Node, inExtensions bool) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, c := range n.Content {
			dropActionConditions(c, inExtensions)
		}
	case yaml.MappingNode:
		kept := n.Content[:0]
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if inExtensions && k.Value == "when" && v.Kind == yaml.ScalarNode {
				continue
			}
			dropActionConditions(v, inExtensions || k.Value == "extensions")
			kept = append(kept, k, v)
		}
		n.Content = kept
	}
}

// dropPageConditions removes the `when` keys under the content of a panel
// whose content.kind is page: the actions and editors its page draws are
// shown by the page grammar (`can_validate`, `next_step=freeze` - display's
// actionShown, read on the row), never by CWM. The panel's
// filter.when stays: CWM evaluates it on the object.
func dropPageConditions(doc *yaml.Node) {
	if len(doc.Content) == 0 {
		return
	}
	content := mapValue(doc.Content[0], "content")
	if k := mapValue(content, "kind"); k == nil || k.Value != "page" {
		return
	}
	dropActionConditions(content, true)
}

// dropExtensionInput removes content.data from a panel document whose
// content.kind is extension.
func dropExtensionInput(doc *yaml.Node) {
	if len(doc.Content) == 0 {
		return
	}
	content := mapValue(doc.Content[0], "content")
	if content == nil {
		return
	}
	if k := mapValue(content, "kind"); k == nil || k.Value != "extension" {
		return
	}
	for i := 0; i+1 < len(content.Content); i += 2 {
		if content.Content[i].Value == "data" {
			content.Content = append(content.Content[:i], content.Content[i+2:]...)
			return
		}
	}
}

func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func walkScalars(n *yaml.Node, key string, visit func(key, value string)) {
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, c := range n.Content {
			walkScalars(c, key, visit)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			walkScalars(n.Content[i+1], n.Content[i].Value, visit)
		}
	case yaml.ScalarNode:
		visit(key, n.Value)
	}
}

// placeholdersIn returns the inner expression of every `${...}` in s, split the
// way interpolateValue splits it.
func placeholdersIn(s string) []string {
	var out []string
	for {
		i := strings.Index(s, "${")
		if i < 0 {
			return out
		}
		s = s[i+2:]
		j := strings.Index(s, "}")
		if j < 0 {
			return out
		}
		out = append(out, strings.TrimSpace(s[:j]))
		s = s[j+1:]
	}
}
