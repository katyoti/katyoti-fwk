// SPDX-License-Identifier: Apache-2.0

package local

import (
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktransport "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/jsonrpc"
	"github.com/spf13/cobra"
)

// NewStdioSubCommand creates a `stdio` subcommand under the ktsp group.
// Name communicates the transport explicitly so the ktsp group can host
// other transports in the future (ws, http, ...).
//
// Pass-through detail (Phase E §5b): the cobra command's context carries
// the in-process Application — `StdioServe` derives every per-request
// ctx from it, so dispatched service handlers see the same EM points,
// services, and configuration as the in-process CLI surface.
func NewStdioSubCommand(container *ktsp.Endpoint) *cobra.Command {
	return &cobra.Command{
		Use:   "stdio",
		Short: "Run as a stdio KTSP server (used by ktw-agents)",
		Long: `Starts the sidecar in stdio server mode: reads JSON-RPC 2.0 requests
from stdin, dispatches them to the KTSP container, and writes responses
to stdout. This is the transport used when ktw-agents spawns sidecars
as subprocesses.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ktransport.StdioServe(cmd.Context(), container)
		},
	}
}

// NewFallbackKTSPCommand creates a top-level `ktsp` command that runs the
// container as a stdio KTSP server. Used when no KTSPDescribeCmd group is
// configured — rare, but kept for binaries that only need the server surface.
func NewFallbackKTSPCommand(container *ktsp.Endpoint) *cobra.Command {
	return &cobra.Command{
		Use:   "ktsp",
		Short: "Run as a stdio KTSP server (used by ktw-agents)",
		Long: `Starts the sidecar in stdio server mode: reads JSON-RPC 2.0 requests
from stdin, dispatches them to the KTSP container, and writes responses
to stdout. This is the transport used when ktw-agents spawns sidecars
as subprocesses.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ktransport.StdioServe(cmd.Context(), container)
		},
	}
}
