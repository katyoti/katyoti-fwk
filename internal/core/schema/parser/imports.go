// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveImports recursively resolves the schema's imports, merging imported
// types into the parent's Types dict. Cycles are detected and reported with
// the full path. Type-name conflicts are errors (no silent override).
//
// Replaces the S3 stub.
func resolveImports(raw *rawSchema) error {
	if len(raw.Imports) == 0 {
		return nil
	}
	visited := map[string]bool{}
	if raw.sourcePath != "" {
		visited[normalizeImportKey(raw.sourcePath)] = true
	}
	return resolveInto(raw, visited, []string{raw.sourcePath})
}

// resolveInto walks the imports of `raw` and merges each into raw.Types.
// `visited` tracks absolute paths already pulled in (cycle detection).
// `chain` is the call stack of paths for error messages.
func resolveInto(raw *rawSchema, visited map[string]bool, chain []string) error {
	for _, imp := range raw.Imports {
		abs, err := resolveImportPath(raw.basePath, imp)
		if err != nil {
			return err
		}
		key := normalizeImportKey(abs)
		if visited[key] {
			cyclePath := append(append([]string(nil), chain...), abs)
			return fmt.Errorf("import cycle detected: %s", strings.Join(cleanChain(cyclePath), " → "))
		}
		visited[key] = true

		body, err := os.ReadFile(abs)
		if err != nil {
			return fmt.Errorf("read import %q: %w", imp, err)
		}
		child, err := decodeOuter(body, formatFromExt(abs))
		if err != nil {
			return fmt.Errorf("parse import %s: %w", abs, err)
		}
		child.basePath = filepath.Dir(abs)
		child.sourcePath = abs

		// Recurse first so nested imports are resolved before we merge child's
		// types upward. Conflicts encountered downstream are caught by mergeTypes.
		if err := resolveInto(child, visited, append(chain, abs)); err != nil {
			return err
		}
		if err := mergeTypes(raw, child); err != nil {
			return err
		}
	}
	// imports list is now fully merged into Types; clear it so downstream
	// stages don't reprocess.
	raw.Imports = nil
	return nil
}

// mergeTypes copies child's Types into parent's, erroring on any name collision.
func mergeTypes(parent, child *rawSchema) error {
	if parent.Types == nil {
		parent.Types = make(map[string]any, len(child.Types))
	}
	for name, def := range child.Types {
		if _, exists := parent.Types[name]; exists {
			return fmt.Errorf(
				"import %s: type %q conflicts with existing definition (rename one or scope under a namespace)",
				child.sourcePath, name,
			)
		}
		parent.Types[name] = def
	}
	return nil
}

// resolveImportPath resolves a (possibly relative) import path against a base
// directory and returns the absolute, cleaned path.
func resolveImportPath(baseDir, imp string) (string, error) {
	if imp == "" {
		return "", fmt.Errorf("empty import path")
	}
	imp = filepath.FromSlash(imp)
	if filepath.IsAbs(imp) {
		return filepath.Clean(imp), nil
	}
	if baseDir == "" {
		// Relative import without a base: resolve against CWD as a last resort.
		// Callers should set basePath explicitly via ParseFile.
		abs, err := filepath.Abs(imp)
		if err != nil {
			return "", fmt.Errorf("resolve import %q: %w", imp, err)
		}
		return abs, nil
	}
	abs, err := filepath.Abs(filepath.Join(baseDir, imp))
	if err != nil {
		return "", fmt.Errorf("resolve import %q: %w", imp, err)
	}
	return abs, nil
}

// normalizeImportKey produces the canonical form used for cycle detection.
// Case-insensitive on Windows (where path comparisons are case-insensitive),
// case-sensitive elsewhere.
func normalizeImportKey(abs string) string {
	abs = filepath.Clean(abs)
	abs = filepath.ToSlash(abs)
	if isCaseInsensitiveFS() {
		abs = strings.ToLower(abs)
	}
	return abs
}

func isCaseInsensitiveFS() bool {
	// runtime.GOOS would do, but we want a lightweight check that doesn't
	// depend on the runtime package. Filename of the binary won't tell us;
	// use a path probe heuristic via filepath.Separator.
	return filepath.Separator == '\\'
}

// cleanChain drops empty entries (e.g. when the entry-point schema was parsed
// from in-memory bytes and has no source path) for tidier error messages.
func cleanChain(chain []string) []string {
	out := chain[:0]
	for _, p := range chain {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
