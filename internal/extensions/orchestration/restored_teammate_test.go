// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
)

// A teammate restored after a restart — its turn is no longer the first —
// still joins its mailbox and team in its new process: messages sent to it
// reach it. (The setup ran only on turn 1: a restored teammate heard nothing.)
func TestARestoredTeammateStillReceivesItsMessages(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()
	loop := newStubLoop()
	loop.SetMeta("_teammate_name", "bob")
	loop.SetMeta("_team_name", "alpha")
	bob := &orchPlugin{loop: loop, mailboxMgr: NewMailboxManager(), teamMgr: NewTeamManager(t.TempDir())}
	bob.mailboxMgr.SetEventBus(bus)
	defer bob.mailboxMgr.Unsubscribe()

	if err := bob.autoSubscribeTopics(loop, 5); err != nil { // restored at turn 5
		t.Fatal(err)
	}

	alice := NewMailboxManager()
	alice.SetEventBus(bus)
	if _, err := alice.SendMessage("alice", "bob", "still there?", "", ""); err != nil {
		t.Fatal(err)
	}
	if msgs := bob.mailboxMgr.ReadMailbox("bob"); len(msgs) != 1 {
		t.Fatalf("bob's mailbox = %d messages; want the one sent to him", len(msgs))
	}
}
