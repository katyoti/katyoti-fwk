// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// A Shutdown that comes before Run has started is not lost: Run returns as
// soon as it starts. (On a loaded machine the goroutine that calls Run may
// start after the caller already asked to shut down.)
func TestAShutdownBeforeRunStillStopsIt(t *testing.T) {
	e, err := New(EngineConfig{Name: "early-stop", WorkspaceRoot: t.TempDir(), HomeDir: t.TempDir(), SessionsDir: t.TempDir()}, ktsp.NewEndpoint("test", "", "1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	e.Shutdown()
	done := make(chan error, 1)
	go func() { done <- e.Run(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return: the Shutdown that came first was lost")
	}
}
