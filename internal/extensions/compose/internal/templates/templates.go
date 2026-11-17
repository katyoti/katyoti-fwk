// SPDX-License-Identifier: Apache-2.0

// Package templates is ktw-compose's template emission engine: simple,
// strict `{{key}}` substitution over the canonical templates shipped at
// `app/ktw-compose/embed/templates/`.
//
// Deliberately NOT text/template (Q-A3): templates stay copy-pastable as
// standalone example files — a user can read one without learning Go
// template syntax, and a skill can quote one verbatim.
//
// Drift ownership (audit F5): these templates are the single source of
// truth for every scaffolded artifact shape. Any change to a canonical
// pattern (sidecarkit options, frontmatter contracts, …) MUST update the
// matching template in the same commit; the package's integration test
// builds a rendered sidecar to catch silent rot.
package templates

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// keyRe matches a `{{key}}` placeholder: lowercase snake identifiers.
var keyRe = regexp.MustCompile(`\{\{([a-z][a-z0-9_]*)\}\}`)

// Render substitutes every `{{key}}` in content with vars[key].
// STRICT both ways: a placeholder without a var, or remaining unresolved
// after substitution, is an error (silent half-rendered output is how
// template rot hides). Unused vars are allowed.
func Render(content string, vars map[string]string) (string, error) {
	var missing []string
	out := keyRe.ReplaceAllStringFunc(content, func(m string) string {
		key := keyRe.FindStringSubmatch(m)[1]
		v, ok := vars[key]
		if !ok {
			missing = append(missing, key)
			return m
		}
		return v
	})
	if len(missing) > 0 {
		sort.Strings(missing)
		return "", fmt.Errorf("templates: unresolved placeholders: %s", strings.Join(dedupe(missing), ", "))
	}
	return out, nil
}

// RenderFS loads a template by path from fsys and renders it.
func RenderFS(fsys fs.FS, path string, vars map[string]string) (string, error) {
	raw, err := fs.ReadFile(fsys, path)
	if err != nil {
		return "", fmt.Errorf("templates: read %s: %w", path, err)
	}
	return Render(string(raw), vars)
}

// RenderFSToFile renders a template from fsys and writes the result to
// outPath (creating parent directories). The conventional `.tmpl`
// suffix is the template's, not the output's — callers pass the real
// output filename.
func RenderFSToFile(fsys fs.FS, path string, vars map[string]string, outPath string) error {
	out, err := RenderFS(fsys, path, vars)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(outPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("templates: create dir for %s: %w", outPath, err)
		}
	}
	if err := os.WriteFile(outPath, []byte(out), 0o644); err != nil {
		return fmt.Errorf("templates: write %s: %w", outPath, err)
	}
	return nil
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
