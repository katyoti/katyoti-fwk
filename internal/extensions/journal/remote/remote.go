// SPDX-License-Identifier: Apache-2.0

// Package remote is the bus-backed JournalService for binaries that do not
// host the journal: every method is one round-trip to ktw-agents, which
// records it (the process holding the workspace). A sidecar wires
// RemoteExtension and its services resolve JournalService the standard way —
// application.UseService[journalapi.JournalService](ctx) — without knowing the
// journal is elsewhere. It reads only: the journal's observer records in its
// own process, and a remote journal records nothing (journal/local).
package remote

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore/busclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
)

// Host is the binary that publishes JournalService.
const Host = "ktw-agents"

// Client implements api.JournalService over the message bus.
type Client struct{ c *busclient.Client }

// New returns the client addressing the journal on its host.
func New() Client { return Client{c: busclient.New(Host, api.JournalServiceDef)} }

// RemoteExtension registers the client as this binary's JournalService.
func RemoteExtension() application.Extension {
	return application.DeclareSimpleService[api.JournalService](
		func(context.Context) (api.JournalService, error) { return New(), nil },
	)
}

var _ api.JournalService = Client{}

func (x Client) Query(ctx context.Context, in api.QueryInput) (api.QueryOutput, error) {
	return busclient.Call[api.QueryOutput](ctx, x.c, "Query", in)
}

func (x Client) History(ctx context.Context, in api.HistoryInput) (api.QueryOutput, error) {
	return busclient.Call[api.QueryOutput](ctx, x.c, "History", in)
}

func (x Client) Transitions(ctx context.Context, in api.TransitionsInput) (api.TransitionsOutput, error) {
	return busclient.Call[api.TransitionsOutput](ctx, x.c, "Transitions", in)
}

func (x Client) Times(ctx context.Context, in api.TimesInput) (api.TimesOutput, error) {
	return busclient.Call[api.TimesOutput](ctx, x.c, "Times", in)
}

func (x Client) Invoked(ctx context.Context, in api.InvokedInput) (api.InvokedOutput, error) {
	return busclient.Call[api.InvokedOutput](ctx, x.c, "Invoked", in)
}

func (x Client) HistoryPage(ctx context.Context, in api.HistoryPageInput) (render.Def, error) {
	return busclient.Call[render.Def](ctx, x.c, "HistoryPage", in)
}
