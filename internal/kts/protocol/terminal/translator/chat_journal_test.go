// SPDX-License-Identifier: Apache-2.0

package translator_test

import (
	"context"
	"encoding/json"
	"fmt"
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

// A long run's journal holds each chat panel node once:
// every message replaces the whole conversation node, a large artifact
// included, but the journal file does not grow by messages x conversation
// size - while the run goes it stays within its live panels plus a bound,
// and the run's end leaves each node's last replace once; the reopened
// session still shows the artifact and every message.
func TestTranslator_ALongRunJournalsEachChatNodeOnce(t *testing.T) {
	dir := t.TempDir()
	bus := eventbus.NewMemoryEventBus()
	r := room.NewRoom(dir, nil)
	t.Cleanup(func() { r.Close() })
	if _, err := r.CreateSession("s1", "test", "task-1"); err != nil {
		t.Fatal(err)
	}
	tr := translator.New(bus, r, "s1")
	tr.Start()

	big := strings.Repeat("x", 64<<10) // a drawio diagram's size
	bus.Publish(agentapi.EventUIArtifactMount, agentapi.UIArtifactMountEvent{ArtifactID: "d1", Kind: "form", Value: map[string]any{"xml": big}})
	bus.Publish(agentapi.EventUIArtifactComplete, agentapi.UIArtifactCompleteEvent{ArtifactID: "d1"})
	const messages = 40
	for i := 1; i <= messages; i++ {
		bus.Publish(agentapi.EventTextDelta, agentapi.TextDeltaEvent{Turn: i, Text: fmt.Sprintf("message %d", i)})
		bus.Publish(agentapi.EventLLMResponse, agentapi.LLMResponseEvent{Turn: i})
	}

	path := filepath.Join(dir, "terminal", "s1", "terminal.jsonl")
	conversation := lastConversationSize(t, path)
	running := fileSize(t, path)
	// Live: the open, the artifact's completion and the last conversation
	// (each about the artifact's size); beyond it at most the bound.
	if limit := int64(1<<20) + 3*conversation; running > limit {
		t.Fatalf("the running journal is %d bytes for a %d-byte conversation, want at most %d (it grows by messages x conversation)", running, conversation, limit)
	}

	bus.Publish(agentapi.EventComplete, agentapi.CompleteEvent{EventBase: agentapi.EventBase{Type: agentapi.EventComplete}, Status: "completed", Summary: "done"})
	tr.Stop()
	r.Close()

	replaces := map[string]int{}
	for _, pe := range journalPanels(t, path) {
		for _, op := range pe.Event.Ops {
			replaces[op.Target]++
		}
	}
	if replaces[chatpanel.ArtifactsNode] != 1 || replaces[chatpanel.NodeID("d1")] != 1 {
		t.Fatalf("the ended run's journal replaces %v, want each chat node once", replaces)
	}

	// Reopened, the session shows the artifact and every message.
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
	var kids []any
	var d1 chatpanel.Artifact
	for _, pe := range sync.Panels {
		for _, op := range pe.Event.Ops {
			if op.Target == chatpanel.ArtifactsNode {
				kids, _ = op.Node["children"].([]any)
			}
		}
		for _, a := range chatpanel.ArtifactsOf(pe.Event) {
			if a.ID == "d1" {
				d1 = a
			}
		}
	}
	if len(kids) != messages+1 {
		t.Fatalf("the reopened conversation holds %d nodes, want the artifact and %d messages", len(kids), messages)
	}
	if d1.Status != chatpanel.StatusComplete {
		t.Fatalf("the reopened session shows d1 as %+v", d1)
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Size()
}

// lastConversationSize is the bytes of the journal's last line replacing
// the conversation node.
func lastConversationSize(t *testing.T, path string) int64 {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var size int64
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var pe terminal.PanelEvent
		if json.Unmarshal([]byte(line), &pe) != nil || pe.Type != "panel.event" {
			continue
		}
		for _, op := range pe.Event.Ops {
			if op.Target == chatpanel.ArtifactsNode {
				size = int64(len(line) + 1)
			}
		}
	}
	if size == 0 {
		t.Fatal("the journal holds no conversation replace")
	}
	return size
}

func journalPanels(t *testing.T, path string) []terminal.PanelEvent {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []terminal.PanelEvent
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var pe terminal.PanelEvent
		if json.Unmarshal([]byte(line), &pe) == nil && pe.Type == "panel.event" {
			out = append(out, pe)
		}
	}
	return out
}
