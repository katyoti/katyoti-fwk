// SPDX-License-Identifier: Apache-2.0

// Package busclient is the one way a binary calls a service another binary
// publishes: a typed client whose every method is one bus round-trip to
// `<provider>:<slot>/<Method>`. A consumer registers the client as the service
// (application.DeclareSimpleService), so callers resolve it with
// application.UseService exactly like a local one and never see the bus.
//
// The per-service clients are GENERATED (Generate, in gen.go) from the service
// interface — one line per method, all of them through Call — so a client is
// never a hand-kept copy of an interface that drifts.
//
// This package depends on the Sender CONTRACT only (internal/kts/api), never on
// the protocol, so a generated client can sit in an extension's `remote/`
// package (`remote-uses-kts-api`). The Sender it calls through comes from the
// SenderProvider service; the bus-backed one is busclient/bus, which the
// consuming binary wires.
package busclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// SenderProvider gives a Client the Sender to call through, per request (the
// Sender is scoped to the request's tenant and universe). Registered as a
// service; busclient/bus.Extension() registers the bus-backed one.
type SenderProvider interface {
	Sender(ctx context.Context) (ktsapi.Sender, error)
}

// ErrNotOverTheBus is returned by a generated method whose signature is not a
// request/reply (a stream, a callback): it exists only in the hosting process.
var ErrNotOverTheBus = errors.New("busclient: method is host-local only (not available over the bus)")

// Client addresses one published service. Stateless apart from its address:
// the scoped sender is built from the request ctx per call, so one instance
// serves every run.
type Client struct {
	slot     string // "<provider>:<slot>/"
	timeouts map[string]time.Duration
}

// New addresses the service def publishes, hosted by provider. The slot is the
// def's bus name — the same one its WithKTSPContainer registers — and each
// method's deadline is the def's TimeoutS.
func New(provider string, def meta.ServiceDef) *Client {
	c := &Client{slot: provider + ":" + def.ResolvedBusName() + "/", timeouts: map[string]time.Duration{}}
	for name, m := range def.Methods {
		if m.TimeoutS > 0 {
			c.timeouts[name] = time.Duration(m.TimeoutS) * time.Second
		}
	}
	return c
}

// Slot returns the address prefix, `<provider>:<slot>/`.
func (c *Client) Slot() string { return c.slot }

// Call sends one method and decodes its reply into O.
func Call[O any](ctx context.Context, c *Client, method string, in any) (O, error) {
	var zero O
	sp, err := application.UseService[SenderProvider](ctx)
	if err != nil {
		return zero, fmt.Errorf("%s%s: no sender (wire busclient/bus.Extension()): %w", c.slot, method, err)
	}
	sender, err := sp.Sender(ctx)
	if err != nil {
		return zero, fmt.Errorf("%s%s: %w", c.slot, method, err)
	}
	// A handler ctx arriving over the bus carries no deadline (the transport
	// passes its timeout as a request option), so without this every call
	// would fall back to the adapter's short default. WithTimeout never
	// extends a caller's own deadline.
	if d, ok := c.timeouts[method]; ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	raw, err := sender.Call(ctx, c.slot+method, in)
	if err != nil {
		return zero, fmt.Errorf("%s%s: %w", c.slot, method, err)
	}
	var out O
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, fmt.Errorf("decode %s%s response: %w", c.slot, method, err)
	}
	return out, nil
}

// NotOverTheBus is the error a generated host-local method returns.
func (c *Client) NotOverTheBus(method string) error {
	return fmt.Errorf("%s%s: %w", c.slot, method, ErrNotOverTheBus)
}
