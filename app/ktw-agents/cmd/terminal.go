// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	termadapter "github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/adapter"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/room"
	termserver "github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/server"
	"github.com/spf13/cobra"
)

func init() {
	terminalCmd := &cobra.Command{
		Use:    "terminal",
		Short:  "Serve the terminal protocol (for external UIs)",
		Hidden: true,
	}

	stdioCmd := &cobra.Command{
		Use:   "stdio",
		Short: "Serve terminal protocol over stdin/stdout",
		Long: `Serves the KTS terminal protocol over stdin/stdout (NDJSON).
An external TUI or GUI spawns ktw-agents as a subprocess:

    ktw-utils tui --spawn "ktw-agents terminal stdio --task hello-world"`,
		RunE: terminalStdioRun,
	}
	stdioCmd.Flags().String("task", "", "Task to start immediately on connect")
	stdioCmd.Flags().String("workspace", "", "Workspace root for the task")
	stdioCmd.Flags().String("input", "", "Initial user input passed to the task (wrapped as user_input in TaskRunRequest.Input)")

	wsCmd := &cobra.Command{
		Use:   "ws",
		Short: "Serve terminal protocol over WebSocket",
		Long: `Serves the KTS terminal protocol over WebSocket. External UIs (TUI, GUI)
connect via ws://<host>:<port>/terminal and receive live terminal messages.
Multiple clients can connect to the same Room simultaneously.

    ktw-agents terminal ws --port 9100
    ktw-gui --connect ws://localhost:9100/terminal`,
		RunE: terminalWSRun,
	}
	wsCmd.Flags().Int("port", 9100, "TCP port to listen on")
	wsCmd.Flags().String("host", "localhost", "Interface to bind (use 0.0.0.0 for all interfaces)")
	wsCmd.Flags().String("path", "/terminal", "HTTP path for the WebSocket endpoint")

	terminalCmd.AddCommand(stdioCmd)
	terminalCmd.AddCommand(wsCmd)
	rootCmd.AddCommand(terminalCmd)
}

func terminalStdioRun(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	adapter := termadapter.NewStdio(os.Stdin, os.Stdout)
	defer adapter.Close()

	taskName, _ := cmd.Flags().GetString("task")
	if taskName == "" {
		// No task — serve protocol and wait for commands.
		ch, err := adapter.Receive(ctx)
		if err != nil {
			return err
		}
		for range ch {
		}
		return nil
	}

	workspace, _ := cmd.Flags().GetString("workspace")
	userInput, _ := cmd.Flags().GetString("input")
	opts := termserver.TaskOpts{
		TaskName:      taskName,
		WorkspaceRoot: workspace,
	}
	if userInput != "" {
		opts.Input = map[string]any{"user_input": userInput}
	}
	return termserver.ServeTask(ctx, adapter, opts)
}

func terminalWSRun(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	port, _ := cmd.Flags().GetInt("port")
	host, _ := cmd.Flags().GetString("host")
	path, _ := cmd.Flags().GetString("path")

	// Resolve or create Room — shared by all connected clients.
	r, _ := application.UseService[*room.Room](ctx)
	if r == nil {
		r = room.NewRoom("", nil)
	}
	defer r.Close()

	// Override HumanGate: route prompts through terminal protocol. The WS
	// surface is observe-all (clients connect with sessionID ""), so the
	// gate registers under "" to match their answer-command routing.
	gate := room.NewHumanGate()
	r.SetHumanGate("", gate)
	if app := application.AppFromContext(ctx); app != nil {
		application.PutService[hgapi.HumanGate](app, "default", hgapi.HumanGate(gate))
	}

	// Verify TaskService is available.
	if _, err := application.UseService[taskapi.TaskService](ctx); err != nil {
		return fmt.Errorf("terminal ws: task service not available: %w", err)
	}

	// Wire WebSocket handler.
	mux := http.NewServeMux()
	srv := termadapter.NewWSServer(r)
	mux.HandleFunc(path, srv.Handler())

	httpServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", host, port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Serve in goroutine so we can handle ctx cancellation.
	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "Terminal protocol WebSocket server listening on ws://%s:%d%s\n", host, port, path)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
