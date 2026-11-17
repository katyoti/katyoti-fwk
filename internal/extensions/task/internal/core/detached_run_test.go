// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// A run outlives the call that started it: a KTSP-served call's context is
// cancelled once it has answered the run id, and the run keeps running to its
// end — only Stop, its deadline or its own failure end it.
func TestARunOutlivesTheCallThatStartedIt(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	call, answered := context.WithCancel(context.Background())
	r, err := svc.Run(call, taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	answered() // the call has answered: its context ends
	eventually(t, "waiting", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusWaiting })
	time.Sleep(200 * time.Millisecond)
	if st := status(t, svc, r.RunID); st != taskapi.RunStatusWaiting {
		t.Fatalf("status = %s after its call ended; want it still waiting", st)
	}
	wake(t, svc, r.RunID)
	eventually(t, "completed", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusCompleted })
}
