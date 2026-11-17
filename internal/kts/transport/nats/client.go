// SPDX-License-Identifier: Apache-2.0

package nats

import (
	"context"
	"errors"
	"fmt"
	"net"
	neturl "net/url"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/busauth"
)

// ClientHandle is a connected NATS client wrapped as a MessageBus.
// Returned by Connect() — held by the caller for the duration of the
// connection. Close() detaches and tears down the underlying NATS conn.
type ClientHandle struct {
	conn *natsgo.Conn
	bus  mbapi.MessageBus

	mu    sync.Mutex
	hooks []func() // run, in order, each time the link comes back
}

// OnReconnect registers fn to run every time nats-go re-establishes the link
// after a drop. Events published while the link was down are gone — a
// subscriber that keeps a derived view must re-check it here, not wait for
// the next event that happens to arrive. Hooks run on the client's reconnect
// goroutine and must not block.
func (h *ClientHandle) OnReconnect(fn func()) {
	if h == nil || fn == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hooks = append(h.hooks, fn)
}

// fireReconnect runs every registered hook once, in registration order.
func (h *ClientHandle) fireReconnect() {
	h.mu.Lock()
	hooks := append([]func(){}, h.hooks...)
	h.mu.Unlock()
	for _, fn := range hooks {
		fn()
	}
}

// MessageBus returns the MessageBus surface for application code.
func (h *ClientHandle) MessageBus() mbapi.MessageBus { return h.bus }

// Close terminates the NATS connection. Idempotent.
func (h *ClientHandle) Close() {
	if h.conn != nil {
		h.conn.Close()
	}
}

// IsConnected reports whether the underlying NATS connection is live.
func (h *ClientHandle) IsConnected() bool {
	return h.conn != nil && h.conn.Status() == natsgo.CONNECTED
}

// ConnectOptions are the public knobs surfaced from nats.Options without
// pulling the natsgo import into caller packages. Per rule
// `nats-message-bus-only`, this transport package is the only place
// allowed to import the NATS Go client.
type ConnectOptions struct {
	// Name is the human-readable client name (shown by `nats` CLI tools).
	Name string

	// MaxReconnects: -1 = infinite (default). 0 disables reconnect.
	MaxReconnects int

	// AllowReconnect, when true, lets nats-go retry on link drop.
	// Default true.
	AllowReconnect bool

	// ResolveURL, when set, is asked for the server's CURRENT URL before every
	// dial — the first one and every reconnect attempt. A server that comes
	// back on another address (an embedded server on a random port, restarted)
	// is then reached by the same reconnect loop that would otherwise dial
	// the old address for ever, and OnReconnect fires as for any reconnect.
	// An empty URL or an error means "dial what the client was given".
	// Called on nats-go's reconnect goroutine; it must return promptly.
	ResolveURL func() (string, error)

	// Credential is the program's bus key (busauth): the connection signs
	// the server's nonce with it, signs every message it sends, and checks
	// the sender of every message it receives against the key's registry.
	// Nil connects without a key - refused by a hub that checks keys.
	Credential *busauth.Credential
}

// resolvingDialer is nats-go's dialer with one change: the address it is asked
// to dial is replaced by the one ResolveURL names now, when it names one. The
// server pool nats-go keeps still carries the URL the client was created with;
// this is where a moved server is followed.
type resolvingDialer struct {
	resolve func() (string, error)
	timeout time.Duration
}

func (d *resolvingDialer) Dial(network, address string) (net.Conn, error) {
	if d.resolve != nil {
		if current, err := d.resolve(); err == nil && current != "" {
			if u, perr := neturl.Parse(current); perr == nil && u.Host != "" {
				address = u.Host
			}
		}
	}
	return (&net.Dialer{Timeout: d.timeout}).Dial(network, address)
}

// Connect opens a NATS connection and wraps it as a MessageBus.
// Suitable for hub clients (ktw-gui, ktw-agents CLI) that consume the
// hub's MessageBus surface without needing access to the NATS Go client.
func Connect(ctx context.Context, url string, opts ConnectOptions) (*ClientHandle, error) {
	natsOpts := []natsgo.Option{}
	if opts.Name != "" {
		natsOpts = append(natsOpts, natsgo.Name(opts.Name))
	}
	if opts.MaxReconnects != 0 {
		natsOpts = append(natsOpts, natsgo.MaxReconnects(opts.MaxReconnects))
	} else {
		natsOpts = append(natsOpts, natsgo.MaxReconnects(-1))
	}
	if !opts.AllowReconnect {
		natsOpts = append(natsOpts, natsgo.NoReconnect())
	}

	if opts.ResolveURL != nil {
		natsOpts = append(natsOpts, natsgo.SetCustomDialer(&resolvingDialer{
			resolve: opts.ResolveURL,
			timeout: natsgo.DefaultTimeout,
		}))
	}

	if opts.Credential != nil {
		natsOpts = append(natsOpts, natsgo.Nkey(opts.Credential.PublicKey(), opts.Credential.SignNonce))
	}

	h := &ClientHandle{}
	natsOpts = append(natsOpts, natsgo.ReconnectHandler(func(*natsgo.Conn) { h.fireReconnect() }))

	conn, err := natsgo.Connect(url, natsOpts...)
	if err != nil {
		if errors.Is(err, natsgo.ErrAuthorization) && opts.Credential == nil {
			return nil, fmt.Errorf("nats: connect %s: %w (the hub checks bus keys and this program has none)", url, err)
		}
		return nil, fmt.Errorf("nats: connect %s: %w", url, err)
	}
	h.conn, h.bus = conn, NewKeyed(conn, opts.Credential)
	return h, nil
}
