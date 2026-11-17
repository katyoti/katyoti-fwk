// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestHelperCommand is not a test: one command of the concurrent-commands
// test (it re-runs this binary with the variables set). It connects as every
// verb does (find or spawn, the hub owned), says which hub served it, holds
// the connection — the command that spawned the hub leaves first, the others
// stay — and fails unless it is still connected to that hub when it ends.
func TestHelperCommand(t *testing.T) {
	if os.Getenv("KTW_TEST_COMMAND") != "1" {
		t.Skip("helper process")
	}
	dataDir := os.Getenv("KTW_DATA")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := ConnectToHub(ctx, ConnectOptions{
		DataDir:       dataDir,
		Mode:          ModeFindOrSpawn,
		HubBinaryPath: os.Getenv("KTW_TEST_HUB_BINARY"),
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	pid := c.LockFile().PID
	fmt.Printf("hub-pid=%d spawned=%v\n", pid, c.Spawned())
	hold, _ := time.ParseDuration(os.Getenv("KTW_TEST_HOLD"))
	if c.Spawned() {
		hold = 200 * time.Millisecond
	}
	time.Sleep(hold)
	if !c.handle.IsConnected() {
		t.Fatalf("lost the hub (pid %d) while using it", pid)
	}
	if !ProcessAlive(pid) {
		t.Fatalf("the hub (pid %d) ended while in use", pid)
	}
	_ = c.Close()
}

// EIGHT CONCURRENT COMMANDS, ONE HUB: eight command processes
// started together on a universe with no hub running start exactly one hub
// and all eight succeed through it — the one that spawned it leaves first
// while the others still use it, and that hub serves them to the end. Once
// the last one leaves, the hub ends by itself. Real hub, skipped in -short.
func TestConcurrentCommandsStartOneHubAndAllSucceedThroughIt(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	binary := hubBinary(t)
	dataDir := t.TempDir()

	const commands = 8
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		pids = map[string]int{}
		fail []string
	)
	for i := 0; i < commands; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestHelperCommand$", "-test.v")
			cmd.Env = append(os.Environ(),
				"KTW_TEST_COMMAND=1",
				"KTW_DATA="+dataDir,
				"KTW_TEST_HUB_BINARY="+binary,
				"KTW_TEST_HOLD="+strconv.Itoa(1500+300*i)+"ms",
			)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			err := cmd.Run()
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fail = append(fail, fmt.Sprintf("command %d: %v\n%s", i, err, out.String()))
				return
			}
			for _, line := range strings.Split(out.String(), "\n") {
				if f := strings.Fields(line); len(f) > 0 && strings.HasPrefix(f[0], "hub-pid=") {
					pids[strings.TrimPrefix(f[0], "hub-pid=")]++
				}
			}
		}(i)
	}
	wg.Wait()
	if len(fail) > 0 {
		t.Fatalf("%d of %d commands failed:\n%s", len(fail), commands, strings.Join(fail, "\n"))
	}
	if len(pids) != 1 {
		t.Fatalf("the commands were served by %d hubs, want one: %v", len(pids), pids)
	}
	var pid int
	for p := range pids {
		pid, _ = strconv.Atoi(p)
	}

	// The last command gone, the hub ends by itself and removes its lockfile.
	deadline := time.Now().Add(30 * time.Second)
	for ProcessAlive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("the hub (pid %d) outlived its last client by 30s", pid)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if lf, _ := ReadLockFile(dataDir); lf != nil && lf.PID == pid {
		t.Fatalf("the hub ended but left its lockfile: %+v", lf)
	}
}
