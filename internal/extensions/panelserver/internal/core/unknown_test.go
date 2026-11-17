// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// : a panel the server keeps nothing of is refused by a typed
// answer - the message key panelapi.MsgUnknownPanel, carried across the wire
// in the KTSP error's Data.msg - so a screen tells it by its key, never by
// the error's text.
func TestAnUnknownPanelIsRefusedByItsMessageKey(t *testing.T) {
	ctx := context.Background()
	s, err := New(Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := panel.NewID()
	reads := map[string]func() error{
		"snapshot": func() error { _, err := s.Snapshot(ctx, panelapi.SnapshotInput{Panel: id}); return err },
		"history":  func() error { _, err := s.History(ctx, panelapi.HistoryInput{Panel: id}); return err },
		"keep":     func() error { _, err := s.Keep(ctx, panelapi.KeepInput{Panel: id}); return err },
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			err := read()
			if !errors.Is(err, ErrUnknownPanel) {
				t.Fatalf("a %s of a panel never seen: %v, want ErrUnknownPanel", name, err)
			}
			m, ok := usermsg.Of(err)
			if !ok || m.Key != panelapi.MsgUnknownPanel || m.Params["panel"] != id {
				t.Fatalf("its message: %+v (%v), want key %s naming the panel", m, ok, panelapi.MsgUnknownPanel)
			}
			// Across the wire: the RPC error carries the key in Data.msg.
			if wire, ok := ktsp.FromError(err).UserMsg(); !ok || wire.Key != panelapi.MsgUnknownPanel {
				t.Fatalf("the KTSP error carries %+v (%v), want key %s", wire, ok, panelapi.MsgUnknownPanel)
			}
		})
	}
	// A refusal for another reason carries no such key.
	if _, err := s.Snapshot(ctx, panelapi.SnapshotInput{Panel: "not-a-panel"}); usermsg.Is(err, panelapi.MsgUnknownPanel) {
		t.Fatalf("a malformed id answers the unknown-panel key: %v", err)
	}
}
