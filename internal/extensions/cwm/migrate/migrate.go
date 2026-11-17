// SPDX-License-Identifier: Apache-2.0

// Package migrate brings a project folder no hub serves to this binary's data
// version: what an import does to the copy it made.
package migrate

import (
	"context"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/core"
)

// Folder brings a project folder no hub serves - the copy an import
// made - to this binary's data version, in place, on a
// scratch workspace removed after. Data newer than this binary knows is
// refused, unchanged. A hub's own project is migrated through the cwm service
// (`cwm migrate`).
func Folder(ctx context.Context, projectRoot string) (api.MigrateOutput, error) {
	scratch, err := os.MkdirTemp("", "ktw-migrate-")
	if err != nil {
		return api.MigrateOutput{}, err
	}
	defer os.RemoveAll(scratch)
	return core.MigrateFolder(ctx, projectRoot, scratch)
}
