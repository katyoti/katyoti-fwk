// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"os/exec"
	"runtime"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
)

// ToolDefs returns the programs ktw-kb runs, for `config check` and
// `describe`: pandoc (office and markup formats), pymupdf (the PDF
// extraction every profile falls back to) and python (which installs and runs
// it) are required; the others improve or
// widen extraction and are optional — offered by an installer, not
// preselected. Each carries its install recipes per platform; the install
// skills stay the prose fallback.
func ToolDefs() []appcore.ToolDef {
	return []appcore.ToolDef{
		// --- Required for basic extraction ---
		{
			Name:         "pandoc",
			ConfigKey:    "pandoc",
			Commands:     []string{"pandoc"},
			InstallSkill: "ktw-docs:/skill/install-pandoc",
			VersionFlag:  "--version",
			Recipes:      docsapi.PandocRecipes(),
		},

		// --- Python: runs the pip recipes below (pip is `python -m pip`) ---
		{
			// The Windows Store's python stub answers --version with no
			// version: the check does not take it for Python.
			Name:         "python",
			Commands:     []string{"python", "python3", "py"},
			InstallSkill: "ktw-kb:/skill/install-python",
			VersionFlag:  "--version",
			Recipes: []publish.InstallRecipe{
				// First Python's full build as a NuGet package (pip included),
				// unpacked in the person's tools folder: no MSI, whose
				// transaction never completes in a fresh Windows Sandbox.
				{Platform: publish.PlatformWindows, Kind: publish.RecipeDownload,
					URL:    "https://api.nuget.org/v3-flatcontainer/python/3.12.10/python.3.12.10.nupkg",
					SHA256: "0eb85c2dfccccf1b17352de4c397f69194035b7d37149eacc16f1147d93de3b8"},
				// The launcher for the person only (its default is for every
				// user, which asks for elevation), and python on their PATH.
				{Platform: publish.PlatformWindows, Kind: publish.RecipeWinget, Package: "Python.Python.3.12",
					Custom: "InstallLauncherAllUsers=0 PrependPath=1"},
				{Platform: publish.PlatformWindows, Kind: publish.RecipeScoop, Package: "python"},
				{Platform: publish.PlatformDarwin, Kind: publish.RecipeBrew, Package: "python@3.12"},
				// python3-pip brings python3 with it.
				{Platform: publish.PlatformLinux, Kind: publish.RecipeApt, Package: "python3-pip"},
				{Platform: publish.PlatformLinux, Kind: publish.RecipeBrew, Package: "python@3.12"},
			},
		},

		// --- PDF tools (pymupdf) ---
		{
			Name:         "pymupdf",
			InstallSkill: "ktw-kb:/skill/install-pymupdf",
			Recipes: []publish.InstallRecipe{
				{Platform: publish.PlatformWindows, Kind: publish.RecipePip, Package: "pymupdf"},
				{Platform: publish.PlatformDarwin, Kind: publish.RecipePip, Package: "pymupdf"},
				{Platform: publish.PlatformLinux, Kind: publish.RecipePip, Package: "pymupdf"},
			},
			// A Python package, checked as one in the Python pip installs with.
			PythonPackage: "pymupdf",
		},
		{
			// The best PDF profile (Markdown output); pymupdf-text is the fallback.
			Name:         "pymupdf4llm",
			InstallSkill: "ktw-kb:/skill/install-pymupdf4llm",
			Optional:     true,
			Recipes: []publish.InstallRecipe{
				{Platform: publish.PlatformWindows, Kind: publish.RecipePip, Package: "pymupdf4llm"},
				{Platform: publish.PlatformDarwin, Kind: publish.RecipePip, Package: "pymupdf4llm"},
				{Platform: publish.PlatformLinux, Kind: publish.RecipePip, Package: "pymupdf4llm"},
			},
			// A Python package, checked as one in the Python pip installs with.
			PythonPackage: "pymupdf4llm",
		},

		// --- Office COM (Windows — PPTX→PDF rendering) ---
		{
			// Windows only, and Microsoft Office itself is not installable by
			// a recipe: pywin32 is, on Windows alone.
			Name:         "office-com",
			InstallSkill: "ktw-kb:/skill/install-office-com",
			Optional:     true,
			Recipes: []publish.InstallRecipe{
				{Platform: publish.PlatformWindows, Kind: publish.RecipePip, Package: "pywin32"},
			},
			Check: func(_ context.Context) publish.CheckResult {
				if runtime.GOOS != "windows" {
					return publish.CheckResult{AdditionalInfo: "Windows only"}
				}
				cmd := exec.Command("python", "-c", "import win32com.client; print('ok')")
				if err := cmd.Run(); err != nil {
					return publish.CheckResult{AdditionalInfo: "pywin32 not installed"}
				}
				return publish.CheckResult{OK: true, FoundVersion: "pywin32 available"}
			},
		},

		// --- Audio/Video tools ---
		{
			Name:         "ffmpeg",
			ConfigKey:    "ffmpeg",
			Commands:     []string{"ffmpeg"},
			InstallSkill: "ktw-kb:/skill/install-ffmpeg",
			VersionFlag:  "-version",
			Optional:     true,
			Recipes: []publish.InstallRecipe{
				{Platform: publish.PlatformWindows, Kind: publish.RecipeWinget, Package: "Gyan.FFmpeg"},
				{Platform: publish.PlatformWindows, Kind: publish.RecipeScoop, Package: "ffmpeg"},
				{Platform: publish.PlatformDarwin, Kind: publish.RecipeBrew, Package: "ffmpeg"},
				{Platform: publish.PlatformLinux, Kind: publish.RecipeApt, Package: "ffmpeg"},
				{Platform: publish.PlatformLinux, Kind: publish.RecipeBrew, Package: "ffmpeg"},
			},
		},

		// --- AI-powered extraction tools (Python) ---
		{
			Name:         "docling",
			ConfigKey:    "docling",
			Commands:     []string{"docling"},
			InstallSkill: "ktw-kb:/skill/install-docling",
			VersionFlag:  "--version",
			Optional:     true,
			Recipes: []publish.InstallRecipe{
				{Platform: publish.PlatformWindows, Kind: publish.RecipePip, Package: "docling"},
				{Platform: publish.PlatformDarwin, Kind: publish.RecipePip, Package: "docling"},
				{Platform: publish.PlatformLinux, Kind: publish.RecipePip, Package: "docling"},
			},
		},
	}
}

// APIKeyDefs returns the API key definitions for the ktw-kb sidecar.
func APIKeyDefs() []appcore.APIKeyDef {
	return []appcore.APIKeyDef{
		// The embedding key is the key_env of a profile of
		// EmbeddingProfiles.yaml; the declaration is static (made before any
		// config is loaded), so both shipped variables are optional here and
		// a profile whose variable is unset fails at index or query time,
		// naming it.
		{
			EnvName:     "OPENAI_API_KEY",
			Label:       "OpenAI API key",
			Required:    false,
			Description: "Key of the default openai embedding profile (indexing and search)",
		},
		{
			EnvName:     "AZURE_API_KEY",
			Label:       "Azure / Foundry API key",
			Required:    false,
			Description: "Key of a foundry or azure-openai embedding profile when its key_env names it",
		},
		{
			EnvName:     "ANTHROPIC_API_KEY",
			Label:       "Anthropic API key",
			Required:    false,
			Description: "Used by vision-ocr and render-vision extractors (agentic profiles)",
		},
		{
			EnvName:     "ASSEMBLY_AI_API_KEY",
			Label:       "AssemblyAI API key",
			Required:    false,
			Description: "Used for audio/video transcription with speaker diarization",
		},
	}
}
