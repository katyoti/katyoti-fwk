// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// RUNTIME ROWS ARE NAMED ONE WAY: a to-do names the CWM
// objects it is about, never the reverse. No edge, no parent, no field of a
// CWM object holds a to-do's id - except the named provenance fields below,
// each read tolerating a row that is gone (a to-do is a transient ticket: its
// row ends, is pruned, or moves into another store).

// todoIDRE is a to-do id, the whole of a value: <module>.TD-<stamp>-<suffix>.
var todoIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.TD-\d{8}-\d+-[0-9a-f]{4}$`)

// todoRefAllowed are the fields that may carry a to-do id, as their path of
// keys (a list's items add no key): provenance, never a link.
var todoRefAllowed = map[string]string{
	"fields.task":                  "an inbox answer: the to-do it answered",
	"capabilities.plan.stale.todo": "a plan's stale step: the to-do it withdrew",
}

// todoRefsOutsideAllowlist is every value under root's module objects that is
// a to-do id, in a field the allowlist does not name. The rows themselves
// (`.ktw/todo/`, `.ktw/done/`) are not content.
func todoRefsOutsideAllowlist(t *testing.T, root string) []string {
	t.Helper()
	var problems []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); n == "todo" || n == "done" || n == "waiting" {
				if filepath.Base(filepath.Dir(p)) == ".ktw" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(p, ".cwm.yaml") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil || !bytes.Contains(b, []byte(".TD-")) {
			return nil
		}
		var doc map[string]any
		if err := config.DecodeBytes(b, marshal.YAML, &doc); err != nil {
			return nil
		}
		walkTodoRefs(doc, "", func(path, value string) {
			if _, ok := todoRefAllowed[path]; !ok {
				problems = append(problems, fmt.Sprintf("%s: %s = %s", rel(root, p), path, value))
			}
		})
		return nil
	})
	sort.Strings(problems)
	return problems
}

func walkTodoRefs(v any, path string, found func(path, value string)) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			next := k
			if path != "" {
				next = path + "." + k
			}
			walkTodoRefs(e, next, found)
		}
	case []any:
		for _, e := range x {
			walkTodoRefs(e, path, found)
		}
	case string:
		if todoIDRE.MatchString(strings.TrimSpace(x)) {
			found(path, x)
		}
	}
}

// TestNoObjectNamesARuntimeRow: no module object names a to-do outside the
// provenance allowlist - an edge, a parent or any other field naming one is
// refused. The guard is falsified on a planted edge.
func TestNoObjectNamesARuntimeRow(t *testing.T) {
	planted := t.TempDir()
	obj := "id: billing.F-001\ntype: feature\nedges:\n  - kind: depends-on\n    to: billing.TD-20261104-080654150343-9369\n" +
		"capabilities:\n  plan:\n    stale:\n      - todo: billing.TD-20261104-080654150343-9370\n"
	if err := os.WriteFile(filepath.Join(planted, "F-001.cwm.yaml"), []byte(obj), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := todoRefsOutsideAllowlist(t, planted); len(got) != 1 || !strings.Contains(got[0], "edges.to") {
		t.Fatalf("the planted edge: %v; want it found, and the stale step allowed", got)
	}

	if got := todoRefsOutsideAllowlist(t, filepath.Join(repoRoot(t), "modules")); len(got) > 0 {
		t.Errorf("module objects name to-dos outside the provenance fields (%v):\n  %s", todoRefAllowed, strings.Join(got, "\n  "))
	}
}
