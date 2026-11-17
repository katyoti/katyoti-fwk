// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	natsembedded "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/embedded"
)

// adminUniverseWithDataDir lays out an admin universe at a temp home whose
// Settings.yaml names dataDir as its data folder.
func adminUniverseWithDataDir(t *testing.T, dataDir string) string {
	t.Helper()
	home := t.TempDir()
	cfg := filepath.Join(home, ".katyoti", "config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "data_dir: " + filepath.ToSlash(dataDir) + "\n"
	if err := os.WriteFile(filepath.Join(cfg, "Settings.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// A sidecar inherits its hub's KTW_DATA (the case universe's
// data folder). The admin sender must read the admin universe's own data
// folder and spawn the admin hub without that KTW_DATA.
func TestAdminHubIgnoresCallersKTWData(t *testing.T) {
	callers := t.TempDir()
	admins := t.TempDir()
	t.Setenv("KTW_DATA", callers)
	home := adminUniverseWithDataDir(t, admins)

	opts, universe := adminHubAt(home)
	if filepath.Clean(opts.DataDir) != filepath.Clean(admins) {
		t.Fatalf("admin DataDir = %s, want the admin universe's own %s (the caller's KTW_DATA is %s)", opts.DataDir, admins, callers)
	}
	if universe != "admin" {
		t.Errorf("universe = %q, want admin", universe)
	}
	for _, kv := range opts.SpawnEnv {
		if strings.HasPrefix(kv, "KTW_DATA=") {
			t.Errorf("the admin hub's spawn env carries %s: a hub spawned from here would write its lock file in the caller's data folder", kv)
		}
	}
}

func startFakeHub(t *testing.T, dataDir string) {
	t.Helper()
	h, err := natsembedded.StartEmbedded(natsembedded.EmbeddedOptions(filepath.Join(t.TempDir(), "js"), true))
	if err != nil {
		t.Fatalf("start embedded NATS: %v", err)
	}
	t.Cleanup(h.Stop)
	if err := hubclient.WriteLockFile(dataDir, hubclient.LockFile{PID: os.Getpid(), NATSURL: h.Server.ClientURL()}); err != nil {
		t.Fatal(err)
	}
}

// The reproduction: the case universe's hub writes its lock file in X (the
// KTW_DATA the sidecar inherited), the admin hub in Y (the admin universe's
// data_dir); the management slot is served only under scope admin, on Y. The
// admin sender's call is answered - it reached Y, not X.
func TestAdminHubSenderReachesTheAdminHubNotTheCallersHub(t *testing.T) {
	callers := t.TempDir()
	admins := t.TempDir()
	startFakeHub(t, callers)
	startFakeHub(t, admins)
	t.Setenv("KTW_DATA", callers)
	t.Setenv("KTW_TENANT_ID", "local")
	home := adminUniverseWithDataDir(t, admins)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	serving, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: admins, Mode: hubclient.ModeReuse})
	if err != nil {
		t.Fatalf("connect to the admin fake hub: %v", err)
	}
	defer serving.Close()
	ep := ktsp.NewEndpoint("ktw-univers", "test", "1.0.0")
	slot := ktsp.NewSlot("orchestrator", ktsp.DefaultInstance, "management", nil)
	slot.Add(ktsp.RawMethod("create-project", ktsp.MethodOpts{Desc: "answers ok"},
		func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":true}`), nil
		}))
	ep.AddSlot(slot)
	group, err := messagebus.Subscribe(ctx, serving.MessageBus(), "local", "admin", "ktw-univers", ep)
	if err != nil {
		t.Fatalf("serve scope admin: %v", err)
	}
	defer group.Unsubscribe()

	sender := &lazyHubSender{target: func() (hubclient.ConnectOptions, string) { return adminHubAt(home) }}
	defer sender.Close()
	callCtx, callCancel := context.WithTimeout(ctx, 10*time.Second)
	defer callCancel()
	got, err := sender.Call(callCtx, "ktw-univers:orchestrator/create-project", map[string]any{})
	if err != nil {
		t.Fatalf("the admin sender's call failed (it reached the hub of the caller's KTW_DATA %s?): %v", callers, err)
	}
	if string(got) != `{"ok":true}` {
		t.Errorf("answer = %s, want the admin hub's", got)
	}
}
