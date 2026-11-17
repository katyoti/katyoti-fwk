// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/katyoti/katyoti-fwk/internal/core/server"
	ktsm "github.com/katyoti/katyoti-fwk/internal/kts/sm"
)

// NewSMLifecycle creates a server.Lifecycle for the Session Manager.
// Used by cmd/sm.go to build start/stop/restart/status commands.
func NewSMLifecycle(version string, port int) *server.Lifecycle {
	return &server.Lifecycle{
		Name:    "sm",
		Version: version,
		Host:    "localhost",
		Port:    port,
		URL:     fmt.Sprintf("ws://localhost:%d/ktsp", port),
		FgArgs:  []string{"sm", "start", "--fg", "--port", fmt.Sprintf("%d", port)},
		Start:   makeSMStartFunc(port),
	}
}

func makeSMStartFunc(port int) server.StartFunc {
	return func(ctx context.Context) error {
		cm := ktsm.NewContainerManager()
		sm := ktsm.NewManager(cm)
		ws := ktsm.NewWsServer(sm, port)

		err := ws.Start(ctx)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
