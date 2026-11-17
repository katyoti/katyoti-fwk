// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// imageRe is a markdown image: `![label](target)` or `![label](target "title")`.
var imageRe = regexp.MustCompile(`!\[[^\]]*\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)`)

// ModuleScheme names a file of the deck object's module, from its root —
// `![Flux](module:/dossiers/d1/travail/flux.drawio)` — so a deck shows a
// dossier's diagram where it lives, not a copy. Such a file is listed under
// ModuleAssetPrefix (`_module/<path>`); the host reads it from the module.
const (
	ModuleScheme      = "module:"
	ModuleAssetPrefix = "_module/"
)

// LocalAsset reports whether target names a file beside the deck: a relative
// path, not a URL (`https:`, `data:`, `icon:`…), not an absolute path, not an
// anchor. The second result is the cleaned path; it is empty when the path
// leaves the deck's folder (`..`).
func LocalAsset(target string) (string, bool) {
	t := strings.TrimSpace(target)
	if rest, ok := strings.CutPrefix(t, ModuleScheme); ok {
		// A file of the module, from its root: never above it.
		if i := strings.IndexAny(rest, "?#"); i >= 0 {
			rest = rest[:i]
		}
		clean := path.Clean("/" + strings.ReplaceAll(rest, "\\", "/"))
		if clean == "/" || strings.Contains(rest, "..") {
			return "", true
		}
		return ModuleAssetPrefix + strings.TrimPrefix(clean, "/"), true
	}
	if t == "" || strings.HasPrefix(t, "/") || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "\\") {
		return "", false
	}
	if i := strings.Index(t, ":"); i > 0 && !strings.ContainsAny(t[:i], "/.") {
		return "", false // a scheme: https:, data:, icon:, or a drive letter
	}
	if i := strings.IndexAny(t, "?#"); i >= 0 {
		t = t[:i]
	}
	clean := path.Clean(strings.ReplaceAll(t, "\\", "/"))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", true
	}
	return clean, true
}

// LocalAssets lists the files a deck uses beside it — its images and its
// slides' image backgrounds given by a relative path — sorted, without
// duplicates. Code blocks and inline code are not read. A path that leaves
// the deck's folder is left out (the lint names it).
func LocalAssets(src string) []string {
	set := map[string]bool{}
	add := func(t string) {
		if p, ok := LocalAsset(t); ok && p != "" {
			set[p] = true
		}
	}
	lines := splitLines(src)
	kinds, _ := classify(lines)
	for i, l := range lines {
		if kinds[i] == kText {
			for _, m := range imageRe.FindAllStringSubmatch(inlineCode.ReplaceAllString(l, ""), -1) {
				add(m[1])
			}
		}
	}
	for _, s := range Parse(src).Slides {
		if bg := s.Settings["background"]; bg != "" && !IsColour(bg) {
			add(bg)
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// LintAssets checks the files a deck names beside it, where the host knows the
// deck's folder: a path leaving it (`..`) is an error, a file that is not
// there a warning, each at its line. exists answers for a path relative to the
// deck's folder.
func LintAssets(src string, exists func(rel string) bool) []Finding {
	var out []Finding
	check := func(line int, target string) {
		p, ok := LocalAsset(target)
		switch {
		case !ok:
		case p == "":
			out = append(out, Finding{Line: line, Rule: "asset-outside", Severity: SeverityError,
				Message: fmt.Sprintf("%q leaves the deck's folder; a file beside the deck is in its folder or below", target)})
		case !exists(p):
			out = append(out, Finding{Line: line, Rule: "asset-missing", Severity: SeverityWarn,
				Message: fmt.Sprintf("%q is not beside the deck; the slide shows it broken", p)})
		}
	}
	lines := splitLines(src)
	kinds, _ := classify(lines)
	for i, l := range lines {
		if kinds[i] == kText {
			for _, m := range imageRe.FindAllStringSubmatch(inlineCode.ReplaceAllString(l, ""), -1) {
				check(i+1, m[1])
			}
		}
	}
	for _, s := range Parse(src).Slides {
		if bg := s.Settings["background"]; bg != "" && !IsColour(bg) {
			check(s.SettingsLine, bg)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out
}
