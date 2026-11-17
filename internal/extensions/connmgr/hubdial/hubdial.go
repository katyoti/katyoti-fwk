// SPDX-License-Identifier: Apache-2.0

// Package hubdial reaches a running universe's hub from another process and
// its ConnMgrService on that universe's bus: the members that run, a member's
// restart.
package hubdial

import (
	"context"
	"encoding/json"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

// Conn is a running universe's hub reached from another process: its members
// and the connection to close once done.
type Conn interface {
	// Running is the members whose process runs.
	Running(ctx context.Context) ([]string, error)
	// Restart stops a running member and starts it again; false when it
	// was not running.
	Restart(ctx context.Context, member string) (bool, error)
	// Close closes the connection to the hub.
	Close()
}

// Dial reaches the hub of the universe at home through its lockfile, when
// one runs, and its ConnMgrService on the universe's bus: nil (no error) when
// no hub runs there. The one lane a control-plane service takes to restart a
// member of another universe (the marketplace after a key is saved, the
// orchestrator after the served project is switched).
func Dial(ctx context.Context, home string) (Conn, error) {
	env := application.EnvOf(home)
	if hubclient.Running(env.DataDir()) == nil {
		return nil, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	hc, err := hubclient.ConnectToHub(cctx, hubclient.ConnectOptions{DataDir: env.DataDir(), Mode: hubclient.ModeReuse})
	if err != nil {
		return nil, err
	}
	tenantID, universeID := cenv.ResolveTenantUniverse(env.UniverseDir())
	return &busHub{hc: hc, sender: messagebus.NewSender(hc.MessageBus(), tenantID, universeID)}, nil
}

// busHub is a universe's hub over its bus.
type busHub struct {
	hc     *hubclient.HubClient
	sender *messagebus.Sender
}

func (h *busHub) Running(ctx context.Context) ([]string, error) {
	raw, err := h.sender.Call(ctx, cmapi.MembersPath, cmapi.MembersInput{})
	if err != nil {
		return nil, err
	}
	var out cmapi.MembersOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	var names []string
	for _, m := range out.Members {
		if m.State == cmapi.ConnReady {
			names = append(names, m.Name)
		}
	}
	return names, nil
}

func (h *busHub) Restart(ctx context.Context, member string) (bool, error) {
	raw, err := h.sender.Call(ctx, cmapi.RestartPath, cmapi.RestartInput{Name: member})
	if err != nil {
		return false, err
	}
	var out cmapi.RestartOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return false, err
	}
	return out.Restarted, nil
}

func (h *busHub) Close() { _ = h.hc.Close() }
