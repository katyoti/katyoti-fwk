// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/spf13/cobra"
)

type appContextKey struct{}

// ContextKey returns the key used to store Application in cobra context.
// Exported for use by packages that build their own ExecuteContext call.
func ContextKey() appContextKey { return appContextKey{} }

// SetAppContext stores the Application in the cobra command's context.
// Called once after app.Run() completes, before rootCmd.Execute().
func SetAppContext(cmd *cobra.Command, app *application.Application) {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cmd.SetContext(context.WithValue(ctx, appContextKey{}, app))
}

// AppFrom retrieves the Application from a cobra command's context.
// Tries application.AppFromContext first (new pattern), falls back to legacy key.
func AppFrom(cmd *cobra.Command) *application.Application {
	if cmd.Context() == nil {
		panic("appcore.AppFrom: command has no context")
	}
	// New pattern
	if app := application.AppFromContext(cmd.Context()); app != nil {
		return app
	}
	// Legacy fallback
	app, ok := cmd.Context().Value(appContextKey{}).(*application.Application)
	if !ok || app == nil {
		panic("appcore.AppFrom: Application not found in context")
	}
	return app
}
