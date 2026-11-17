// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/services"
)

// ── Context helpers ─────────────────────────────────────────────────────

type appCtxKey struct{}
type selfProviderCtxKey struct{}
type projectCtxKey struct{}

// WithProject stamps the PROJECT directory a session/request operates on.
//
// A hub is one-per-universe but serves N projects; the process-singleton
// `Env()` is correct only for UNIVERSE-scoped resources (config, sidecars,
// DataDir). PROJECT-scoped resources (the CWM graph, documents) belong to the
// specific project a call targets — which the singleton cannot answer. A
// session sets this once (from the request that started it); project-scoped
// resolvers then read it via `ProjectEnv(ctx)`.
func WithProject(ctx context.Context, projectDir string) context.Context {
	if projectDir == "" {
		return ctx
	}
	return context.WithValue(ctx, projectCtxKey{}, projectDir)
}

// ProjectFromContext returns the project directory stamped on ctx, or "" if none
// (single-project CLI / no session project — fall back to the singleton).
func ProjectFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(projectCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// ProjectEnv returns the AppEnvironment for the ctx's project (`EnvOf(projectDir)`)
// when one is stamped, else the process-singleton `Env()`.
//
// Use ProjectEnv for PROJECT-scoped paths (`HomeDir` = CWM/docs). Use `Env()`
// directly ONLY for UNIVERSE-scoped paths (`ConfigDir`/`UniverseDir`/`DataDir`),
// which are the hub's single universe. This is the accessor that lets ONE hub
// serve N projects without the "which project's HomeDir?" ambiguity.
func ProjectEnv(ctx context.Context) AppEnvironment {
	if p := ProjectFromContext(ctx); p != "" {
		return EnvOf(p)
	}
	return Env()
}

// WithApplication stores the Application in context.
// Call once at startup (e.g., in cobra's ExecuteContext).
// All Use* functions resolve from this.
func WithApplication(ctx context.Context, app *Application) context.Context {
	ctx = context.WithValue(ctx, appCtxKey{}, app)
	if app == nil || app.sm == nil {
		return ctx // a bare Application literal (tests) has no manager to name
	}
	// ApplicationScope resolves to THIS app's container from here on, not to
	// whichever Application was constructed last in the process.
	return services.WithApplicationContainer(ctx, app.sm.Container())
}

// AppFromContext retrieves the Application from context, or nil.
func AppFromContext(ctx context.Context) *Application {
	if app, ok := ctx.Value(appCtxKey{}).(*Application); ok {
		return app
	}
	return nil
}

// WithSelfProvider stores an explicit self-provider override on the
// context. Takes precedence over the Application's Provider() when both
// are present. Primary use cases:
//   - Tests that exercise the slot-addressing path without booting a
//     full Application (just pass a synthetic provider).
//   - Sub-process bridges that need to forge a provider identity that
//     differs from the host binary's AppConfig.AppName (rare).
//
// Empty name is a no-op (returns ctx unchanged). Pass an empty string
// to remove an override only if no Application is bound — otherwise the
// Application's Provider() wins automatically when this override is
// absent.
func WithSelfProvider(ctx context.Context, name string) context.Context {
	if name == "" {
		return ctx
	}
	return context.WithValue(ctx, selfProviderCtxKey{}, name)
}

// SelfProviderFromContext reads the binary's self-provider name from
// the context. Resolution order:
//  1. Explicit WithSelfProvider override (if present).
//  2. Application.Provider() (if an Application is bound via
//     WithApplication and its provider field has been set —
//     `appcore.NewAppExt` populates it from AppConfig.AppName at the
//     Contribute phase).
//
// Returns `("", false)` when neither path yields a value. Callers that
// require a provider — typically the slot URI parser — should error
// distinctly so authors get a clear "missing provider, did you bind the
// Application?" message rather than a generic empty-token NATS panic.
func SelfProviderFromContext(ctx context.Context) (string, bool) {
	if v, ok := ctx.Value(selfProviderCtxKey{}).(string); ok && v != "" {
		return v, true
	}
	if app := AppFromContext(ctx); app != nil {
		if p := app.Provider(); p != "" {
			return p, true
		}
	}
	return "", false
}

// Context returns a context.Context with this Application stored in it.
// Used by init-phase code that receives *Application but needs to call Use* functions.
func (app *Application) Context() context.Context {
	return WithApplication(context.Background(), app)
}

// ── Runtime accessors (Use* — from context) ─────────────────────────────

// UseService resolves a service by type from context.
// This is the ONE universal pattern for service access at runtime.
//
//	svc, err := application.UseService[MyService](ctx)
func UseService[T any](ctx context.Context) (T, error) {
	return UseNamedService[T](ctx, "default")
}

// UseNamedService resolves a named service instance from context.
//
//	svc, err := application.UseNamedService[CacheService](ctx, "redis")
//
// Safe to call during factory execution — `services.Get` works
// re-entrantly because `ServiceManager` no longer holds a process-wide
// lock around factory calls (Phase E §5a refactor — per-entry
// `sync.Once` in ScopeContainer, registry RWMutex only).
func UseNamedService[T any](ctx context.Context, name string) (T, error) {
	app := AppFromContext(ctx)
	if app == nil {
		var zero T
		return zero, fmt.Errorf("application.UseNamedService[%s](%s): no Application in context", config.TypeName[T](), name)
	}
	return services.Get[T](app.sm, ctx, name)
}

// UseConfig loads a config file for T from context.
// File name derived from T's type name: PluginsConfig → "Plugins.yaml".
//
//	cfg, err := application.UseConfig[PluginsConfig](ctx)
func UseConfig[T any](ctx context.Context) (T, error) {
	app := AppFromContext(ctx)
	if app == nil {
		var zero T
		return zero, fmt.Errorf("application.UseConfig[%s]: no Application in context", config.TypeName[T]())
	}
	return config.UseConfig[T](app.cfg)
}

// UseExtensions returns all contributed entries for an extension point type.
//
//	entries := application.UseExtensions[PublishedService](ctx)
func UseExtensions[T any](ctx context.Context) []T {
	app := AppFromContext(ctx)
	if app == nil {
		return nil
	}
	point, err := em.GetPoint[T](app.em)
	if err != nil {
		return nil
	}
	return point.Entries()
}
