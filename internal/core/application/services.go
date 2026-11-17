// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/services"
)

// ── Direct registration (init phase — operates on *Application) ─────────

// RegisterConfigured registers a service class with config.
// The create function receives ctx for resolving deps via UseService[T](ctx).
func RegisterConfigured[TService, TConfig any](app *Application, create func(ctx context.Context, cfg *TConfig) (TService, error)) {
	app.sm.RegisterService(services.ConfiguredService[TService, TConfig](create))
}

// RegisterSimple registers a service class with no config.
func RegisterSimple[TService any](app *Application, create func(ctx context.Context) (TService, error)) {
	app.sm.RegisterService(services.SimpleService[TService](create))
}

// RegisterService registers a ServiceRegistration with the application.
func RegisterService(app *Application, reg services.ServiceRegistration) {
	app.sm.RegisterService(reg)
}

// PutService registers a pre-created named service instance.
func PutService[T any](app *Application, name string, svc T) {
	services.Put[T](app.sm, name, svc)
}

// GetServiceByName resolves a service by class name and instance name.
// Non-generic version for dynamic/reflection-based resolution.
func GetServiceByName(app *Application, className, instanceName string) (any, error) {
	return app.sm.GetByName(app.Context(), className, instanceName)
}

// ── Extension declarations (return Extension for use in New()) ──────────

// DeclareService returns an Extension that contributes a service class with config.
// The create function receives ctx + decoded config. Deps resolved via UseService[T](ctx).
func DeclareService[TService, TConfig any](
	create func(ctx context.Context, cfg *TConfig) (TService, error),
) Extension {
	className := config.TypeName[TService]()
	return func(init *ApplicationInit) {
		init.AddContribute("services", "."+className, func(app *Application) error {
			RegisterConfigured(app, create)
			return nil
		})
	}
}

// DeclareSimpleService returns an Extension that contributes a service class with no config.
func DeclareSimpleService[TService any](
	create func(ctx context.Context) (TService, error),
) Extension {
	className := config.TypeName[TService]()
	return func(init *ApplicationInit) {
		init.AddContribute("services", "."+className, func(app *Application) error {
			RegisterSimple(app, create)
			return nil
		})
	}
}

// DeclareScopedService returns an Extension that contributes a scoped service class with config.
func DeclareScopedService[TService, TConfig any](
	scope services.Scope,
	create func(ctx context.Context, cfg *TConfig) (TService, error),
) Extension {
	className := config.TypeName[TService]()
	return func(init *ApplicationInit) {
		init.AddContribute("services", "."+className, func(app *Application) error {
			app.sm.RegisterService(services.ScopedService[TService, TConfig](scope, create))
			return nil
		})
	}
}

// DeclareScopedSimpleService returns an Extension that contributes a scoped service class with no config.
func DeclareScopedSimpleService[TService any](
	scope services.Scope,
	create func(ctx context.Context) (TService, error),
) Extension {
	className := config.TypeName[TService]()
	return func(init *ApplicationInit) {
		init.AddContribute("services", "."+className, func(app *Application) error {
			app.sm.RegisterService(services.ScopedSimpleService[TService](scope, create))
			return nil
		})
	}
}

// ── Init-phase service access (takes *Application directly) ─────────────

// AppUseService resolves a service from the Application's ServiceManager.
// Use UseService[T](ctx) at runtime — this is for init-phase only.
func AppUseService[T any](app *Application) (T, error) {
	return services.Get[T](app.sm, app.Context(), "default")
}

// AppUseNamedService resolves a named service from the Application's SM.
func AppUseNamedService[T any](app *Application, name string) (T, error) {
	return services.Get[T](app.sm, app.Context(), name)
}

// AppUseConfig loads a config from the Application's ConfigService.
func AppUseConfig[T any](app *Application) (T, error) {
	return config.UseConfig[T](app.cfg)
}

// ── Extension composition ───────────────────────────────────────────────

// DeclareExtensions composes multiple extensions into one.
func DeclareExtensions(exts ...Extension) Extension {
	return func(init *ApplicationInit) {
		for _, ext := range exts {
			init.AddExtension(ext)
		}
	}
}
