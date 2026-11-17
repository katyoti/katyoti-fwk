// SPDX-License-Identifier: Apache-2.0

// Package embedded hosts the in-process NATS server used by ktw-hub.
//
// Importing this package pulls in `github.com/nats-io/nats-server/v2` —
// ~8-12 MB of binary weight. Only ktw-hub (and test code that spins up
// an isolated bus) should import here. Every other package — every
// sidecar — imports the parent `nats` package which is client-only
// (`nats.go` library, no server).
//
// Lint guard `tools/import-discipline/` enforces this boundary in CI.
package embedded

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"

	"github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/busauth"
)

// EmbeddedHandle holds an embedded nats-server + a connected client.
// Stop() drains the client and shuts down the server cleanly.
type EmbeddedHandle struct {
	Server *natsserver.Server
	Client *natsgo.Conn
	// Credential is the key the in-process client connected with, nil on a
	// server that checks no keys.
	Credential *busauth.Credential
}

// StartEmbeddedKeyed is StartEmbedded on a server that refuses every
// connection not signing its nonce with a key of reg - the keys the hub
// minted (busauth.Ensure) -, its in-process client connected with hub's key.
// One account (the server's global one); each connection is known by its
// key's owner (`<kind>:<name>`).
func StartEmbeddedKeyed(opts *natsserver.Options, reg busauth.Lookup, hub *busauth.Credential) (*EmbeddedHandle, error) {
	if opts == nil {
		return nil, errors.New("StartEmbeddedKeyed: nil options")
	}
	if reg == nil || hub == nil {
		return nil, errors.New("StartEmbeddedKeyed: a registry and the hub's key are required")
	}
	opts.AlwaysEnableNonce = true
	opts.CustomClientAuthentication = &keyAuth{reg: reg}
	return start(opts, hub)
}

// keyAuth is the embedded server's authentication: a connection presents a
// public key of the registry and signs the server's nonce with it.
type keyAuth struct{ reg busauth.Lookup }

func (a *keyAuth) Check(c natsserver.ClientAuthentication) bool {
	o := c.GetOpts()
	if o == nil || o.Nkey == "" || o.Sig == "" {
		return false
	}
	// The nonce first: proof the caller holds the key before any registry work.
	if !busauth.VerifyNonce(o.Nkey, c.GetNonce(), o.Sig) {
		return false
	}
	caller, ok := a.reg.Lookup(o.Nkey)
	if !ok {
		return false
	}
	c.RegisterUser(&natsserver.User{Username: caller.String()})
	return true
}

// EmbeddedOptions returns canonical server options for the embedded mode.
// See modules/bus/tools.md §2.1.
//
// storeDir is the JetStream storage path (typically `<DataDir>/nats/jetstream/`).
// quiet=true silences the server log; useful in tests.
func EmbeddedOptions(storeDir string, quiet bool) *natsserver.Options {
	return &natsserver.Options{
		Host:                  "127.0.0.1",
		Port:                  natsserver.RANDOM_PORT,
		ServerName:            "ktw-hub-embedded",
		JetStream:             true,
		StoreDir:              storeDir,
		NoLog:                 quiet,
		Debug:                 false,
		Trace:                 false,
		DisableShortFirstPing: true,
		MaxPayload:            MaxPayload,
	}
}

// MaxPayload is the largest message the embedded server carries: 8 MiB, the
// size NATS recommends at most, instead of its 1 MiB default. A whole-corpus
// reply (every CWM object of a repository, unfiltered) is several MiB; a
// reply still larger fails with an error naming its size, never in silence.
const MaxPayload = 8 << 20

// StartEmbedded starts an embedded nats-server, connects an in-process
// client, and provisions the JetStream streams + KV buckets.
//
// On failure at any step, all started resources are torn down before returning.
func StartEmbedded(opts *natsserver.Options) (*EmbeddedHandle, error) {
	if opts == nil {
		return nil, errors.New("StartEmbedded: nil options")
	}
	return start(opts, nil)
}

// start starts the server of opts and its in-process client, keyed with cred
// when not nil.
func start(opts *natsserver.Options, cred *busauth.Credential) (*EmbeddedHandle, error) {
	// Make sure the JetStream store directory exists; nats-server will
	// create it but we want a deterministic error path.
	if opts.JetStream && opts.StoreDir != "" {
		if abs, err := filepath.Abs(opts.StoreDir); err == nil {
			opts.StoreDir = abs
		}
	}
	sizeStore(opts)

	ns, err := natsserver.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("nats: NewServer: %w", err)
	}

	// Wire the server logger to its built-in (no-op when NoLog=true).
	ns.ConfigureLogger()

	// Run the server.
	go ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		ns.Shutdown()
		return nil, errors.New("nats: server not ready within 10s")
	}

	clientOpts := []natsgo.Option{
		natsgo.InProcessServer(ns),
		natsgo.Name("ktw-hub"),
		natsgo.MaxReconnects(-1),
		natsgo.ReconnectWait(time.Second),
	}
	if cred != nil {
		clientOpts = append(clientOpts, natsgo.Nkey(cred.PublicKey(), cred.SignNonce))
	}
	nc, err := natsgo.Connect(ns.ClientURL(), clientOpts...)
	if err != nil {
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, fmt.Errorf("nats: Connect: %w", err)
	}

	// Provision JetStream resources idempotently.
	if err := provisionJetStream(nc, storeLimit(ns)); err != nil {
		_ = nc.Drain()
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, fmt.Errorf("nats: provision JetStream: %w", err)
	}

	return &EmbeddedHandle{Server: ns, Client: nc, Credential: cred}, nil
}

// Clients counts the connections the server has had under the client name
// given — the hub's outside clients dial under one name, its members under
// their own: open, those still connected; ever, every one since the server
// started, closed ones included (the server keeps them), so a client that
// came and went between two counts is still known. Nil-safe.
func (h *EmbeddedHandle) Clients(name string) (open, ever int) {
	if h == nil || h.Server == nil {
		return 0, 0
	}
	for offset := 0; ; {
		page, err := h.Server.Connz(&natsserver.ConnzOptions{State: natsserver.ConnAll, Offset: offset, Limit: 1024})
		if err != nil || page == nil {
			return open, ever
		}
		for _, c := range page.Conns {
			if c.Name != name {
				continue
			}
			ever++
			if c.Stop == nil {
				open++
			}
		}
		offset += len(page.Conns)
		if len(page.Conns) == 0 || offset >= page.Total {
			return open, ever
		}
	}
}

// Stop drains the client connection and shuts down the embedded server.
// Safe to call on a partially-initialized handle (nil-safe).
func (h *EmbeddedHandle) Stop() {
	if h == nil {
		return
	}
	if h.Client != nil {
		_ = h.Client.Drain()
		// Drain returns immediately and runs async; wait briefly for it.
		// The conservative way is to poll Status until CLOSED.
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) && h.Client.Status() != natsgo.CLOSED {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if h.Server != nil {
		h.Server.Shutdown()
		h.Server.WaitForShutdown()
	}
}
