// SPDX-License-Identifier: Apache-2.0

package api

import "context"

// BackupSnapshotSource gives the universe backup job the scheduler store's
// newest consistent snapshot. The store's owner contributes it to the
// backup's point at init (application.Contribute); with none contributed the
// job copies the universe's configuration files only.
type BackupSnapshotSource struct {
	// Name is the contributor's name.
	Name string
	// Newest answers the newest snapshot's path - taken first when the
	// current hour has none - or "" when there is none.
	Newest func(ctx context.Context) (string, error)
}
