// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/server"
	"github.com/katyoti/katyoti-fwk/internal/extensions/testharness/internal/core"
)

const (
	defaultPort = 8099
	defaultHost = "127.0.0.1"
)

// Extension registers the test harness lifecycle commands (start/stop/restart/status).
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddBootstrap("testharness", "lifecycle", func(app *application.Application) error {
			svc := core.NewTestHarnessService()

			factory := func() (*server.Lifecycle, error) {
				port := defaultPort
				host := defaultHost

				return &server.Lifecycle{
					Name:       "ktw-test",
					SubCommand: "server",
					Version:    "dev",
					Host:       host,
					Port:       port,
					URL:        fmt.Sprintf("http://%s:%d", host, port),
					FgArgs:     []string{"server", "start", "--fg"},
					Start: func(ctx context.Context) error {
						srv := &http.Server{
							Addr:    net.JoinHostPort(host, fmt.Sprintf("%d", port)),
							Handler: svc.Handler(),
						}
						go func() {
							<-ctx.Done()
							_ = srv.Close()
						}()
						fmt.Printf("ktw-test listening on http://%s:%d\n", host, port)
						if err := srv.ListenAndServe(); err != http.ErrServerClosed {
							return err
						}
						return nil
					},
				}, nil
			}

			// Register lifecycle commands on the root cobra command
			cr, err := application.AppUseNamedService[appcore.CobraCommand](app, "root")
			if err != nil {
				return fmt.Errorf("testharness: %w", err)
			}
			cr.Cmd.AddCommand(server.CobraCommandsLazy("ktw-test", "server", factory))
			return nil
		})
	}
}
