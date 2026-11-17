// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// notTools are `cwm-*` words that name something other than a tool — each
// with what it is, so the list cannot quietly become a place to hide a dead
// tool name.
var notTools = map[string]string{
	"cwm_object_context": "the prompt block the cwm plugin injects for the object a chat starts on",
}

// cwmWord is a cwm tool word: a kebab `cwm-object-…` / `cwm-edge-…` name, or
// any snake `cwm_…` word — tool names are kebab, so a snake one is dead
// unless notTools names it.
var cwmWord = regexp.MustCompile(`\bcwm_[a-z_]+|\bcwm-(?:object|edge)-[a-z-]*`)

// TestEveryCitedCWMToolIsRegistered: a skill, task, persona or permission
// profile that names a `cwm-*` tool names one an agent can actually call.
// Skills cited `cwm_search_vector`, `cwm_node_tree`, `cwm_effective_rules`… —
// tools that never existed — and nothing noticed, because nothing checked.
// A word ending in `-` is a prefix pattern (a permission glob) and must match
// at least one registered tool.
func TestEveryCitedCWMToolIsRegistered(t *testing.T) {
	registered := map[string]bool{}
	for _, tool := range CWMTools() {
		registered[tool.ToolDef().Name] = true
	}

	root := filepath.Join("..", "..", "..", "..", "..")
	embeds, _ := filepath.Glob(filepath.Join(root, "app", "*", "embed"))
	if len(embeds) < 5 {
		t.Fatalf("only %d embed trees found from %s — the scan is not working", len(embeds), root)
	}
	dead := map[string][]string{}
	scanned := 0
	for _, e := range embeds {
		_ = filepath.WalkDir(e, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			switch filepath.Ext(p) {
			case ".md", ".yaml", ".yml", ".json":
			default:
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			scanned++
			for _, w := range cwmWord.FindAllString(string(b), -1) {
				if registered[w] || notTools[w] != "" {
					continue
				}
				if strings.HasSuffix(w, "-") && prefixMatches(registered, w) {
					continue
				}
				rel, _ := filepath.Rel(root, p)
				dead[w] = append(dead[w], filepath.ToSlash(rel))
			}
			return nil
		})
	}
	if scanned < 100 {
		t.Fatalf("only %d files scanned — the scan is not working", scanned)
	}
	if len(dead) > 0 {
		var lines []string
		for w, files := range dead {
			lines = append(lines, w+" ← "+strings.Join(uniq(files), ", "))
		}
		sort.Strings(lines)
		t.Fatalf("cited but not registered as a cwm tool:\n  %s", strings.Join(lines, "\n  "))
	}
}

func prefixMatches(registered map[string]bool, prefix string) bool {
	for name := range registered {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
