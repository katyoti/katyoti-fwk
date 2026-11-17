// SPDX-License-Identifier: Apache-2.0

package translator_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
	termadapter "github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/adapter"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/chatpanel"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/room"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/translator"
)

// A reopened session still shows its artifacts: the chat
// panel's events are journaled in the session's journal - but for the
// replaces of an artifact still streaming, which its completion makes
// stale - and a client connecting to the session again gets them in its
// state.sync, each artifact as it ended.
func TestTranslator_AReopenedSessionShowsItsArtifacts(t *testing.T) {
	dir := t.TempDir()
	bus := eventbus.NewMemoryEventBus()
	r := room.NewRoom(dir, nil)
	if _, err := r.CreateSession("s1", "test", "task-1"); err != nil {
		t.Fatal(err)
	}
	tr := translator.New(bus, r, "s1")
	tr.Start()
	bus.Publish(agentapi.EventUIArtifactMount, agentapi.UIArtifactMountEvent{ArtifactID: "f1", Kind: "form", Value: map[string]any{"name": "A"}})
	bus.Publish(agentapi.EventUIArtifactPatch, agentapi.UIArtifactPatchEvent{ArtifactID: "f1", Ops: []any{
		map[string]any{"kind": "replace", "path": "/name", "value": "Ad"},
	}})
	bus.Publish(agentapi.EventUIArtifactPatch, agentapi.UIArtifactPatchEvent{ArtifactID: "f1", Ops: []any{
		map[string]any{"kind": "replace", "path": "/name", "value": "Ada"},
	}})
	bus.Publish(agentapi.EventUIArtifactComplete, agentapi.UIArtifactCompleteEvent{ArtifactID: "f1"})
	bus.Publish(agentapi.EventUIArtifactMount, agentapi.UIArtifactMountEvent{ArtifactID: "p2", Kind: "page", DSLTree: map[string]any{"kind": "text", "content": "hi"}})
	tr.Stop()
	r.Close()

	// The journal holds the open, the two conversation replaces and the
	// completion - never a streaming replace.
	raw, err := os.ReadFile(filepath.Join(dir, "terminal", "s1", "terminal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	panels := 0
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var pe terminal.PanelEvent
		if json.Unmarshal([]byte(line), &pe) != nil || pe.Type != "panel.event" {
			if strings.Contains(line, `"ui_artifact"`) {
				t.Fatalf("the journal still carries an artifact message: %s", line)
			}
			continue
		}
		panels++
		if chatpanel.Transient(pe.Event) {
			t.Fatalf("a streaming replace was journaled: %s", line)
		}
		if pe.Session != "s1" {
			t.Fatalf("a panel event names session %q", pe.Session)
		}
	}
	if panels != 4 {
		t.Fatalf("%d panel events journaled, want 4 (open, two conversations, the completion)", panels)
	}

	// The session reopened: its client's state.sync draws both artifacts.
	r2 := room.NewRoom(dir, nil)
	t.Cleanup(func() { r2.Close() })
	if _, err := r2.CreateSession("s1", "test", "task-1"); err != nil {
		t.Fatal(err)
	}
	server, client := termadapter.NewChannelPair(64)
	t.Cleanup(func() { server.Close() })
	ctx := context.Background()
	if err := r2.Connect(ctx, server, "s1"); err != nil {
		t.Fatal(err)
	}
	ch, err := client.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var sync terminal.StateSyncEvent
	if err := json.Unmarshal(recvOne(t, ch, 2*time.Second), &sync); err != nil || sync.Type != "state.sync" {
		t.Fatalf("state.sync: %v %+v", err, sync)
	}
	arts := map[string]chatpanel.Artifact{}
	for _, pe := range sync.Panels {
		for _, a := range chatpanel.ArtifactsOf(pe.Event) {
			arts[a.ID] = a
		}
	}
	f1, ok := arts["f1"]
	if !ok || f1.Status != chatpanel.StatusComplete {
		t.Fatalf("the reopened session shows f1 as %+v (panels %d)", f1, len(sync.Panels))
	}
	if v, _ := f1.Value.(map[string]any); v["name"] != "Ada" {
		t.Fatalf("f1 reopened with %v, want its last patch", f1.Value)
	}
	if p2, ok := arts["p2"]; !ok || p2.Kind != "page" {
		t.Fatalf("the reopened session lost p2: %+v", arts)
	}
}
