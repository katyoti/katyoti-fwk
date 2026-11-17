// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	busclientbus "github.com/katyoti/katyoti-fwk/internal/appcore/busclient/bus"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	codeapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	codelocal "github.com/katyoti/katyoti-fwk/internal/extensions/code/local"
	cwmclient "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/remote"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
)

// languagePackResolverExtension registers the per-language LanguagePack proxies
// that the code methodology resolves via UseNamedService[LanguagePack](ctx,
// lang). In the sidecar topology each language pack runs in its own
// `ktw-<lang>code` binary, so ktw-code dispatches to it over the bus instead of
// resolving an in-process impl. The proxy (BusLanguagePack) wraps the transparent
// hub sender; the PROVIDER is the language discriminator.
func languagePackResolverExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddBootstrap("ktw-code", ".language-pack-go", func(app *application.Application) error {
			application.PutService[codeapi.LanguagePack](app, "go",
				codelocal.NewBusLanguagePack(appcore.NewLazyHubSender(), "ktw-gocode"))
			return nil
		})
	}
}

// buildExtensions declares all ApplicationExtensions for the ktw-code sidecar.
//
// Intentionally does NOT wire `tasklocal.Extension()` — that publishes the full
// TaskService runner, which only ktw-agents should host. Sidecars contribute
// task *definitions* via TaskLibrary (WithTasks), they do not run agent tasks
// themselves.
//
// AutoKTSPContainer is called WITHOUT WithFlatCLI so CodeService methods nest
// under a `code` command group (`ktw-code code analyze-folder`, ...) alongside
// the `task` group TaskLibrary provides. Keeps the root surface clean.
// Migrated to the sidecarkit builder 2026-05-23 (SN-12).
func buildExtensions(efs embed.FS) []application.Extension {
	return sidecarkit.NewSidecar(appcore.AppConfig{
		AppName:    "ktw-code",
		Version:    version,
		Short:      "ktw-code — code analysis sidecar",
		EmbeddedFS: efs,
	},
		sidecarkit.WithDomain(codelocal.Extension()),
		sidecarkit.WithKTSPDescription("Code analysis and structure extraction"),
		// NO WithFlatCLI — methods nest under a `code` group.
		// LanguagePack proxies: the code methodology resolves UseNamedService
		// [LanguagePack](ctx, lang); ktw-code dispatches to each `ktw-<lang>code`
		// peer over the bus (B7 sidecar topology).
		sidecarkit.WithExtraExtensions(
			languagePackResolverExtension(),
			// CWM lives in ktw-agents: this binary's CWMService is the bus
			// client — `module verify` records its test-run, and CharterPublish
			// / ManifestWrite read the module's objects, through it.
			cwmclient.RemoteExtension(),
			busclientbus.Extension(),           // the bus-backed Sender the generated client calls through
			busclientbus.RunningHubExtension(), // the CLI's bus: the running hub's
		),
		// This binary's own CWM type pack (embed/cwm — the code-method
		// kinds: module/audit review, charter/test-run/finding,
		// wants/closes) is a part of its pack, read by the kit.
		// The skills runtime (SkillManager, `sc skills`): this binary's own wiring, never the kit's.
		sidecarkit.WithDomain(skillslocal.Extension()),
	)
}
