// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

func TestEngine_New(t *testing.T) {
	container := ktsp.NewEndpoint("test", "test container", "1.0.0")
	cfg := EngineConfig{Name: "test-engine"}

	e, err := New(cfg, container)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if e.ID() == "" {
		t.Error("expected non-empty ID")
	}
	if len(e.ID()) != 8 {
		t.Errorf("expected 8-char short UUID, got %q", e.ID())
	}
	if e.Name() != "test-engine" {
		t.Errorf("Name() = %q, want %q", e.Name(), "test-engine")
	}
	if e.Container() != container {
		t.Error("Container() returned wrong container")
	}
	if e.Logger() == nil {
		t.Error("expected non-nil logger")
	}
}

func TestEngine_New_NilContainer(t *testing.T) {
	_, err := New(EngineConfig{}, nil)
	if err == nil {
		t.Fatal("expected error for nil container")
	}
}

func TestEngine_New_DefaultName(t *testing.T) {
	container := ktsp.NewEndpoint("test", "", "1.0.0")
	e, err := New(EngineConfig{}, container)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	// Name should be "engine-<uuid>"
	if e.Name()[:7] != "engine-" {
		t.Errorf("expected default name starting with 'engine-', got %q", e.Name())
	}
}

func TestEngine_RunAndShutdown(t *testing.T) {
	container := ktsp.NewEndpoint("test", "", "1.0.0")
	cfg := EngineConfig{
		Name:          "test-engine",
		WorkspaceRoot: t.TempDir(),
		HomeDir:       t.TempDir(),
		SessionsDir:   t.TempDir(),
	}

	e, err := New(cfg, container)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Run in goroutine, shutdown after brief delay
	done := make(chan error, 1)
	go func() {
		done <- e.Run(context.Background())
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown
	e.Shutdown()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after Shutdown()")
	}
}

func TestEngine_NewContext(t *testing.T) {
	container := ktsp.NewEndpoint("test", "", "1.0.0")
	e, err := New(EngineConfig{Name: "ctx-test"}, container)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	slotCfg := map[string]any{"key": "value"}
	ectx := e.NewContext(slotCfg)

	if ectx.ContainerID() != e.ID() {
		t.Errorf("ContainerID() = %q, want %q", ectx.ContainerID(), e.ID())
	}
	if ectx.Logger() == nil {
		t.Error("expected non-nil Logger")
	}
	cfg := ectx.Config()
	if cfg["key"] != "value" {
		t.Errorf("Config() = %v, want key=value", cfg)
	}
}

func TestEngineConfig_SMMode(t *testing.T) {
	if SMEmbedded != "embedded" {
		t.Errorf("SMEmbedded = %q", SMEmbedded)
	}
	if SMRemote != "remote" {
		t.Errorf("SMRemote = %q", SMRemote)
	}
	if SMStandalone != "standalone" {
		t.Errorf("SMStandalone = %q", SMStandalone)
	}
}
