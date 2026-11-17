// SPDX-License-Identifier: Apache-2.0

// Package application provides the unified entry point for all ktw binaries.
//
// Application owns the full lifecycle: config → extension manager → service manager → bootstrap.
// Extensions interact with Application during init (hook registration) and at runtime
// (service resolution, point access).
//
// # Bootstrap
//
//	app := application.New(cfgSvc,
//	    application.DeclareService(myext.NewGreeter),
//	    appcore.NewAppExt(cfg),
//	)
//	app.Run()
//	svc, _ := application.UseService[Greeter](app)
//
// # Phases
//
//	PhaseRegisterPoints → PhaseContribute → PhaseAppInit → PhaseBootstrap → PhaseDone
package application

import (
	"fmt"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/services"
)

// Application owns the EM, ServiceManager, and ConfigService.
// Created via New(), bootstrapped via Run().
type Application struct {
	cfg      *config.ConfigService
	env      AppEnvironment
	em       *em.EM
	sm       *services.ServiceManager
	phase    Phase
	init     *ApplicationInit
	logger   *slog.Logger
	provider string // the binary's self-provider name (e.g. "ktw-agents"), set by appcore.NewAppExt
}

// Provider returns the binary's self-provider name (the value of
// appcore.AppConfig.AppName recorded at NewAppExt time). Used by:
//   - The KTSP slot URI parser, when an authoring path has empty
//     provider before the `:` separator (e.g. `:/Terminal/StartTask`
//     resolves the provider from the active Application).
//   - The endpoint dispatch sanity-check that incoming bus requests
//     address the current binary.
//
// Returns "" when called before NewAppExt's Contribute hook fires
// (very early init) — callers depending on it MUST resolve at
// AppInit phase or later.
func (app *Application) Provider() string {
	return app.provider
}

// SetProvider records the binary's self-provider name. Called by
// appcore.NewAppExt during the Contribute phase using cfg.AppName.
// Public so non-appcore boots (tests, custom binaries) can populate
// the field without duplicating the wiring. Idempotent — repeated
// calls overwrite (last writer wins).
func (app *Application) SetProvider(name string) {
	app.provider = name
}

// New creates a new Application with the given ConfigService and extensions.
// Bootstrap services (Config, SM, Application) are registered in the SM cache immediately.
// Extensions are collected but not executed until Run().
func New(cfgSvc *config.ConfigService, exts ...Extension) *Application {
	return NewWithEnv(cfgSvc, nil, exts...)
}

// NewWithEnv creates a new Application with explicit ConfigService and AppEnvironment.
// If env is nil, Env() returns nil (useful in tests with mock config).
// Extensions are collected but not executed until Run().
func NewWithEnv(cfgSvc *config.ConfigService, env AppEnvironment, exts ...Extension) *Application {
	sm := services.NewServiceManager(cfgSvc)
	app := &Application{
		cfg:    cfgSvc,
		env:    env,
		em:     em.New(),
		sm:     sm,
		phase:  PhaseInit,
		logger: slog.Default(),
	}

	// Register bootstrap services.
	services.Put[*config.ConfigService](app.sm, "default", cfgSvc)
	if app.env == nil {
		app.env = Env() // use global singleton
	}
	services.Put[AppEnvironment](app.sm, "default", app.env)
	services.Put[*services.ServiceManager](app.sm, "default", app.sm)
	services.Put[*Application](app.sm, "default", app)

	// Collect hooks from all extensions.
	app.init = newApplicationInit()
	for _, ext := range exts {
		ext(app.init)
	}

	return app
}

// Run executes all 4 phases across all registered extensions.
// Each phase iterates all hooks before advancing to the next.
func (app *Application) Run() error {
	phases := []Phase{PhaseRegisterPoints, PhaseContribute, PhaseAppInit, PhaseBootstrap}
	for _, phase := range phases {
		app.phase = phase
		app.logger.Debug(phase.String())
		for _, entry := range app.init.hooks[phase] {
			app.em.SetCurrentExt(entry.Pkg + entry.Name)
			if err := entry.Fn(app); err != nil {
				return fmt.Errorf("application: %s%s: %s: %w",
					entry.Pkg, entry.Name, phase, err)
			}
		}
	}

	app.em.SetCurrentExt("")
	app.phase = PhaseDone

	// Register EM as a service (available after all points are registered).
	services.Put[*em.EM](app.sm, "default", app.em)
	return nil
}

// SetLogger sets a custom logger for the Application.
func (app *Application) SetLogger(logger *slog.Logger) {
	app.logger = logger
	app.em.SetLogger(logger)
}

// Register adds extensions after construction but before Run().
func (app *Application) Register(exts ...Extension) {
	for _, ext := range exts {
		ext(app.init)
	}
}
