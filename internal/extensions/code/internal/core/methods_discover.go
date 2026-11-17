// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
)

// ModuleDiscover is the methodology's pack-dispatch verb (Track B #4
// phase 1 — the FIRST CodeService verb routed through a LanguagePack):
// resolve the language (explicit > extension inference, Q-B4), look up
// the registered pack instance, delegate the scan, return its
// proposals.
//
// Phase 1 is propose-only: Output.ModuleID echoes TargetModuleID but
// nothing is committed to the substrate — the commit half (module
// node + facet writes) lands with the cm-task that owns module CRUD.
func (s *codeService) ModuleDiscover(ctx stdctx.Context, in api.ModuleDiscoverInput) (api.ModuleDiscoverOutput, error) {
	if in.Path == "" {
		return api.ModuleDiscoverOutput{}, fmt.Errorf("%w: ModuleDiscover: path is required", api.ErrInvalidInput)
	}

	lang := in.Language
	if lang == "" {
		inferred, err := inferLanguage(in.Path)
		if err != nil {
			return api.ModuleDiscoverOutput{}, fmt.Errorf("ModuleDiscover: %w", err)
		}
		lang = inferred
	}

	pack, err := application.UseNamedService[api.LanguagePack](ctx, lang)
	if err != nil {
		return api.ModuleDiscoverOutput{}, fmt.Errorf("ModuleDiscover: no LanguagePack registered for %q: %w", lang, err)
	}

	disc, err := pack.Discover(ctx, api.DiscoverInput{Path: in.Path, DryRun: in.DryRun})
	if err != nil {
		return api.ModuleDiscoverOutput{}, fmt.Errorf("ModuleDiscover: pack %q: %w", lang, err)
	}

	return api.ModuleDiscoverOutput{
		ModuleID:  in.TargetModuleID,
		Discovery: disc,
	}, nil
}

// languageByExtension is the v1 inference table (Q-B4). Grows as
// packs ship; ties broken by file count.
var languageByExtension = map[string]string{
	".go": "go",
}

// inferLanguage scans the tree (bounded) and picks the language with
// the most matching files. Errors when nothing matches — the caller
// then passes Language explicitly.
func inferLanguage(root string) (string, error) {
	counts := map[string]int{}
	seen := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if lang, ok := languageByExtension[filepath.Ext(d.Name())]; ok {
			counts[lang]++
		}
		seen++
		if seen > 20000 {
			return filepath.SkipAll
		}
		return nil
	})
	best, bestN := "", 0
	for lang, n := range counts {
		if n > bestN {
			best, bestN = lang, n
		}
	}
	if best == "" {
		return "", fmt.Errorf("%w: cannot infer language at %s (no known extensions); pass language explicitly",
			api.ErrInvalidInput, root)
	}
	if _, err := os.Stat(root); err != nil {
		return "", fmt.Errorf("%w: path %s: %v", api.ErrInvalidInput, root, err)
	}
	return best, nil
}
