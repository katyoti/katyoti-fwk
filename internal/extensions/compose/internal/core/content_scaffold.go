// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/appcore/frozenhost"
	"github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
)

// contentDirs maps an --include token to the conventional folder ktw-host
// adopts for that contribution type. The folder name IS the type, which is why
// no mapping file is needed at runtime.
var contentDirs = map[string]string{
	"skills":     "skills",
	"tasks":      "tasks",
	"personas":   "personas",
	"references": "references",
	"scripts":    "scripts",
}

// CreateContent scaffolds a CONTENT FOLDER — a files-only sidecar that runs on
// ktw-host with nothing compiled.
//
// Distinct from CreateSidecar, which scaffolds a Go sidecar under app/<name>/
// (main.go, cmd/, embedded content) and only works inside this repo. That is
// the wrong tool for the contributor this path exists for: someone with files
// and no Go toolchain, working in their own folder. Without this they had to
// hand-write ktw-host.yaml from documentation, which is a poor first step for a
// feature whose whole pitch is "just write files".
//
// The starter skill is not decoration. An included dir with no valid file fails
// the strict skill-corpus gate, so scaffolding an empty `skills/` would hand the
// user a folder that refuses to publish.
func (s *Service) CreateContent(_ context.Context, in api.CreateContentInput) (api.CreateContentOutput, error) {
	var out api.CreateContentOutput
	if !kebabRe.MatchString(in.Name) {
		return out, fmt.Errorf("invalid name %q: kebab-case required — it becomes the catalog item AND the runtime provider", in.Name)
	}

	parent := wsPath(in.Out)
	if strings.TrimSpace(parent) == "" {
		parent = "."
	}
	target := filepath.Join(parent, in.Name)
	if _, err := os.Stat(target); err == nil {
		return out, fmt.Errorf("%s already exists — compose never overwrites", target)
	}

	include := in.Include
	if include == "" {
		include = "skills"
	}
	var dirs []string
	for _, tok := range strings.Split(include, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		dir, ok := contentDirs[tok]
		if !ok {
			keys := make([]string, 0, len(contentDirs))
			for k := range contentDirs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return out, fmt.Errorf("unknown include %q (recognized: %s)", tok, strings.Join(keys, ", "))
		}
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	display := in.DisplayName
	if display == "" {
		display = in.Name
	}
	desc := in.Description
	if desc == "" {
		desc = fmt.Sprintf("%s — files-only sidecar", in.Name)
	}

	manifest := fmt.Sprintf("name: %s\ndisplayName: %s\ndescription: %s\n", in.Name, display, desc)
	manifestPath := filepath.Join(target, frozenhost.ManifestFile)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return out, err
	}
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		return out, err
	}
	out.Folder = target
	out.Files = append(out.Files, manifestPath)

	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(target, dir), 0o755); err != nil {
			return out, err
		}
		if dir != "skills" {
			continue
		}
		skill := filepath.Join(target, "skills", in.Name+"-usage.md")
		body := fmt.Sprintf(`---
name: %s-usage
description: How to use %s.
---

Replace this with the real procedure. Everything in this folder is loaded by
`+"`ktw-host --folder`"+` — no build step, no Go.
`, in.Name, display)
		if err := os.WriteFile(skill, []byte(body), 0o644); err != nil {
			return out, err
		}
		out.Files = append(out.Files, skill)
	}
	return out, nil
}
