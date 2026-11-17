// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"os"
	"testing"
	"time"
)

// A MEMBER'S OWN HUBCLIENT NEVER KEEPS AN OWNED HUB UP: a
// process the hub serves (`<binary> sc ktsp nats`) dials its hub under a
// member name, so once the last outside client leaves, the hub's count
// (Clients(ClientName)) reaches none and it ends — the member still connected.
func TestOwnedHubEndsWithAMemberHubClientConnected(t *testing.T) {
	keepPoll, keepFirst := unusedPoll, firstClientWait
	unusedPoll, firstClientWait = 10*time.Millisecond, 5*time.Second
	t.Cleanup(func() { unusedPoll, firstClientWait = keepPoll, keepFirst })
	keepArgs := processArgs
	t.Cleanup(func() { processArgs = keepArgs })

	dataDir := t.TempDir()
	bus := startBus(t)
	if err := WriteLockFile(dataDir, LockFile{PID: os.Getpid(), NATSURL: bus.Server.ClientURL()}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dial := func(args []string) *HubClient {
		t.Helper()
		processArgs = func() []string { return args }
		hc, err := ConnectToHub(ctx, ConnectOptions{DataDir: dataDir, Mode: ModeReuse})
		if err != nil {
			t.Fatalf("connect %v: %v", args, err)
		}
		return hc
	}

	// The member, served by the hub after a host flag, dials for its own needs.
	member := dial([]string{`C:\k\bin\ktw-agents.exe`, "--folder", "x", "sc", "ktsp", "nats"})
	defer member.Close()
	// A command of the universe: an outside client.
	outside := dial([]string{"ktw-agents", "todos", "modes", "--out", "json"})

	if got := dialNameOf([]string{"/k/ktw-agents", "sc", "ktsp", "nats"}); got != MemberClientName+":ktw-agents" {
		t.Errorf("a member dials as %q, want %q", got, MemberClientName+":ktw-agents")
	}
	for _, args := range [][]string{{"ktw-agents", "sc", "ktsp", "stdio"}, {"sc", "ktsp", "nats"}, {"ktw-agents"}, nil} {
		if got := dialNameOf(args); got != ClientName {
			t.Errorf("%v dials as %q, want the outside client's %q", args, got, ClientName)
		}
	}

	ended := make(chan struct{})
	go EndWhenUnused(ctx, func() (int, int) { return bus.Clients(ClientName) },
		func() bool { return true }, func() {}, func() { close(ended) })

	select {
	case <-ended:
		t.Fatal("the hub ended while an outside client was connected")
	case <-time.After(300 * time.Millisecond):
	}
	outside.Close()
	select {
	case <-ended:
	case <-time.After(10 * time.Second):
		open, _ := bus.Clients(ClientName)
		t.Fatalf("the hub did not end after its last outside client left: %d connection(s) still counted as outside clients, the member's included", open)
	}
}

func dialNameOf(args []string) string {
	keep := processArgs
	defer func() { processArgs = keep }()
	processArgs = func() []string { return args }
	return dialName()
}
