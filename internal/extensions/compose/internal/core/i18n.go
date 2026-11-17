// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"strings"

	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	contentlint "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/lint"
)

// I18n reports what a pack or a compiled sidecar still has to translate into
// one language, and scaffolds the stubs on a folder. The judgement is the one
// translation lint's (contentlint: PackTranslations / ManifestTranslations,
// LintTranslations underneath); this verb only chooses the source and shapes
// the report.
func (s *Service) I18n(ctx context.Context, in api.I18nInput) (api.I18nOutput, error) {
	lang := strings.TrimSpace(in.Lang)
	if lang == "" {
		lang = "fr"
	}
	if strings.TrimSpace(in.Target) == "" {
		return api.I18nOutput{}, fmt.Errorf("i18n: give a pack folder or a sidecar executable")
	}
	into := strings.TrimSpace(in.Into)
	if into != "" && into != targetProject {
		return api.I18nOutput{}, fmt.Errorf("i18n: invalid --into %q (want %q)", in.Into, targetProject)
	}
	var entries []contentlint.EntryTranslation
	var manifest *mbapi.Manifest
	target := s.lintPath(in.Target)
	if st, err := os.Stat(target); err == nil && st.IsDir() {
		if entries, err = contentlint.PackTranslations(target, lang); err != nil {
			return api.I18nOutput{}, err
		}
	} else {
		if in.Scaffold && into == "" {
			return api.I18nOutput{}, fmt.Errorf("i18n: --scaffold writes beside the sources — give the pack folder, not an executable (or --into project: override objects of the project)")
		}
		m, err := s.exeReader().Manifest(ctx, in.Target)
		if err != nil {
			return api.I18nOutput{}, err
		}
		manifest = m
		entries = contentlint.ManifestTranslations(m, lang)
	}
	if into != "" && manifest == nil {
		return api.I18nOutput{}, fmt.Errorf("i18n: --into project scaffolds a sidecar's entries - give its executable; the project's own entries are files of its contributions folder: i18n <folder> --scaffold")
	}

	out := api.I18nOutput{Status: "complete", Lang: lang, Entries: len(entries)}
	for _, e := range entries {
		out.Missing += len(e.Missing)
		if len(e.Findings) > 0 {
			out.Status = "invalid"
		}
		if in.Scaffold && into == "" && len(e.Missing) > 0 {
			n, err := contentlint.ScaffoldTranslations(e, lang)
			if err != nil {
				return out, err
			}
			if n > 0 {
				out.Scaffolded = append(out.Scaffolded, e.Entry)
			}
		}
		if in.Missing && len(e.Missing) == 0 && len(e.Findings) == 0 {
			continue
		}
		r := api.I18nEntry{Entry: e.Entry, Kind: e.Kind, SourceLang: e.SourceLang, Errors: e.Findings}
		for _, g := range e.Missing {
			r.Missing = append(r.Missing, api.I18nText{Path: g.Path, Source: g.Source})
		}
		out.Report = append(out.Report, r)
	}
	if in.Scaffold && into != "" {
		if err := s.i18nIntoProject(ctx, manifest, entries, lang, &out); err != nil {
			return out, err
		}
	}
	if out.Status == "complete" && out.Missing > 0 {
		out.Status = "incomplete"
	}
	return out, nil
}
