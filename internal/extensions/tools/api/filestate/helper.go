// SPDX-License-Identifier: Apache-2.0

package filestate

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// From resolves the session-scoped Store for the given context.
//
// The store is registered as a session-scoped service via Extension(), so
// the returned Store is the per-session instance — two contexts in
// different sessions will get different stores. If no store is registered
// (e.g. the tool is being unit-tested without going through the agent
// loop), From returns nil. Callers MUST nil-check before use:
//
//	store := filestate.From(ctx)
//	if store != nil {
//	    if entry, ok := store.Lookup(absPath); ok { ... }
//	}
//
// The nil-tolerance is intentional: it lets file_read run in test contexts
// (and in any future single-shot CLI mode) without forcing a session
// scope to exist. Production agent runs always have a session.
func From(ctx context.Context) Store {
	store, err := application.UseService[Store](ctx)
	if err != nil {
		return nil
	}
	return store
}
