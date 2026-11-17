// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Basic, artifact-AGNOSTIC linters — methods owns these so any node type can
// compose them (a ktw-compose `skill` node lists `file-exists`, `markdown-frontmatter`,
// `frontmatter-keys`, ... alongside its own `skill-format`). Each is a pure
// `input -> LintResult`; the lint service wraps them as runnable methods.

// LintFileExists: the target file exists.
func LintFileExists(path string) LintResult {
	if strings.TrimSpace(path) == "" {
		return ErrorResult("no-path", "file-exists: empty path")
	}
	if _, err := os.Stat(path); err != nil {
		return Result(LintFinding{"file-missing", "no file at " + path, SeverityError})
	}
	return Result()
}

// LintMarkdownFrontmatter: the file is well-structured markdown — it opens with
// a `---` YAML frontmatter block that parses, and has a body after it.
func LintMarkdownFrontmatter(path string) LintResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return ErrorResult("unreadable", "markdown-frontmatter: cannot read "+path)
	}
	fm, body, ok := SplitFrontmatter(string(data))
	if !ok {
		return Result(LintFinding{"frontmatter-missing", "no opening `---` YAML frontmatter block", SeverityError})
	}
	var m map[string]any
	if err := yaml.Unmarshal([]byte(fm), &m); err != nil || m == nil {
		return Result(LintFinding{"frontmatter-invalid", "frontmatter is not valid YAML", SeverityError})
	}
	if strings.TrimSpace(body) == "" {
		return Result(LintFinding{"body-empty", "no markdown body after the frontmatter", SeverityError})
	}
	return Result()
}

// LintFrontmatterKeys: every required key is present in the frontmatter. The
// required set is the linter's input (e.g. `{keys: [name, description]}`), so the
// node type passes its type's required keys.
func LintFrontmatterKeys(path string, keys []string) LintResult {
	m, lr := frontmatterMap(path, "frontmatter-keys")
	if m == nil {
		return lr
	}
	var findings []LintFinding
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			findings = append(findings, LintFinding{"frontmatter-key-missing", "required frontmatter key missing: " + k, SeverityError})
		}
	}
	return Result(findings...)
}

// LintNonEmpty: the file has non-whitespace content.
func LintNonEmpty(path string) LintResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return ErrorResult("unreadable", "non-empty: cannot read "+path)
	}
	if strings.TrimSpace(string(data)) == "" {
		return Result(LintFinding{"empty", "file is empty", SeverityError})
	}
	return Result()
}

// ── helpers ──────────────────────────────────────────────────────────────

// SplitFrontmatter returns the YAML frontmatter + the body when content opens
// with a `---\n ... \n---` block. Exported so artifact-specific linters in
// other packs (e.g. ktw-compose's skill-format) split the same way.
func SplitFrontmatter(content string) (fm, body string, ok bool) {
	s := content
	s = strings.TrimLeft(s, " \t\r\n")
	if !strings.HasPrefix(s, "---") {
		return "", "", false
	}
	rest := s[3:]
	if i := strings.Index(rest, "\n"); i >= 0 {
		rest = rest[i+1:]
	} else {
		return "", "", false
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", false
	}
	fm = rest[:end]
	body = rest[end+4:]
	if j := strings.Index(body, "\n"); j >= 0 {
		body = body[j+1:]
	}
	return fm, body, true
}

func frontmatterMap(path, rule string) (map[string]any, LintResult) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrorResult("unreadable", rule+": cannot read "+path)
	}
	fm, _, ok := SplitFrontmatter(string(data))
	if !ok {
		return nil, Result(LintFinding{"frontmatter-missing", "no `---` YAML frontmatter block", SeverityError})
	}
	var m map[string]any
	if err := yaml.Unmarshal([]byte(fm), &m); err != nil || m == nil {
		return nil, Result(LintFinding{"frontmatter-invalid", "frontmatter is not valid YAML", SeverityError})
	}
	return m, LintResult{}
}

// LintValueIn: a value is one of the allowed ones — the declarative way a type
// fixes a field to a set (a `goal` is always asymptotic): the type's `lint`
// hook passes the field (`${object.want.reachability}`) as value. An empty
// value reads as def (the field's schema default) before it is checked. The
// finding carries rule and message as given, at error severity.
func LintValueIn(value, def string, allowed []string, rule, message string) LintResult {
	if len(allowed) == 0 {
		return ErrorResult("no-allowed-values", "value-in: no allowed value given")
	}
	v := strings.TrimSpace(value)
	if v == "" {
		v = def
	}
	for _, a := range allowed {
		if v == a {
			return Result()
		}
	}
	if rule == "" {
		rule = "value-not-allowed"
	}
	shown := v
	if shown == "" {
		shown = "(empty)"
	}
	msg := "value " + shown + " is not one of " + strings.Join(allowed, ", ")
	if message != "" {
		msg = message + " (" + msg + ")"
	}
	return Result(LintFinding{rule, msg, SeverityError})
}
