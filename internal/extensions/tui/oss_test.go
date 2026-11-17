// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"context"
	"errors"
	"testing"
)

// TestOSSBuildHasNoTerminalUI: the published build's entry points answer
// ErrNotInBuild and IsTerminal is false, so `ktw-agents run` takes its plain path.
func TestOSSBuildHasNoTerminalUI(t *testing.T) {
	if IsTerminal() {
		t.Fatal("IsTerminal() = true in the oss build")
	}
	if err := RunTUI(context.Background(), nil); !errors.Is(err, ErrNotInBuild) {
		t.Fatalf("RunTUI = %v, want ErrNotInBuild", err)
	}
	if err := RunMultiPaneConsole(context.Background(), nil, nil); !errors.Is(err, ErrNotInBuild) {
		t.Fatalf("RunMultiPaneConsole = %v, want ErrNotInBuild", err)
	}
	if got := RenderMarkdownWidth("# a", 10); got != "# a" {
		t.Fatalf("RenderMarkdownWidth = %q, want the text unchanged", got)
	}
}
