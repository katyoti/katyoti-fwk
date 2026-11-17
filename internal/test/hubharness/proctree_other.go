// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package hubharness

import (
	"os/exec"
	"strconv"
	"strings"
)

// descendants answers the processes pid started, and theirs (ps lists every
// process with its parent).
func descendants(pid int) []int {
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=").Output()
	if err != nil {
		return nil
	}
	children := map[int][]int{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		c, err1 := strconv.Atoi(f[0])
		p, err2 := strconv.Atoi(f[1])
		if err1 == nil && err2 == nil {
			children[p] = append(children[p], c)
		}
	}
	return walkTree(pid, children)
}
