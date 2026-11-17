// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/console"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/server"
	"github.com/katyoti/katyoti-fwk/internal/core/services"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/spf13/cobra"
)

// AppCoreExt registers the core EM points (Manifests + PublishedServices).
// Idempotent — safe to call multiple times.
func AppCoreExt(init *application.ApplicationInit) {
	init.AddRegisterPoints("appcore", ".points", func(app *application.Application) error {
		if app.PointInfo(em.PointID[PublishedService]()) == nil {
			return application.RegisterPoint(app, NewPublishedServicesPoint())
		}
		return nil
	})
}

// SetProviderExt returns an Extension that records the binary's
// self-provider name on the Application — the same call
// `NewAppExt(cfg)` makes internally for sidecars using the standard
// AppConfig path. ktw-agents (and any other binary with a custom
// `buildApp`) wires this once at bootstrap so downstream extensions
// — chiefly `publish.ServicesContributionsExtension` — can resolve
// their own provider via `app.Provider()` without each call site
// passing a literal binary name.
//
// Wire BEFORE `appcore.SidecarExt(...)`: SidecarExt's contribution
// producers read the provider at AppInit time and fail loudly when it's
// empty.
func SetProviderExt(name string) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("appcore", ".provider."+name, func(app *application.Application) error {
			app.SetProvider(name)
			return nil
		})
	}
}

// NewAppExt returns an Extension that builds the full CLI for an application.
func NewAppExt(cfg AppConfig) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(AppCoreExt)

		init.AddContribute("appcore", "."+cfg.AppName, func(app *application.Application) error {
			// Record the binary's self-provider name on the Application
			// so the slot URI parser (and any other code that needs to
			// answer "what am I called on the bus?") can read it via
			// `application.SelfProviderFromContext(ctx)` or
			// `app.Provider()`. Single source of truth — no more
			// ad-hoc reads of `cr.Cmd.Use` scattered through callers.
			app.SetProvider(cfg.AppName)

			// Version string: a build-time-stamped cfg.Version wins;
			// otherwise ResolveVersion derives the git revision from the
			// binary's embedded VCS build info (so a plain build reports
			// its commit, not "dev"). Setting the `Version` field on the
			// cobra root automatically wires the `--version` flag (in
			// addition to the explicit `version` sub-command added below).
			rootVersion := ResolveVersion(cfg.Version)
			root := &cobra.Command{
				Use:     cfg.AppName,
				Short:   cfg.Short,
				Long:    cfg.Long,
				Version: rootVersion,
				PersistentPostRun: func(cmd *cobra.Command, args []string) {
					console.FlushAll()
					// Dispose ApplicationScope's container: every singleton
					// service that implements io.Closer gets Close()'d.
					// Specifically reaps `displaybridge.schemaBridge` whose
					// supervised bun subprocess would otherwise orphan when
					// a one-shot CLI command (e.g. `resources list`, `tasks
					// list`) exits — there's no other shutdown hook for
					// short-lived processes. Long-running commands like
					// `sc ktsp stdio` / `sc ktsp nats` already drain on SIGTERM;
					// PersistentPostRun fires AFTER that drain.
					CloseApplicationScope(cmd.Context())
				},
			}
			application.PutService[CobraCommand](app, "root", CobraCommand{Cmd: root})
			return nil
		})

		init.AddBootstrap("appcore", "."+cfg.AppName, func(app *application.Application) error {
			cr, err := application.AppUseNamedService[CobraCommand](app, "root")
			if err != nil {
				return fmt.Errorf("appcore: %w", err)
			}
			root := cr.Cmd

			// config lives under sc config (SidecarExt; modules/sidecars/tools.md §1)
			root.AddCommand(NewVersionCommand(cfg.AppName, ResolveVersion(cfg.Version)))
			root.AddCommand(NewDescribeCommand(cfg))
			root.AddCommand(NewDirsCommand())

			// KTSP commands (extensions, ktsp describe, serve, CLI)
			ktslocal.AddKTSPCommands(app, root, ktslocal.KTSPCommandConfig{
				ExtensionsCmd: func(container *ktsp.Endpoint) *cobra.Command {
					return NewExtensionsCommand(cfg, container)
				},
				KTSPDescribeCmd: func(container *ktsp.Endpoint) *cobra.Command {
					return NewKTSPDescribeCommand(cfg, container)
				},
			})

			// Server lifecycle commands
			if factory, err := application.AppUseService[server.LifecycleFactory](app); err == nil {
				root.AddCommand(server.CobraCommandsLazy(cfg.AppName, "server", factory))
			}

			return nil
		})
	}
}

// KTSPExt returns an Extension that puts the KTSP container in the ServiceManager.
// Delegates to kts/local.KTSPExt.
func KTSPExt(container *ktsp.Endpoint) application.Extension {
	return ktslocal.KTSPExt(container)
}

// ServerExt returns an Extension that puts the server lifecycle factory in the ServiceManager.
func ServerExt(factory server.LifecycleFactory) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("appcore", ".server", func(app *application.Application) error {
			application.PutService[server.LifecycleFactory](app, "default", factory)
			return nil
		})
	}
}

// CommandExt returns an Extension that adds a cobra command to the CLI root.
func CommandExt(cmd *cobra.Command) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddBootstrap("appcore.command", "."+cmd.Use, func(app *application.Application) error {
			cr, err := application.AppUseNamedService[CobraCommand](app, "root")
			if err != nil {
				return fmt.Errorf("appcore: %w", err)
			}
			cr.Cmd.AddCommand(cmd)
			return nil
		})
	}
}

// BootstrapCLI runs the Application with the given extensions and executes the CLI root command.
func BootstrapCLI(name string, exts ...application.Extension) error {
	// Route framework logs to a file so they don't pollute stdout/stderr
	// (which carry the command's actual output). KTW_LOG_STDERR=1 opts out.
	SetupFileLogging(name)
	ApplyRuntimeGuards(name)
	server.Init(application.Env().DataDir())
	app := application.New(application.Config(), exts...)
	if err := app.Run(); err != nil {
		return fmt.Errorf("bootstrap[%s]: %w", name, err)
	}
	cr, err := application.AppUseNamedService[CobraCommand](app, "root")
	if err != nil {
		return fmt.Errorf("bootstrap[%s]: %w", name, err)
	}
	ctx := application.WithApplication(context.Background(), app)
	ctx = context.WithValue(ctx, appContextKey{}, app)
	// The person using this command is the cause of every write it makes —
	// over the bus, the header carries it (person.Cause: the profile's
	// handle, when one is set).
	return ExecuteCLI(person.Cause(ctx), cr.Cmd)
}

// ExecuteCLI runs a CLI root command, then closes the application's services
// WHATEVER the outcome: Cobra skips PersistentPostRun when the command fails,
// and an error exit must release what a success does (the workspace lock).
func ExecuteCLI(ctx context.Context, root *cobra.Command) error {
	err := root.ExecuteContext(ctx)
	CloseApplicationScope(ctx)
	return err
}

// CloseApplicationScope disposes the application container of the app in ctx:
// every singleton with background work (services.Drainer) is first drained
// while the services are still open — the CWM event observers record the
// command's own write — then every io.Closer is closed — the CWM service releasing
// the workspace lock, an owned hub, a supervised subprocess. A one-shot command
// calls it on EVERY exit path; closing twice is a no-op.
func CloseApplicationScope(ctx context.Context) {
	if container := services.ApplicationScope.ContainerFromContext(ctx); container != nil {
		_ = container.Close()
	}
}
