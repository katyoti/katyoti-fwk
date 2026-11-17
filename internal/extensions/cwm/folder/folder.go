// SPDX-License-Identifier: Apache-2.0

// Package folder opens a project folder no hub serves, in this process: a
// program that lays out a project before any universe serves it (a solution's
// install) writes and reads its objects through CWM's verbs -
// validated and indexed like any other write - never by writing files itself.
// It is public because a package outside cwm imports it: marketplace's
// soldocs (internal/extensions/marketplace/internal/soldocs) writes a
// solution's documents into a project folder at install.
package folder

import (
	"context"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// Project is a project folder opened in process. Close it when done: it
// releases the store and removes its scratch workspace.
type Project struct {
	api.CWMService
	close func()
}

// Close releases the project.
func (p *Project) Close() {
	if p.close != nil {
		p.close()
		p.close = nil
	}
}

// Open opens the project folder projectRoot in process, on a scratch
// workspace (its index) removed at Close. Data newer than this binary knows
// is refused, unchanged. Only the types compiled into CWM are
// declared: an object of another type is written as given, unchecked by its
// type. A hub's own project is written through the cwm service, never through
// Open.
func Open(ctx context.Context, projectRoot string) (*Project, error) {
	scratch, err := os.MkdirTemp("", "ktw-cwm-folder-")
	if err != nil {
		return nil, err
	}
	if err := storage.WriteWorkspaceRaw(scratch, []byte("version: \"1\"\n")); err != nil {
		os.RemoveAll(scratch)
		return nil, err
	}
	svc, err := core.New(ctx, &core.CWMServiceConfig{WorkspaceRoot: scratch, ProjectRoot: projectRoot})
	if err != nil {
		os.RemoveAll(scratch)
		return nil, err
	}
	return &Project{CWMService: svc, close: func() {
		svc.Close()
		os.RemoveAll(scratch)
	}}, nil
}
