// SPDX-License-Identifier: Apache-2.0

package ask_user

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// structuredAsk is a shipped text telling an agent to ask a select, a confirm
// or a form through ask-user.
var structuredAsk = regexp.MustCompile("(?i)ask-user(\\W{0,6}type:?\\s*\"?|\\W{0,4}with type \"|` )(confirm|select|form)\\b")

// TestNoPackAsksAValidationThroughAskUserAlone: every shipped
// task or skill that tells an agent to ask a select, a confirm or a form
// through ask-user asks it with request-validation when it is offered - the
// line says both; ask-user alone would be refused once IN-039's switch is on.
func TestNoPackAsksAValidationThroughAskUserAlone(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "app")
	var bad []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		slash := filepath.ToSlash(p)
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") || !strings.Contains(slash, "/embed/") {
			return nil
		}
		if strings.Contains(slash, "/references/") {
			return nil // rubrics quoting past runs, not instructions
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if structuredAsk.MatchString(line) && !strings.Contains(line, "request-validation") {
				rel, _ := filepath.Rel(root, p)
				bad = append(bad, filepath.ToSlash(rel)+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	for _, b := range bad {
		t.Errorf("asks a validation through ask-user alone: %s", b)
	}
}
