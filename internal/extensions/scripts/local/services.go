// SPDX-License-Identifier: Apache-2.0

// Package local provides the in-process ScriptService as a PublishedService.
package local

import (
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scripts/api"
	scriptcore "github.com/katyoti/katyoti-fwk/internal/extensions/scripts/internal/core"
)

// Script loaders and custom script engines are contributed to two EM list
// points (api.ScriptLoader, api.ScriptEngine), declared HERE, not in internal/core: an extension point is WIRING — this
// package is the only thing that registers it, and the domain reads its
// contributions type-routed through application.UseExtensions, never by naming
// the point. Holding them in internal/ made the domain layer import the extension
// manager for a variable it did not use.
//
// A FRESH point per application (Extension registers them): a point is state,
// and one instance shared by every application of a process leaks the first
// one's contributions into the next.

// Extension registers ScriptService as a published service AND, at
// Bootstrap phase, surfaces every discovered script as its own slot
// method on the `scripts` slot. Per the 2026-05-23 the command-surface decision
// (ms-sidecar-norm), the slot has NO top-level CLI surface — it
// lives ONLY as a KTSP slot, addressable by the canonical URI
// `<provider>:scripts/<name>`. LLMs invoke scripts via that URI;
// operators inspect via `sc resources list --type script`.
//
// Per 2E + 6U: each script becomes addressable by the canonical slot
// URI `<provider>:scripts/<name>`, giving the LLM a uniform callable
// surface across services and scripts.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.PublishService[api.ScriptService, scriptcore.ScriptManagerConfig](
			scriptcore.NewScriptManager,
			appcore.WithServiceDef(api.ScriptServiceDef),
			appcore.WithKTSPContainer(),
			// Slot-only: no top-level CLI. (Previously `cli.flat: false`
			// to force grouped `scripts list/run` — that surface is
			// retired in favor of `<provider>:scripts/<name>` URIs +
			// `sc resources list --type script`.)
			appcore.WithKTSPSlotTags(map[string]string{"cli": "false"}),
		))

		init.AddRegisterPoints("scripts", ".points", func(app *application.Application) error {
			if err := application.RegisterPoint(app, em.NewListPoint[api.ScriptLoader]()); err != nil {
				return err
			}
			return application.RegisterPoint(app, em.NewListPoint[api.ScriptEngine]())
		})

		// Per-script slot-method registration (2E). Folded into the
		// main Extension() so every sidecar that imports scripts gets
		// the unified callable surface for free — no per-binary wire.
		init.AddExtension(PerScriptSlotMethodsExtension())
	}
}

// ContributeScriptLoader returns an Extension that contributes a ScriptLoader to the EP.
func ContributeScriptLoader(ldr loader.Loader, root string, provider string) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("scripts", ".loader."+provider, func(app *application.Application) error {
			return application.Contribute(app, api.ScriptLoader{Loader: ldr, Root: root, Provider: provider})
		})
	}
}

// ContributeScriptEngine returns an Extension that contributes a custom ScriptEngine.
func ContributeScriptEngine(engine api.ScriptEngine) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("scripts", ".engine."+engine.Name(), func(app *application.Application) error {
			return application.Contribute(app, engine)
		})
	}
}
