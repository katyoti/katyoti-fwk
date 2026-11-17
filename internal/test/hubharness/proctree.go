// SPDX-License-Identifier: Apache-2.0

package hubharness

import (
	"os"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
)

// walkTree answers every process under root in a parent → children map,
// root excluded.
func walkTree(root int, children map[int][]int) []int {
	var out []int
	seen := map[int]bool{root: true}
	queue := []int{root}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, c := range children[p] {
			if c <= 0 || seen[c] {
				continue
			}
			seen[c] = true
			out = append(out, c)
			queue = append(queue, c)
		}
	}
	return out
}

// memberStopWait is how long the harness waits for the processes the hub
// started to be gone once they are stopped.
const memberStopWait = 15 * time.Second

// stopAll ends the given processes and waits until none is alive; it answers
// those still alive after the wait. A member the hub spawned holds files in
// the test's folders (its run owner's lock under <data>/runs/owners): the
// folders can be removed only once it has exited.
func stopAll(pids []int) []int {
	for _, pid := range pids {
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	}
	return waitGone(pids, memberStopWait)
}

// waitGone waits until none of pids is alive, at most d; it answers those
// still alive.
func waitGone(pids []int, d time.Duration) []int {
	deadline := time.Now().Add(d)
	for {
		var alive []int
		for _, pid := range pids {
			if hubclient.ProcessAlive(pid) {
				alive = append(alive, pid)
			}
		}
		if len(alive) == 0 || time.Now().After(deadline) {
			return alive
		}
		time.Sleep(100 * time.Millisecond)
	}
}
