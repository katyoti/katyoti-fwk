// SPDX-License-Identifier: Apache-2.0

package filestate

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// Extension contributes the filestate Store as a session-scoped service.
//
// Each session that calls SessionScope.EnterScope (or RunInScope) gets its
// own Store instance, created on first use within that session and disposed
// when the session ends — the ScopeContainer calls Store.Close on dispose
// because Store is an io.Closer.
//
// Wire this from any binary that hosts file-touching tools (ktw-agents,
// sidecars, tests):
//
//	app := application.NewApp(...)
//	app.Use(filestate.Extension())
//
// Tools resolve the per-session store via filestate.From(ctx).
func Extension() application.Extension {
	return application.DeclareScopedSimpleService[Store](
		application.SessionScope,
		func(_ context.Context) (Store, error) {
			return NewMemoryStore(DefaultCapacity), nil
		},
	)
}
