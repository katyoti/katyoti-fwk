// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteReadInfo(t *testing.T) {
	dir := t.TempDir()
	Init(dir)

	info := ServerInfo{
		Name:    "test-server",
		PID:     12345,
		URL:     "ws://localhost:7770",
		Host:    "localhost",
		Port:    7770,
		Started: time.Now().Truncate(time.Second),
		Version: "0.1.0",
		Exe:     "/usr/bin/test",
	}

	if err := WriteInfo(info); err != nil {
		t.Fatalf("WriteInfo: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "servers", "test-server.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("info file not found: %v", err)
	}

	got, err := ReadInfo("test-server")
	if err != nil {
		t.Fatalf("ReadInfo: %v", err)
	}

	if got.Name != info.Name {
		t.Errorf("Name = %q, want %q", got.Name, info.Name)
	}
	if got.PID != info.PID {
		t.Errorf("PID = %d, want %d", got.PID, info.PID)
	}
	if got.URL != info.URL {
		t.Errorf("URL = %q, want %q", got.URL, info.URL)
	}
	if got.Port != info.Port {
		t.Errorf("Port = %d, want %d", got.Port, info.Port)
	}
}

func TestRemoveInfo(t *testing.T) {
	dir := t.TempDir()
	Init(dir)

	info := ServerInfo{Name: "rm-test", PID: 1}
	if err := WriteInfo(info); err != nil {
		t.Fatal(err)
	}

	if err := RemoveInfo("rm-test"); err != nil {
		t.Fatalf("RemoveInfo: %v", err)
	}

	_, err := ReadInfo("rm-test")
	if !os.IsNotExist(err) {
		t.Errorf("expected not exist, got: %v", err)
	}
}

func TestListServers(t *testing.T) {
	dir := t.TempDir()
	Init(dir)

	for _, name := range []string{"sm", "voices"} {
		if err := WriteInfo(ServerInfo{Name: name, PID: 1}); err != nil {
			t.Fatal(err)
		}
	}

	servers, err := ListServers()
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(servers) != 2 {
		t.Errorf("got %d servers, want 2", len(servers))
	}
}

func TestCleanStaleInfo(t *testing.T) {
	dir := t.TempDir()
	Init(dir)

	// Write info with a PID that definitely doesn't exist
	if err := WriteInfo(ServerInfo{Name: "stale", PID: 999999999}); err != nil {
		t.Fatal(err)
	}

	if !CleanStaleInfo("stale") {
		t.Error("expected stale info to be cleaned")
	}

	_, err := ReadInfo("stale")
	if !os.IsNotExist(err) {
		t.Error("expected info file to be removed")
	}
}

func TestIsProcessAlive(t *testing.T) {
	// Current process should be alive
	if !IsProcessAlive(os.Getpid()) {
		t.Error("current process should be alive")
	}
	// Non-existent PID
	if IsProcessAlive(999999999) {
		t.Error("non-existent PID should not be alive")
	}
	if IsProcessAlive(0) {
		t.Error("PID 0 should not be alive")
	}
}

func TestLifecycleForeground(t *testing.T) {
	dir := t.TempDir()
	Init(dir)

	started := make(chan struct{})
	l := &Lifecycle{
		Name:       "fg-test",
		SubCommand: "test",
		Version:    "0.0.1",
		Host:       "localhost",
		Port:       19999,
		URL:        "ws://localhost:19999",
		Start: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		// Override signal handling — just use our context
		done <- func() error {
			exePath, _ := os.Executable()
			info := ServerInfo{
				Name: l.Name, PID: os.Getpid(), URL: l.URL,
				Host: l.Host, Port: l.Port, Started: time.Now(),
				Version: l.Version, Exe: exePath,
			}
			if err := WriteInfo(info); err != nil {
				return err
			}
			defer RemoveInfo(l.Name)
			return l.Start(ctx)
		}()
	}()

	<-started

	// Verify info file was written
	info, err := ReadInfo("fg-test")
	if err != nil {
		t.Fatalf("ReadInfo after start: %v", err)
	}
	if info.URL != "ws://localhost:19999" {
		t.Errorf("URL = %q", info.URL)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
}
