// SPDX-License-Identifier: Apache-2.0

// Package remote is the bus-backed PanelService for binaries that do not run
// the panel server: every method is one round-trip to ktw-hub, which keeps
// the universe's panels. A binary wires RemoteExtension (with
// busclient/bus.Extension(), the Sender it calls through) and its services
// resolve PanelService the standard way -
// application.UseService[panelapi.PanelService](ctx) - without knowing the
// server is elsewhere: a workflow's ask reads a panel's log after a restart
//
// HOST vs CONSUMER: a binary either runs the panel server (panelserver/local)
// or consumes it (RemoteExtension). Never both.
package remote

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore/busclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
)

// Host is the binary that publishes PanelService.
const Host = "ktw-hub"

// Client implements panelapi.PanelService over the message bus.
type Client struct{ c *busclient.Client }

// New returns the client addressing the panel server on its host.
func New() Client { return Client{c: busclient.New(Host, panelapi.PanelServiceDef)} }

// RemoteExtension registers the client as this binary's PanelService.
func RemoteExtension() application.Extension {
	return application.DeclareSimpleService[panelapi.PanelService](
		func(context.Context) (panelapi.PanelService, error) { return New(), nil },
	)
}

var _ panelapi.PanelService = Client{}

func (x Client) Snapshot(ctx context.Context, in panelapi.SnapshotInput) (panelapi.Snapshot, error) {
	return busclient.Call[panelapi.Snapshot](ctx, x.c, "Snapshot", in)
}

func (x Client) History(ctx context.Context, in panelapi.HistoryInput) (panelapi.HistoryOutput, error) {
	return busclient.Call[panelapi.HistoryOutput](ctx, x.c, "History", in)
}

func (x Client) List(ctx context.Context, in panelapi.ListInput) (panelapi.ListOutput, error) {
	return busclient.Call[panelapi.ListOutput](ctx, x.c, "List", in)
}

func (x Client) Keep(ctx context.Context, in panelapi.KeepInput) (panelapi.KeepOutput, error) {
	return busclient.Call[panelapi.KeepOutput](ctx, x.c, "Keep", in)
}

func (x Client) Template(ctx context.Context, in panelapi.TemplateInput) (panelapi.TemplateOutput, error) {
	return busclient.Call[panelapi.TemplateOutput](ctx, x.c, "Template", in)
}
