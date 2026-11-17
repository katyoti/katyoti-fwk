// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// recordInvocation tells the journal an action ran on the object its input
// names (node_id), after every Run that changes something: the object's
// history shows it (journal History). Best effort by design: ActionService is
// generic infra reused by binaries that may ship no journal, and a failed
// record must never fail (or delay reporting of) the action itself — failures
// log at debug and the Run result passes through untouched.
func (s *Service) recordInvocation(ctx context.Context, uri mbapi.ToolURI, input json.RawMessage, out actionsapi.RunOutput, runErr error) {
	if s.app == nil {
		return
	}
	var in struct {
		NodeID string `json:"node_id"`
	}
	if len(input) == 0 || json.Unmarshal(input, &in) != nil || in.NodeID == "" {
		return
	}
	journal, err := application.AppUseService[journalapi.JournalService](s.app)
	if err != nil {
		slog.Debug("action audit: no journal", "uri", uri.String(), "error", err)
		return
	}
	rec := journalapi.InvokedInput{ID: in.NodeID, URI: uri.String(), Status: "ok", Note: "completed"}
	switch {
	case runErr != nil:
		rec.Status, rec.Note = "failed", runErr.Error()
	case out.TaskID != "":
		rec.Status, rec.Note = "launched", "task "+out.TaskID+" started"
	}
	if _, err := journal.Invoked(ctx, rec); err != nil {
		slog.Debug("action audit: not journaled", "uri", uri.String(), "node", in.NodeID, "error", err)
	}
}
