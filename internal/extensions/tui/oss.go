// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
)

// The published build has no terminal UI: the Bubbletea and Glamour files
// are tagged !oss (their dependencies stay out of it). These counterparts
// keep the package's API so its importers compile.

// IsTerminal reports false: with no terminal UI, callers take their plain path.
func IsTerminal() bool { return false }

// RenderMarkdown returns text unchanged (no Glamour in this build).
func RenderMarkdown(text string) string { return text }

// RenderMarkdownWidth returns text unchanged (no Glamour in this build).
func RenderMarkdownWidth(text string, _ int) string { return text }

// PrintMarkdown prints text as it is.
func PrintMarkdown(text string) { fmt.Print(text) }

// PrintMarkdownWidth prints text as it is.
func PrintMarkdownWidth(text string, _ int) { fmt.Print(text) }

// RunTUI answers ErrNotInBuild.
func RunTUI(context.Context, terminal.TerminalTransport) error { return ErrNotInBuild }

// RunMultiPaneConsole answers ErrNotInBuild.
func RunMultiPaneConsole(context.Context, <-chan PaneSpec, <-chan struct{}) error {
	return ErrNotInBuild
}
