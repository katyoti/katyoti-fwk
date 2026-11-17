// SPDX-License-Identifier: Apache-2.0

package api

import "github.com/katyoti/katyoti-fwk/internal/core/publish"

// The document toolchain's install recipes, declared once by ktw-docs, which
// owns the toolchain and its install skills (ktw-docs:/skill/install-*); every
// sidecar that runs one of these programs (ktw-docs, ktw-kb, a publishing connector)
// puts these recipes on its own publish.ToolDef. Package managers only: they
// pick the machine's architecture themselves, which a download recipe (one URL
// per platform) cannot.

// PandocVersion is the pandoc version ktw-docs is validated with (the pair
// with TypstVersion).
const PandocVersion = "3.11"

// TypstVersion is the typst version ktw-docs is validated with.
const TypstVersion = "0.15.1"

// PandocRecipes installs pandoc. No apt recipe: Debian and Ubuntu ship a
// pandoc older than PandocVersion.
func PandocRecipes() []publish.InstallRecipe {
	return []publish.InstallRecipe{
		{Platform: publish.PlatformWindows, Kind: publish.RecipeWinget, Package: "JohnMacFarlane.Pandoc", MinVersion: PandocVersion},
		{Platform: publish.PlatformWindows, Kind: publish.RecipeScoop, Package: "pandoc", MinVersion: PandocVersion},
		{Platform: publish.PlatformDarwin, Kind: publish.RecipeBrew, Package: "pandoc", MinVersion: PandocVersion},
		{Platform: publish.PlatformLinux, Kind: publish.RecipeBrew, Package: "pandoc", MinVersion: PandocVersion},
	}
}

// TypstRecipes installs typst. No apt recipe: Debian and Ubuntu do not
// package it.
func TypstRecipes() []publish.InstallRecipe {
	return []publish.InstallRecipe{
		{Platform: publish.PlatformWindows, Kind: publish.RecipeWinget, Package: "Typst.Typst", MinVersion: TypstVersion},
		{Platform: publish.PlatformWindows, Kind: publish.RecipeScoop, Package: "typst", MinVersion: TypstVersion},
		{Platform: publish.PlatformDarwin, Kind: publish.RecipeBrew, Package: "typst", MinVersion: TypstVersion},
		{Platform: publish.PlatformLinux, Kind: publish.RecipeBrew, Package: "typst", MinVersion: TypstVersion},
	}
}

// BunRecipes installs bun.
func BunRecipes() []publish.InstallRecipe {
	return []publish.InstallRecipe{
		{Platform: publish.PlatformWindows, Kind: publish.RecipeWinget, Package: "Oven-sh.Bun", MinVersion: "1.0"},
		{Platform: publish.PlatformWindows, Kind: publish.RecipeScoop, Package: "bun", MinVersion: "1.0"},
		{Platform: publish.PlatformDarwin, Kind: publish.RecipeBrew, Package: "bun", MinVersion: "1.0"},
		{Platform: publish.PlatformLinux, Kind: publish.RecipeBrew, Package: "bun", MinVersion: "1.0"},
	}
}

// BunTool is bun's declaration, optional: ktw-docs runs it for the ELK
// architecture-diagram layout, and a sidecar whose program installs with a bun
// recipe (mermaid-cli) declares it too, so an installer provides bun first.
func BunTool() publish.ToolDef {
	return publish.ToolDef{Name: "bun", Commands: []string{"bun"}, InstallSkill: "ktw-docs:/skill/install-bun", VersionFlag: "--version", Optional: true, Recipes: BunRecipes()}
}

// DrawioRecipes installs draw.io desktop (a brew cask on macOS: brew installs
// a cask by its name when no formula has it). No Linux recipe: draw.io ships
// for Linux only as an AppImage, a .deb or an .rpm per architecture, which no
// package-manager kind installs; the install skill covers it.
func DrawioRecipes() []publish.InstallRecipe {
	return []publish.InstallRecipe{
		{Platform: publish.PlatformWindows, Kind: publish.RecipeWinget, Package: "JGraph.Draw"},
		{Platform: publish.PlatformWindows, Kind: publish.RecipeScoop, Package: "extras/draw.io"},
		{Platform: publish.PlatformDarwin, Kind: publish.RecipeBrew, Package: "drawio"},
	}
}

// MermaidCLIRecipes installs mermaid-cli (mmdc), an npm package, globally
// with bun on every platform.
func MermaidCLIRecipes() []publish.InstallRecipe {
	const pkg = "@mermaid-js/mermaid-cli"
	return []publish.InstallRecipe{
		{Platform: publish.PlatformWindows, Kind: publish.RecipeBun, Package: pkg},
		{Platform: publish.PlatformDarwin, Kind: publish.RecipeBun, Package: pkg},
		{Platform: publish.PlatformLinux, Kind: publish.RecipeBun, Package: pkg},
	}
}
