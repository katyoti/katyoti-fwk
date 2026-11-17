// SPDX-License-Identifier: Apache-2.0

package core

import (
	"log/slog"
	"os/exec"
	"time"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
)

// watchExit waits on a member's process — the one Wait its exec.Cmd gets. An
// exit the manager did not ask for (the member crashed, was killed, went down
// with the machine's sleep) takes the member back to DECLARED: a connection
// left READY on a dead process would answer every Acquire with its stale
// sender, and nothing would ever start the member again. The next Acquire
// starts it. The returned channel is closed once the process has exited;
// closeConn waits on it.
func (m *ManagerImpl) watchExit(c *stdioConnection, cmd *exec.Cmd) chan struct{} {
	exited := make(chan struct{})
	go func() {
		err := cmd.Wait()
		close(exited) // before c.mu: closeConn waits on it holding c.mu
		c.mu.Lock()
		if c.cmd != cmd {
			// Closed on purpose (closeConn reset it) or replaced by a new
			// start: nothing to undo.
			c.mu.Unlock()
			return
		}
		prev := c.state
		c.cancelIdleTimerLocked()
		c.cmd = nil
		c.exited = nil
		c.sender = nil
		c.refCount = 0
		c.state = cmapi.ConnDeclared
		c.spawnedAt = time.Time{}
		c.lastCall = time.Time{}
		reason := "exited"
		if err != nil {
			reason = err.Error()
		}
		c.lastErr = reason
		name := c.spec.Name
		c.mu.Unlock()
		slog.Warn("connmgr: a member exited; it starts again at its next call", "sidecar", name, "exit", reason)
		m.emitStateChange(name, prev, cmapi.ConnDeclared, cmapi.ReasonExited+": "+reason)
	}()
	return exited
}
