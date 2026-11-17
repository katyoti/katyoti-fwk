// SPDX-License-Identifier: Apache-2.0

package tui

import "github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"

// PaneSpec describes one task run's console pane: a label + the terminal
// transport carrying that run's ALREADY-TRANSCODED terminal messages (a
// per-run translator feeds it). The multi-pane console adds one pane per spec.
type PaneSpec struct {
	RunID     string
	Label     string
	Transport terminal.TerminalTransport
}
