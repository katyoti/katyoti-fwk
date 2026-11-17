// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// A SLOW HUB IS THE UNIVERSE'S HUB: a command whose first dial
// of the live hub went unanswered (the hub busy, the machine loaded) joins
// that hub once it answers; it never starts a second hub that supplants it.

// slowHub stands in front of a real bus: until ready it accepts a connection
// and says nothing (the client's handshake times out), then it relays.
func slowHub(t *testing.T, target string, silentFor time.Duration) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	ready := time.Now().Add(silentFor)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				if time.Now().Before(ready) {
					_, _ = io.Copy(io.Discard, conn) // silent until the client gives up
					return
				}
				up, err := net.Dial("tcp", target)
				if err != nil {
					return
				}
				defer up.Close()
				go func() { _, _ = io.Copy(up, conn) }()
				_, _ = io.Copy(conn, up)
			}(conn)
		}
	}()
	return "nats://" + ln.Addr().String()
}

func TestFindOrSpawnJoinsALiveHubThatIsSlowToAnswer(t *testing.T) {
	asHub(t)
	var spawns int32
	prev := spawnHub
	spawnHub = func(context.Context, ConnectOptions) (*HubClient, error) {
		atomic.AddInt32(&spawns, 1)
		return nil, errors.New("spawned")
	}
	t.Cleanup(func() { spawnHub = prev })

	bus := startBus(t)
	addr := bus.Server.Addr().String()
	url := slowHub(t, addr, 3*time.Second)
	dir := t.TempDir()
	if err := WriteLockFile(dir, LockFile{PID: os.Getpid(), NATSURL: url}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	hc, err := ConnectToHub(ctx, ConnectOptions{DataDir: dir, Mode: ModeFindOrSpawn})
	if n := atomic.LoadInt32(&spawns); n != 0 {
		t.Fatalf("%d hub(s) spawned beside the live hub that was slow to answer", n)
	}
	if err != nil {
		t.Fatalf("the slow live hub was not joined: %v", err)
	}
	defer hc.Close()
	if hc.Spawned() {
		t.Error("the client reports it started the hub it joined")
	}
	if lf, _ := ReadLockFile(dir); lf == nil || lf.NATSURL != url {
		t.Fatalf("the live hub's lockfile was replaced or removed: %+v", lf)
	}
}
