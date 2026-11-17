// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	contentlint "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/lint"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
)

// LintSkill is compose's artifact-SPECIFIC linter method — the only lint
// surface compose owns. The generic basics live in the methods LinterService.
// ActionService routes `ktw-compose:compose/lint-skill` here and
// object-verify folds the returned findings into its report.

// lintPath resolves a relative lint target against the workspace root, so a
// node's workspace-relative `skill_file` resolves the same way object-verify's
// files probe does. Absolute paths pass through unchanged.
func (s *Service) lintPath(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	root := s.repoRoot
	if root == "" {
		root = application.Env().WorkspaceDir()
	}
	if root == "" {
		return p
	}
	return filepath.Join(root, p)
}

func (s *Service) LintSkill(ctx context.Context, in api.LintSkillInput) (verify.LintResult, error) {
	res, err := s.lintArtifact(ctx, mbapi.ResourceTypeSkill, in.Ref())
	if err != nil {
		return verify.LintResult{}, err
	}
	// No served body → format-only (the object-type verify path). When the host
	// passes the served (in-binary) body, ALSO run the embedded-freshness check
	// — a stale sidecar is caught deterministically before a test scores the
	// wrong version. The catalog read is the host's job; this is the files-only
	// COMPARE, so a files-only compose binary needs no catalog client of its own.
	if strings.TrimSpace(in.Served) == "" {
		return res, nil
	}
	// The freshness compare asks "is the served body the LATEST source?". When an
	// active workspace overlay shadows this skill (allow_overwrites on + an
	// overlay twin exists), the overlay IS the latest — the served body is
	// EXPECTED to be the overlay, ahead of the embedded source, pending promote.
	// Compare against the overlay then, so overlay iteration doesn't false-flag
	// stale-embedded-skill. With no overlay, compare against the source file (the
	// genuine stale-build check).
	source := s.lintPath(in.File)
	compareFile, overlayActive := source, false
	if tw := s.overlayTwin(providerOf(in.Skill), source); tw != "" {
		compareFile, overlayActive = tw, true
	}
	var fresh verify.LintResult
	if data, err := os.ReadFile(compareFile); err != nil {
		fresh = verify.ErrorResult("source-file-unreadable", "embedded-skill-fresh: cannot read source file "+compareFile)
	} else {
		fresh = CompareEmbeddedFresh(providerOf(in.Skill), in.Served, string(data))
		if overlayActive && len(fresh.Findings) == 0 {
			// Served matches the live overlay — testing the latest, not stale.
			fresh = verify.Result(verify.LintFinding{
				Rule:     "served-from-overlay",
				Message:  "served body matches the active workspace overlay (not yet promoted) — you are testing the live overlay version; no rebuild needed, promote when validated",
				Severity: verify.SeverityInfo,
			})
		}
	}
	return verify.Result(append(res.Findings, fresh.Findings...)...), nil
}

// overlayTwin returns the overlay file that mirrors a source file's embed tail
// for `provider`, when the universe gate is on and the overlay copy exists.
// Empty when no active overlay shadows this skill.
func (s *Service) overlayTwin(provider, sourcePath string) string {
	if provider == "" || sourcePath == "" {
		return ""
	}
	if !cenv.AllowOverwrites(s.home()) {
		return ""
	}
	tail := embedTail(sourcePath)
	if tail == "" {
		return ""
	}
	f := filepath.Join(cenv.OverwritesDir(s.home(), provider), filepath.FromSlash(tail))
	if _, err := os.Stat(f); err != nil {
		return ""
	}
	return f
}

// embedTail returns the `embed/…` suffix of a path (the part that mirrors under
// an overlay tree), or "" when the path has no embed/ segment.
func embedTail(p string) string {
	p = filepath.ToSlash(p)
	if i := strings.Index(p, "embed/"); i >= 0 {
		return p[i:]
	}
	return ""
}

func (s *Service) LintTask(ctx context.Context, in api.LintTaskInput) (verify.LintResult, error) {
	return s.lintArtifact(ctx, mbapi.ResourceTypeTask, in.Ref())
}

func (s *Service) LintReference(ctx context.Context, in api.LintReferenceInput) (verify.LintResult, error) {
	return s.lintArtifact(ctx, "reference", in.Ref())
}

func (s *Service) LintPersona(ctx context.Context, in api.LintPersonaInput) (verify.LintResult, error) {
	return s.lintArtifact(ctx, "persona", in.Ref())
}

func (s *Service) LintGuideRecipe(ctx context.Context, in api.LintGuideRecipeInput) (verify.LintResult, error) {
	return s.lintArtifact(ctx, guideapi.Kind, in.Ref())
}

// lintArtifact is the one body behind every per-type verb: a file on disk
// (read through the same loader and parser the sidecar provider uses; links
// resolved beside it) or a resource read from the sidecar that publishes it
// (judged against that sidecar's own manifest, as its census does).
func (s *Service) lintArtifact(ctx context.Context, kind string, ref api.ArtifactRef) (verify.LintResult, error) {
	switch {
	case ref.File != "" && ref.URI != "":
		return verify.LintResult{}, fmt.Errorf("lint-%s: give --file or --uri, not both", kind)
	case ref.File != "":
		return contentlint.LintFile(s.lintPath(ref.File), kind, nil), nil
	case ref.URI != "":
		if ref.Exe == "" {
			return verify.LintResult{}, fmt.Errorf("lint-%s: --uri needs --exe, the sidecar that publishes it", kind)
		}
		rd := s.exeReader()
		m, err := rd.Manifest(ctx, ref.Exe)
		if err != nil {
			return verify.LintResult{}, err
		}
		src, err := rd.Source(ctx, ref.Exe, ref.URI)
		if err != nil {
			return verify.LintResult{}, err
		}
		prov, _, _ := strings.Cut(ref.URI, ":")
		return contentlint.LintBytes(kind, src, prov, contentlint.NewUniverse([]*mbapi.Manifest{m}), contentlint.ScopeSelf), nil
	}
	return verify.LintResult{}, fmt.Errorf("lint-%s: give --file <path> or --uri <uri> --exe <sidecar>", kind)
}

// LintDeck lints one slide deck. The grammar is the slidedeck extension's
// (deckapi.Lint — the same code the display tool runs); the layouts are the
// deck-layout contributions the given executable publishes, read from its
// manifest like every other files-only lint.
func (s *Service) LintDeck(ctx context.Context, in api.LintDeckInput) (verify.LintResult, error) {
	if strings.TrimSpace(in.File) == "" {
		return verify.LintResult{}, fmt.Errorf("lint-deck: give --file <deck>.deck.md")
	}
	src, err := os.ReadFile(s.lintPath(in.File))
	if err != nil {
		return verify.ErrorResult("unreadable", "cannot read "+in.File), nil
	}
	var g deckapi.Grammar
	var findings []verify.LintFinding
	if len(in.Exe) == 0 {
		findings = append(findings, verify.LintFinding{
			Rule:     "layouts-unchecked",
			Message:  "layouts, themes and slots were not checked — pass --exe <sidecar>[,<sidecar>…] (e.g. ktw-docs) to check them",
			Severity: verify.SeverityInfo,
		})
	} else {
		for _, exe := range in.Exe {
			m, err := s.exeReader().Manifest(ctx, exe)
			if err != nil {
				return verify.LintResult{}, err
			}
			for _, c := range m.Contributions {
				if len(c.Data()) == 0 {
					continue
				}
				switch c.Type {
				case deckapi.ContributionTypeDeckLayout:
					var l deckapi.LayoutBody
					if err := json.Unmarshal(c.Data(), &l); err == nil {
						g.Layouts = append(g.Layouts, l)
					}
				case deckapi.ContributionTypeDeckTheme:
					var t deckapi.ThemeBody
					if err := json.Unmarshal(c.Data(), &t); err == nil {
						g.Themes = append(g.Themes, t)
					}
				}
			}
		}
		if len(g.Layouts) == 0 {
			findings = append(findings, verify.LintFinding{
				Rule:     "layouts-unchecked",
				Message:  strings.Join(in.Exe, ", ") + " publish no deck-layout — layouts and slots were not checked",
				Severity: verify.SeverityWarn,
			})
		}
		// These sidecars are not the universe: a theme none of them publishes
		// is unchecked here, not unknown (ThemesComplete stays false).
	}
	for _, f := range deckapi.Lint(string(src), g) {
		sev := verify.SeverityWarn
		switch f.Severity {
		case deckapi.SeverityError:
			sev = verify.SeverityError
		case deckapi.SeverityInfo:
			sev = verify.SeverityInfo
		}
		findings = append(findings, verify.LintFinding{
			Rule:     f.Rule,
			Message:  fmt.Sprintf("line %d: %s", f.Line, f.Message),
			Severity: sev,
		})
	}
	return verify.Result(findings...), nil
}
