// SPDX-License-Identifier: Apache-2.0

// Package main is a tiny fake sidecar binary used by the connmgr
// integration tests. It mirrors what `<binary> sc ktsp nats` does but is
// stripped to a single demo/echo method plus a manual ResourceService
// stub — no per-binary dependencies (no skills, no tasks, no config),
// so the test build is fast.
//
// Builds via `go build ./internal/extensions/connmgr/internal/fakesidecar`
// from the connmgr core test (see nats_acquire_test.go::buildFakeSidecar).
//
// Inside the module's `internal/` tree so it isn't accessible to
// downstream callers; only connmgr's own tests build it.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	mbsubscribe "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
	"github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/busauth"
)

// fakeGetManifest returns a hard-coded Manifest so the integration
// test can exercise the catalog-refresh path end-to-end without
// importing manifestlocal (which would pull in the full extension
// machinery and slow the test build). Hash is hard-coded to a
// deterministic placeholder; production binaries compute it via
// manifestcore.computeHash.
func fakeGetManifest(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	m := &mbapi.Manifest{
		Provider: "fake",
		Version:  "0.0.1",
		Hash:     "sha256:fake-deterministic-hash",
		Contributions: []contribapi.Contribution{
			{Provider: "fake", Type: "skill", ID: "demo-skill"},
		},
	}
	return json.Marshal(mbapi.ContributionGetManifestOutput{Manifest: m})
}

func main() {
	url := os.Getenv("KTW_NATS_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "fake-sidecar: KTW_NATS_URL required")
		os.Exit(1)
	}
	tenant := os.Getenv("KTW_TENANT_ID")
	if tenant == "" {
		tenant = "local"
	}
	universe := os.Getenv("KTW_UNIVERSE_ID")
	if universe == "" {
		universe = "test"
	}

	ep := ktsp.NewEndpoint("fake", "", "")
	slot := ktsp.NewSlot("demo", ktsp.DefaultInstance, "", nil)
	slot.Add(ktsp.RawMethod("echo", ktsp.MethodOpts{},
		func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			if len(params) == 0 {
				return json.RawMessage(`null`), nil
			}
			return params, nil
		}))
	// getenv answers a variable as this process sees it, after reading the
	// file KTW_FAKE_DOTENV names the way a member reads its .env (a variable
	// already set wins): the restart test asks it which key value it took.
	loadFakeDotenv(os.Getenv("KTW_FAKE_DOTENV"))
	slot.Add(ktsp.RawMethod("getenv", ktsp.MethodOpts{},
		func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var in struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(params, &in)
			return json.Marshal(map[string]string{"value": os.Getenv(in.Name)})
		}))
	ep.AddSlot(slot)

	// Stub the `contributions` slot with just a `get-manifest` method so
	// the catalog refresh path has something to call.
	contribSlot := ktsp.NewSlot("resources", ktsp.DefaultInstance, "", nil)
	contribSlot.Add(ktsp.RawMethod("get-manifest", ktsp.MethodOpts{}, fakeGetManifest))
	ep.AddSlot(contribSlot)

	ctx := context.Background()
	key, err := busauth.FromEnv() // the key a keyed hub handed it
	if err != nil {
		fmt.Fprintln(os.Stderr, "bus key:", err)
		os.Exit(1)
	}
	h, err := natsxport.Connect(ctx, url, natsxport.ConnectOptions{
		Credential:     key,
		Name:           "fake-sidecar",
		AllowReconnect: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		os.Exit(1)
	}
	defer h.Close()

	// self-provider matches the endpoint's binary name ("fake").
	g, err := mbsubscribe.Subscribe(ctx, h.MessageBus(), tenant, universe, "fake", ep)
	if err != nil {
		fmt.Fprintln(os.Stderr, "subscribe:", err)
		os.Exit(1)
	}
	defer g.Drain()

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-sigCtx.Done()
}

// loadFakeDotenv sets each NAME=value line of path not already set.
func loadFakeDotenv(path string) {
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || k == "" {
			continue
		}
		if _, set := os.LookupEnv(k); !set {
			_ = os.Setenv(k, v)
		}
	}
}
