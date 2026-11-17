// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	docscore "github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/core"
	drawiorender "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/render"
)

// ToolDefs returns the programs ktw-docs runs, for `config check` and
// `describe`: pandoc and typst are required (every render goes through them);
// the others serve one feature each and are optional — offered by an
// installer, not preselected. Recipes come from docsapi (the toolchain's one
// declaration); the install skills stay the prose fallback.
func ToolDefs() []appcore.ToolDef {
	// Chrome / Chromium: browser-based export only.
	chrome := appcore.NewChromeToolDef()
	chrome.Optional = true
	return []appcore.ToolDef{
		// Version: the validated pair — pandoc and typst move together (see the
		// install skills); another version is reported "outdated".
		{Name: "pandoc", ConfigKey: "pandoc", Commands: []string{"pandoc"}, InstallSkill: "ktw-docs:/skill/install-pandoc", VersionFlag: "--version", Version: docsapi.PandocVersion, Recipes: docsapi.PandocRecipes()},
		{Name: "typst", ConfigKey: "typst", Commands: []string{"typst"}, InstallSkill: "ktw-docs:/skill/install-typst", VersionFlag: "--version", Version: docsapi.TypstVersion, Recipes: docsapi.TypstRecipes()},
		// bun: the ELK architecture-diagram layout only.
		docsapi.BunTool(),
		chrome,
		{
			// .drawio sources only.
			Name:         "drawio",
			InstallSkill: "ktw-docs:/skill/install-drawio",
			Optional:     true,
			Recipes:      docsapi.DrawioRecipes(),
			Check: func(_ context.Context) publish.CheckResult {
				_, err := drawiorender.FindDrawioExecutable()
				return publish.CheckResult{OK: err == nil}
			},
		},
		{
			// A web service, not a program: the public instance is the
			// default, so nothing to install and no recipe; self-hosting is
			// in the install skill.
			Name:         "kroki",
			InstallSkill: "ktw-docs:/skill/install-kroki",
			Optional:     true,
			Check: func(ctx context.Context) publish.CheckResult {
				settings, _ := application.UseConfig[docscore.DocServiceConfig](ctx)
				krokiURL := settings.KrokiURL
				if krokiURL == "" {
					krokiURL = "https://kroki.io"
				}
				client := &http.Client{Timeout: 10 * time.Second}
				resp, err := client.Get(krokiURL)
				if err != nil {
					return publish.CheckResult{AdditionalInfo: fmt.Sprintf("unreachable: %s", krokiURL)}
				}
				resp.Body.Close()
				return publish.CheckResult{FoundVersion: krokiURL, OK: true}
			},
		},
	}
}
