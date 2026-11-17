// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// personKindRE is a bare test of an actor's kind against a person: what iam's
// PersonAct (an act only a person may do, by its permission code) and IsPerson
// (who acted) answer instead.
var personKindRE = regexp.MustCompile(`[Kk]ind\s*(==|!=)\s*("person"|cause\.KindPerson)`)

// personKindLeft are the bare person-kind tests still standing, by file, each
// with how many and the feature of IN-039 that converts it. The count may only
// go down: a new test, or one more in a file, fails; once a file has none it
// leaves this list. (the deletion) ends it empty.
var personKindLeft = map[string]struct {
	n   int
	why string
}{
	"internal/extensions/scheduler/internal/core/service.go":              {8, "a person's to-do and report - answerable_by, a person's step"},
	"internal/extensions/scheduler/internal/core/runner.go":               {1, "a person's step"},
	"internal/extensions/scheduler/internal/core/groups.go":               {1, "holding-list members"},
	"internal/extensions/planification/internal/core/service.go":          {5, "a person's step"},
	"internal/extensions/planification/internal/core/delivery.go":         {3, "a person's step"},
	"internal/extensions/planification/internal/core/ends_on.go":          {2, "a person's step"},
	"internal/extensions/planification/internal/core/actualize.go":        {1, "a person's step"},
	"internal/extensions/planification/internal/core/collect.go":          {1, "satisfies, a person's step"},
	"internal/extensions/planification/internal/core/demand.go":           {1, "a person's step"},
	"internal/extensions/planification/internal/core/gates.go":            {1, "a person's gate"},
	"internal/extensions/planification/internal/core/handtest.go":         {1, "a test by hand is one card"},
	"internal/extensions/planification/internal/core/hold.go":             {1, "a person's hold"},
	"internal/extensions/planification/internal/core/increment_accept.go": {1, "a person's step"},
	"internal/extensions/planification/internal/core/module_update.go":    {1, "a person's step"},
	"internal/extensions/planification/internal/core/release_gates.go":    {1, "a person's gate"},
	"internal/extensions/planification/internal/core/revise.go":           {1, "a person's step"},
	"internal/extensions/planification/internal/core/views.go":            {1, "a person's hold"},
	"internal/extensions/decisions/internal/core/evaluation.go":           {1, "a decide step"},
	"internal/extensions/inbox/internal/core/service.go":                  {1, "answerable_by, the person-kind deletion"},
}

// TestNoNewBarePersonKindTest: no check reads "is a person" on its
// own - outside iam (internal/extensions/profile), an act only a person may do
// asks profileapi.PersonAct with its permission code, and who acted is read by
// profileapi.IsPerson. The tests still standing are listed with the feature
// converting them, and their number only goes down.
func TestNoNewBarePersonKindTest(t *testing.T) {
	root := repoRoot(t)
	iam := filepath.Join(root, "internal", "extensions", "profile") + string(os.PathSeparator)
	found := map[string][]string{}
	for _, top := range []string{"app", "internal", "tools"} {
		_ = filepath.WalkDir(filepath.Join(root, top), func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if n := d.Name(); n == "node_modules" || n == "testdata" || n == "frontend" {
					return filepath.SkipDir
				}
				if strings.HasPrefix(p+string(os.PathSeparator), iam) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			for i, line := range strings.Split(string(b), "\n") {
				code := strings.TrimSpace(line)
				if strings.HasPrefix(code, "//") || !personKindRE.MatchString(code) {
					continue
				}
				f := rel(root, p)
				found[f] = append(found[f], strconv.Itoa(i+1)+": "+code)
			}
			return nil
		})
	}
	var problems []string
	for f, lines := range found {
		left, listed := personKindLeft[f]
		if !listed || len(lines) > left.n {
			problems = append(problems, f+" ("+strconv.Itoa(len(lines))+", at most "+strconv.Itoa(left.n)+"):\n    "+strings.Join(lines, "\n    "))
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Errorf("an act only a person may do asks profileapi.PersonAct with its permission code, and who acted is read by profileapi.IsPerson - never a bare person-kind test:\n  %s", strings.Join(problems, "\n  "))
	}
	for f, left := range personKindLeft {
		if n := len(found[f]); n < left.n {
			t.Logf("%s has %d bare person-kind tests left, fewer than its %d: lower its count (%s)", f, n, left.n, left.why)
		}
	}
}
