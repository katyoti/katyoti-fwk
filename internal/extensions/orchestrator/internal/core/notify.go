// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/admin"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// UniversesChangedEvent is the payload of a registry-change notification.
// Deliberately minimal: it says WHAT changed, not what the new state IS.
// Subscribers re-read through the service, so the event can never carry a
// stale or partial view of the registry.
type UniversesChangedEvent struct {
	Change string `json:"change"` // created | updated | deleted | attached | sidecars-changed
	Name   string `json:"name"`
}

// notifyUniversesChanged announces a registry mutation on the admin universe.
// This matters most for `katyoti universe create` in a terminal: exactly the
// change an open GUI cannot otherwise see. Best-effort — see admin.PublishEvent.
func notifyUniversesChanged(ctx context.Context, change, name string) {
	admin.PublishEvent(ctx, subjects.UniversesChangedSubject, UniversesChangedEvent{Change: change, Name: name})
}
