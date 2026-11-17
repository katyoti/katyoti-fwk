// SPDX-License-Identifier: Apache-2.0

// Routing helper — when a sidecar server is running for the same app, CLI
// invocations dispatch their method call through the server's HTTP endpoint
// rather than running the service in-process. This is what makes
// `ktw-example server start` followed by `ktw-example navigate ...` actually
// reuse the same chrome instance (instead of each CLI process launching its
// own browser).
//
// Detection: read `<homedir>/servers/<appname>.json` (written by Lifecycle
// at server start, removed at server stop). If present + URL non-empty, POST
// to `<URL>/<method-kebab>` with the JSON-encoded params; the auto-HTTP
// handler at `internal/kts/local/auto_http.go:buildHTTPHandler` accepts the
// same shape `container.Handle` accepts.
//
// Fallback: any error reading the info file, or any HTTP transport error,
// falls through to local in-process dispatch. This keeps CLI usable when no
// server is running, and gracefully degrades on stale info files.

package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	"github.com/katyoti/katyoti-fwk/internal/core/server"
	kcontainer "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// RemoteDispatcher sends one generated verb — provider, slot, method, the
// JSON the flags built — to the process serving this binary's slots, and
// returns the JSON it answered. timeout is the method's advertised one (zero:
// the transport's default).
type RemoteDispatcher func(ctx context.Context, provider, slot, method string, timeout time.Duration, data []byte) ([]byte, error)

// remoteDispatcher is set once per process, by the binary's bootstrap, when
// it decides it is a client of its running universe (nil: dispatch here).
var remoteDispatcher atomic.Pointer[RemoteDispatcher]

// SetRemoteDispatcher routes every generated verb through fn from now on;
// nil restores in-process dispatch.
func SetRemoteDispatcher(fn RemoteDispatcher) {
	if fn == nil {
		remoteDispatcher.Store(nil)
		return
	}
	remoteDispatcher.Store(&fn)
}

// dispatchCall invokes a method either via the running server (preferred) or
// via the local in-process container. Returns the same byte slice the local
// `container.Handle` would return — JSON-encoded output struct on success.
//
// Fallback policy: only **transport** failures (server unreachable, connection
// refused, malformed response, network timeout) cause a fallback to local
// dispatch. **Application-level** errors (the server replied with HTTP 4xx/5xx)
// are surfaced directly — falling back in that case would spin up a duplicate
// browser/process locally only to hit the same error, leaving orphaned state
// (this is the "2 chrome windows per command" failure mode).
func dispatchCall(ctx context.Context, container *kcontainer.Endpoint, slotName, methodName string, data []byte) ([]byte, error) {
	appName := container.Info().Name
	// A CLIENT OF THE RUNNING UNIVERSE: when the binary's
	// slots are served by another process — the universe's hub has a serving
	// member — every generated verb is sent there, with the method's own
	// timeout, and its answer rendered here. Never a fallback to in-process
	// dispatch: the local service opened nothing (no workspace, no lock), and
	// an error is the serving process's answer, authoritative as the HTTP
	// server's below.
	if remote := remoteDispatcher.Load(); remote != nil {
		var timeout time.Duration
		if slot := container.Slot(slotName, kcontainer.DefaultInstance); slot != nil {
			if def := slot.Def(methodName); def != nil && def.Opts().TimeoutS > 0 {
				timeout = time.Duration(def.Opts().TimeoutS) * time.Second
			}
		}
		return (*remote)(ctx, appName, slotName, methodName, timeout, data)
	}
	if appName != "" {
		if info, err := server.ReadInfo(appName); err == nil && info.URL != "" {
			body, status, transportErr := postToServer(ctx, info.URL, methodName, data)
			if transportErr != nil {
				// Server unreachable — fall through to local. Routing degrades
				// to old behavior; the caller sees its own browser session.
			} else {
				// Server replied (HTTP completed). Always surface the response,
				// even on non-200 — the server's diagnostic is the authoritative
				// outcome of this call.
				if status >= 200 && status < 300 {
					return body, nil
				}
				return body, fmt.Errorf("server %s returned %d: %s", info.URL, status, body)
			}
		}
	}
	// CLI dispatch addresses single-instance services today; named-
	// instance addressing on the wire (CRUD connections, etc.) goes
	// through the bus, not the CLI. When CLI gains `slot(instance)`
	// syntax in URIs, parse it upstream and thread the instance here.
	return container.Handle(ctx, slotName, kcontainer.DefaultInstance, methodName, data)
}

// postToServer issues a POST to the auto-HTTP server's `/<method-kebab>` route.
//
// Returns (body, status, transportErr):
//   - on transport failure (connection refused, timeout, malformed response):
//     body = nil, status = 0, transportErr != nil
//   - on any HTTP response (including 4xx/5xx): body and status are populated,
//     transportErr = nil
//
// The caller decides whether non-2xx is a fallback trigger or a hard error;
// see `dispatchCall` for the policy.
func postToServer(ctx context.Context, baseURL, methodName string, data []byte) ([]byte, int, error) {
	url := fmt.Sprintf("%s/%s", baseURL, corereflect.ToKebab(methodName))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}
