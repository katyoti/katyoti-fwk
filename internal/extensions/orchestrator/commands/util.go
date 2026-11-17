// SPDX-License-Identifier: Apache-2.0

// Package commands provides Cobra commands for the ktw-univers binary.
//
// All commands resolve api.OrchestratorService from DI at RunE time (lazy
// command pattern).
//
// ktw-univers is a one-shot CLI/sidecar with five commands:
// list, info, create, delete, scan-inventory. Each command opens the
// registry, performs its operation, and exits. There is no daemon mode —
// active-universe management lives in ktw-gui per the orchestrator
// architecture ADR.
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// resolveSvc fetches the OrchestratorService from DI. Used by every command's
// RunE.
func resolveSvc(ctx context.Context) (api.OrchestratorService, error) {
	svc, err := application.UseService[api.OrchestratorService](ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve orchestrator service: %w", err)
	}
	return svc, nil
}

// printJSON writes v as indented JSON to w.
func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// universeTable formats a slice of UniverseSummary as a tab-aligned table.
func universeTable(w io.Writer, list []api.UniverseSummary) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tDISPLAY\tHOME\tPROTECTED"); err != nil {
		return err
	}
	for _, u := range list {
		prot := ""
		if u.Protected {
			prot = "*"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", u.Name, u.DisplayName, u.HomePath, prot); err != nil {
			return err
		}
	}
	return tw.Flush()
}
