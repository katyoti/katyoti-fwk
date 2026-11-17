// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// ToolDefs returns the external tool definitions for the ktw-sync sidecar:
// rclone, required, with an install recipe per platform; the install skill
// stays the prose fallback.
func ToolDefs() []appcore.ToolDef {
	return []appcore.ToolDef{
		{
			Name:         "rclone",
			Commands:     []string{"rclone"},
			VersionFlag:  "version",
			InstallSkill: "ktw-sync:/skill/install-rclone",
			Recipes: []publish.InstallRecipe{
				{Platform: publish.PlatformWindows, Kind: publish.RecipeWinget, Package: "Rclone.Rclone"},
				{Platform: publish.PlatformWindows, Kind: publish.RecipeScoop, Package: "rclone"},
				{Platform: publish.PlatformDarwin, Kind: publish.RecipeBrew, Package: "rclone"},
				{Platform: publish.PlatformLinux, Kind: publish.RecipeApt, Package: "rclone"},
				{Platform: publish.PlatformLinux, Kind: publish.RecipeBrew, Package: "rclone"},
			},
		},
	}
}
