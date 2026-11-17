// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

// The platforms an install recipe targets — runtime.GOOS values.
const (
	PlatformWindows = "windows"
	PlatformDarwin  = "darwin"
	PlatformLinux   = "linux"
)

// Platforms lists every platform a recipe may target, in display order.
var Platforms = []string{PlatformWindows, PlatformDarwin, PlatformLinux}

// The installer kinds of a recipe.
const (
	RecipeWinget   = "winget"   // Package is the winget id (Windows)
	RecipeScoop    = "scoop"    // Package is the scoop app, optionally bucket/app (Windows)
	RecipeBrew     = "brew"     // Package is the formula or cask (macOS, Linux)
	RecipeApt      = "apt"      // Package is the apt package (Linux)
	RecipePip      = "pip"      // Package is the pip requirement (any platform)
	RecipeBun      = "bun"      // Package is the npm package installed globally with bun (any platform)
	RecipeDownload = "download" // URL is fetched and checked against SHA256 (any platform)
	RecipeScript   = "script"   // Script is a script the sidecar ships (any platform)
)

// recipeKinds maps each installer kind to the platforms it may target; nil
// means every platform.
var recipeKinds = map[string][]string{
	RecipeWinget:   {PlatformWindows},
	RecipeScoop:    {PlatformWindows},
	RecipeBrew:     {PlatformDarwin, PlatformLinux},
	RecipeApt:      {PlatformLinux},
	RecipePip:      nil,
	RecipeBun:      nil,
	RecipeDownload: nil,
	RecipeScript:   nil,
}

// RecipeInstallers names, per kind, the commands that run its installer, any
// one of which is enough: pip runs as `<python> -m pip`, so a python command
// provides it as well as pip itself. A program whose declared commands meet a
// kind's installers provides that kind: an engine installs it before the
// programs a recipe of that kind installs. A kind absent here (download,
// script) needs no installer.
var RecipeInstallers = map[string][]string{
	RecipeWinget: {"winget"},
	RecipeScoop:  {"scoop"},
	RecipeBrew:   {"brew"},
	RecipeApt:    {"apt-get"},
	RecipePip:    {"python3", "python", "py", "pip3", "pip"},
	RecipeBun:    {"bun"},
}

// RecipeKinds lists every installer kind a recipe may name.
var RecipeKinds = []string{RecipeWinget, RecipeScoop, RecipeBrew, RecipeApt, RecipePip, RecipeBun, RecipeDownload, RecipeScript}

// InstallRecipe is one structured install step for a program on one platform.
// An engine runs it without knowing the sidecar; the ToolDef's InstallSkill
// stays the prose fallback for an LLM or a person. A tool's recipes for one
// platform are alternatives in order of preference.
type InstallRecipe struct {
	// Platform is the runtime.GOOS the recipe installs on: windows, darwin or linux.
	Platform string `json:"platform" yaml:"platform"`
	// Kind is the installer: winget, scoop, brew, apt, pip, bun, download or script.
	Kind string `json:"kind" yaml:"kind"`
	// Package is what a package-manager kind installs (winget id, scoop app,
	// brew formula, apt package, pip requirement, bun package).
	Package string `json:"package,omitempty" yaml:"package,omitempty"`
	// URL is what a download fetches (http or https).
	URL string `json:"url,omitempty" yaml:"url,omitempty"`
	// SHA256 is the hex digest a download must match; required for a download.
	SHA256 string `json:"sha256,omitempty" yaml:"sha256,omitempty"`
	// Script is the path of an install script the sidecar ships, relative to
	// its folder (forward slashes, never climbing out of it).
	Script string `json:"script,omitempty" yaml:"script,omitempty"`
	// MinVersion is the lowest version this recipe is expected to satisfy;
	// empty when any version will do.
	MinVersion string `json:"min_version,omitempty" yaml:"minVersion,omitempty"`
	// Custom (winget only) is added to the installer's own silent arguments
	// (winget --custom): what an unattended install needs that the package's
	// defaults do not give - Python's launcher for the person only, so no
	// elevation prompt waits on nobody.
	Custom string `json:"custom,omitempty" yaml:"custom,omitempty"`
}

// Validate refuses a recipe an engine could not run: an unknown platform or
// kind, a kind on a platform it does not serve, a missing or stray field.
func (r InstallRecipe) Validate() error {
	served, known := recipeKinds[r.Kind]
	if !known {
		return fmt.Errorf("unknown installer kind %q (known: %s)", r.Kind, strings.Join(RecipeKinds, ", "))
	}
	if !contains(Platforms, r.Platform) {
		return fmt.Errorf("%s recipe: unknown platform %q (known: %s)", r.Kind, r.Platform, strings.Join(Platforms, ", "))
	}
	if served != nil && !contains(served, r.Platform) {
		return fmt.Errorf("%s recipe: %s does not install on %s (only %s)", r.Kind, r.Kind, r.Platform, strings.Join(served, ", "))
	}
	if r.Custom != "" && r.Kind != RecipeWinget {
		return fmt.Errorf("%s recipe on %s: custom arguments are winget's only", r.Kind, r.Platform)
	}
	switch r.Kind {
	case RecipeDownload:
		if r.URL == "" {
			return fmt.Errorf("download recipe on %s: no url", r.Platform)
		}
		u, err := url.Parse(r.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return fmt.Errorf("download recipe on %s: url %q is not an http(s) URL", r.Platform, r.URL)
		}
		if r.SHA256 == "" {
			return fmt.Errorf("download recipe on %s: no sha256 for %s", r.Platform, r.URL)
		}
		if !isSHA256(r.SHA256) {
			return fmt.Errorf("download recipe on %s: sha256 %q is not 64 hex digits", r.Platform, r.SHA256)
		}
		if r.Package != "" || r.Script != "" {
			return fmt.Errorf("download recipe on %s: package and script do not apply", r.Platform)
		}
	case RecipeScript:
		if r.Script == "" {
			return fmt.Errorf("script recipe on %s: no script path", r.Platform)
		}
		if strings.Contains(r.Script, `\`) || path.IsAbs(r.Script) || strings.Contains(r.Script, ":") {
			return fmt.Errorf("script recipe on %s: script %q must be a relative forward-slash path", r.Platform, r.Script)
		}
		if c := path.Clean(r.Script); c == "." || c == ".." || strings.HasPrefix(c, "../") {
			return fmt.Errorf("script recipe on %s: script %q climbs out of the sidecar's folder", r.Platform, r.Script)
		}
		if r.Package != "" || r.URL != "" || r.SHA256 != "" {
			return fmt.Errorf("script recipe on %s: package, url and sha256 do not apply", r.Platform)
		}
	default: // a package manager
		if r.Package == "" {
			return fmt.Errorf("%s recipe on %s: no package", r.Kind, r.Platform)
		}
		if r.URL != "" || r.SHA256 != "" || r.Script != "" {
			return fmt.Errorf("%s recipe on %s: url, sha256 and script do not apply", r.Kind, r.Platform)
		}
	}
	return nil
}

// RecipesFor returns a tool's recipes for one platform, in order of preference.
func (t ToolDef) RecipesFor(platform string) []InstallRecipe {
	var out []InstallRecipe
	for _, r := range t.Recipes {
		if r.Platform == platform {
			out = append(out, r)
		}
	}
	return out
}

// Validate refuses a tool declaration an engine could not read: no name, or
// a recipe that does not validate. Every error names the tool.
func (t ToolDef) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("tool declaration without a name")
	}
	var errs []error
	for i, r := range t.Recipes {
		if err := r.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("tool %q recipe %d: %w", t.Name, i+1, err))
		}
	}
	return errors.Join(errs...)
}

// ValidateTools validates every declaration, joining the errors.
func ValidateTools(tools []ToolDef) error {
	var errs []error
	for _, t := range tools {
		if err := t.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ToolRequirement is a ToolDef as data: what a sidecar's describe carries so
// an installer engine reads the program, whether it is required and how to
// install it per platform, without knowing the sidecar.
type ToolRequirement struct {
	Name         string          `json:"name"`
	Required     bool            `json:"required"`
	Commands     []string        `json:"commands,omitempty"`
	VersionFlag  string          `json:"version_flag,omitempty"`
	Version      string          `json:"version,omitempty"`
	InstallSkill string          `json:"install_skill,omitempty"`
	Recipes      []InstallRecipe `json:"recipes,omitempty"`
}

// Requirement is the tool declaration as data.
func (t ToolDef) Requirement() ToolRequirement {
	return ToolRequirement{
		Name:         t.Name,
		Required:     !t.Optional,
		Commands:     t.Commands,
		VersionFlag:  t.VersionFlag,
		Version:      t.Version,
		InstallSkill: t.InstallSkill,
		Recipes:      t.Recipes,
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func isSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}
