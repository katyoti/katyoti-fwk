// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// EmitterStub implements agent.Observer by sending events as KTSP
// journal/emit notifications via a Sender. Fire-and-forget.
type EmitterStub struct {
	sender        ktsp.Sender
	containerName string
	sessionID     string
}

// NewEmitterStub creates an emitter stub that routes events through KTSP.
func NewEmitterStub(sender ktsp.Sender, containerName, sessionID string) *EmitterStub {
	return &EmitterStub{
		sender:        sender,
		containerName: containerName,
		sessionID:     sessionID,
	}
}

// OnEvent implements agent.Observer. Errors are logged, not returned.
func (s *EmitterStub) OnEvent(ctx context.Context, eventType string, turn int, data any) {
	if err := s.sender.Notify(ctx, "journal/emit", map[string]any{
		"sessionId":     s.sessionID,
		"containerName": s.containerName,
		"type":          eventType,
		"turn":          turn,
		"data":          data,
	}); err != nil {
		slog.Debug("emitter: failed to emit event", "type", eventType, "error", err)
	}
}

// Close is a no-op for the stub — the remote journal owns its lifecycle.
func (s *EmitterStub) Close() error {
	return nil
}
