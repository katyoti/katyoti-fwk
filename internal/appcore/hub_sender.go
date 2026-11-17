// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

// NewLazyHubSender returns a ktsp.Sender that connects to the RUNNING hub
// (ModeReuse — never spawns one) and builds the scope-bound, lazy-spawn-retrying
// messagebus sender on FIRST use. Every cross-binary typed client (the catalog
// bus reader, the ActionService method dispatcher, ktw-code's BusLanguagePack)
// needs the same `connect → scope → RetrySender` chain; factoring it here means
// a consumer that wants to call a peer sidecar just does:
//
//	codelocal.NewBusLanguagePack(appcore.NewLazyHubSender(), "ktw-gocode")
//
// instead of re-deriving tenant/universe + the retry wrapper by hand. The
// RetrySender handles the cold-target case: an unspawned sidecar answers the
// first call with ErrSlotNotFound, the wrapper calls ConnMgrService.EnsureSpawned
// and retries once.
func NewLazyHubSender() ktsp.Sender {
	return &lazyHubSender{target: activeWorkspaceHub}
}

// HubSender is a ktsp.Sender that may own the hub it talks to: Close stops a
// hub the sender spawned (and only then), so a one-shot CLI leaves nothing
// running behind it.
type HubSender interface {
	ktsp.Sender
	Close() error
}

// NewAdminHubSender returns a sender to the ADMIN universe — the control plane
// serving the management services (`ktw-univers:<slot>/<method>`). It reuses a
// running admin hub, or spawns one scoped to the admin universe (ModeFindOrSpawn)
// that lives until Close. For a CLI that is a pure KTSP client of those services
// ((modules/univers/rules/R-001-the-hub-owns-management-state-clients-are-pure.cwm.md)). The admin universe must exist on disk
// (orchestrator admin.EnsureDefault — the launcher's pre-hub bootstrap).
func NewAdminHubSender() HubSender {
	return &lazyHubSender{target: adminHub}
}

// hubTarget says which hub to reach and how: the connect options and the
// universe the sender is scoped to.
type hubTarget func() (hubclient.ConnectOptions, string)

// activeWorkspaceHub: the RUNNING hub of the active workspace — never spawns.
func activeWorkspaceHub() (hubclient.ConnectOptions, string) {
	env := application.Env()
	universeID := "default"
	if m, err := cenv.LoadUniverseManifest(env.UniverseDir()); err == nil && m != nil && m.Name != "" {
		universeID = m.Name
	}
	return hubclient.ConnectOptions{DataDir: env.DataDir(), Mode: hubclient.ModeReuse}, universeID
}

// adminHub: the admin universe's hub, spawned when none is running.
func adminHub() (hubclient.ConnectOptions, string) {
	return adminHubAt(controlplane.AdminHome())
}

// adminHubAt reaches the admin universe rooted at home. Its data folder is the
// admin universe's own (its data_dir setting, else the default), never the
// caller's KTW_DATA: a sidecar inherits its hub's KTW_DATA, and taking it
// would reach the caller's universe hub and call scope admin where nothing
// listens (Slot not found). A hub spawned from here gets no
// KTW_DATA either, so it writes its lock file in its own data folder.
func adminHubAt(home string) (hubclient.ConnectOptions, string) {
	return hubclient.ConnectOptions{
		DataDir:  application.OwnDataDirOf(home),
		Mode:     hubclient.ModeFindOrSpawn,
		SpawnEnv: withoutEnv(hubclient.SpawnEnvForWorkspace(home), "KTW_DATA"),
	}, controlplane.AdminUniverse
}

// withoutEnv drops the variable name from env.
func withoutEnv(env []string, name string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, name+"=") {
			out = append(out, kv)
		}
	}
	return out
}

type lazyHubSender struct {
	target hubTarget
	once   sync.Once
	hc     *hubclient.HubClient
	inner  ktsp.Sender
	err    error
}

func (l *lazyHubSender) ensure(ctx context.Context) error {
	l.once.Do(func() {
		opts, universeID := l.target()
		hc, err := hubclient.ConnectToHub(ctx, opts)
		if err != nil {
			l.err = fmt.Errorf("connect to ktw-hub (universe %s): %w", universeID, err)
			return
		}
		l.hc = hc
		tenantID := os.Getenv("KTW_TENANT_ID")
		if tenantID == "" {
			tenantID = "local"
		}
		l.inner = messagebus.NewRetrySender(messagebus.NewSender(hc.MessageBus(), tenantID, universeID), "")
	})
	return l.err
}

// Close releases the hub connection; a hub this sender spawned is stopped.
func (l *lazyHubSender) Close() error {
	if l.hc == nil {
		return nil
	}
	return l.hc.Close()
}

func (l *lazyHubSender) Call(ctx context.Context, path string, params any, opts ...ktsp.CallOption) (json.RawMessage, error) {
	if err := l.ensure(ctx); err != nil {
		return nil, err
	}
	return l.inner.Call(ctx, path, params, opts...)
}

func (l *lazyHubSender) Notify(ctx context.Context, path string, params any, opts ...ktsp.CallOption) error {
	if err := l.ensure(ctx); err != nil {
		return err
	}
	return l.inner.Notify(ctx, path, params, opts...)
}

var _ HubSender = (*lazyHubSender)(nil)
