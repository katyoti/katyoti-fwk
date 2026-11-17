// SPDX-License-Identifier: Apache-2.0

//go:build windows

package hubharness

import (
	"os"
	"os/exec"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// tieSelf puts the test process in the process-wide kill-on-close job: the
// processes it starts from then on are created in it, and the ones they start
// too, so none is ever outside it — no window between a start and its
// assignment. The job's one handle is the test process's: when it ends, killed
// or not, the system ends every process left in the job.
func tieSelf() error {
	self, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	return publish.AttachToParentJob(self)
}

func tieCmd(*exec.Cmd) {}
