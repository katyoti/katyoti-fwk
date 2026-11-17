// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
)

// THE SQLITE STORE — the store port's second adapter (package sqlstore): one
// database file in the scheduler's folder (SchedulerDir()/scheduler.db, under
// TenantDataDir()), rows keyed by repository - the uuid of the project a module
// belongs to - and module. A universe with no planning file to migrate runs on it
// (OpenSchedulerStore); one holding files moves onto it with the migration
// that brings them in.

var _ Store = (*sqlstore.Store)(nil)

// OpenSQLiteStore opens the scheduler's SQLite store of an environment: the
// file in its SchedulerDir() and nowhere else - never a path derived from a
// checkout. An environment with no scheduler folder is refused.
func OpenSQLiteStore(env application.AppEnvironment) (*sqlstore.Store, error) {
	dir := ""
	if env != nil {
		dir = env.SchedulerDir()
	}
	if dir == "" {
		return nil, fmt.Errorf("%w: the scheduler's store: the environment names no scheduler folder (SchedulerDir)", cwmapi.ErrInvalidInput)
	}
	return sqlstore.Open(dir, sqlstore.Options{NewID: idOf, Now: now, RepoOf: repositoryOf})
}

// repositoryOf is the uuid of the project module belongs to: the key a
// module's rows carry - never a project key, a universe name or a folder.
// One module only answers no repository: the requests module when it resolves
// to no object (no project serves it, projectless) - the universe's own, its
// rows (a run's holding list) carry none (NULL). Every other
// case is refused, naming the module: any other module that resolves to no
// object (a typo, a module of a project not attached), and any module - the
// requests module included - whose project has no uuid: a project's rows are
// keyed by its uuid or not stored, never mixed with the universe's NULL rows.
func repositoryOf(ctx context.Context, module string) (string, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return "", err
	}
	got, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: module})
	if projectless(module, err) {
		return "", nil // the universe's own requests module: a row of no project
	}
	if err != nil {
		return "", fmt.Errorf("module %s: %w", module, err)
	}
	if got.ProjectID == "" {
		return "", fmt.Errorf("%w: module %s: its project %q has no uuid, the key of its to-dos", cwmapi.ErrInvalidInput, module, got.Project)
	}
	return got.ProjectID, nil
}
